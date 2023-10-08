package mocrelay

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	ErrRecvClosed = errors.New("recv closed")
)

type Handler interface {
	Handle(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error
}

type HandlerFunc func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error

func (f HandlerFunc) Handle(
	r *http.Request,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	return f(r, recv, send)
}

type SimpleHandler Handler

type SimpleHandlerInterface interface {
	HandleStart(*http.Request) (*http.Request, error)
	HandleStop(*http.Request) error
	HandleClientMsg(*http.Request, ClientMsg) ([]ServerMsg, error)
}

func NewSimpleHandler(h SimpleHandlerInterface) SimpleHandler {
	return HandlerFunc(
		func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) (err error) {
			defer func() { err = errors.Join(err, h.HandleStop(r)) }()

			r, err = h.HandleStart(r)
			if err != nil {
				return
			}

			ctx := r.Context()
			var smsgs []ServerMsg

			for cmsg := range recv {
				smsgs, err = h.HandleClientMsg(r, cmsg)
				if err != nil {
					return
				}

				for _, smsg := range smsgs {
					sendServerMsgCtx(ctx, send, smsg)
				}
			}

			return ErrRecvClosed
		},
	)
}

var ErrRouterStop = errors.New("router stopped")

type RouterOption struct {
	BufLen int
	// TODO(high-moctane) Add logger
}

func (opt *RouterOption) bufLen() int {
	if opt == nil || opt.BufLen == 0 {
		return 50
	}
	return opt.BufLen
}

type Router struct {
	Option *RouterOption

	subs *subscribers
}

func NewRouter(option *RouterOption) *Router {
	return &Router{
		Option: option,
		subs:   newSubscribers(),
	}
}

func (router *Router) Handle(
	r *http.Request,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	reqID := GetRequestID(ctx)
	defer router.subs.UnsubscribeAll(reqID)

	subCh := make(chan ServerMsg, router.Option.bufLen())

Loop:
	for {
		select {
		case <-ctx.Done():
			break Loop

		case msg, ok := <-recv:
			if !ok {
				break Loop
			}
			m := router.recv(ctx, reqID, msg, subCh)
			sendServerMsgCtx(ctx, send, m)

		case msg := <-subCh:
			sendCtx(ctx, send, msg)
		}
	}

	return ErrRouterStop
}

func (router *Router) recv(
	ctx context.Context,
	reqID string,
	msg ClientMsg,
	subCh chan ServerMsg,
) ServerMsg {
	switch m := msg.(type) {
	case *ClientReqMsg:
		return router.recvClientReqMsg(ctx, reqID, m, subCh)

	case *ClientEventMsg:
		return router.recvClientEventMsg(ctx, reqID, m, subCh)

	case *ClientCloseMsg:
		return router.recvClientCloseMsg(ctx, reqID, m)

	default:
		return nil
	}
}

func (router *Router) recvClientReqMsg(
	ctx context.Context,
	reqID string,
	msg *ClientReqMsg,
	subCh chan ServerMsg,
) ServerMsg {
	sub := newSubscriber(reqID, msg, subCh)
	router.subs.Subscribe(sub)
	return NewServerEOSEMsg(msg.SubscriptionID)
}

func (router *Router) recvClientEventMsg(
	ctx context.Context,
	reqID string,
	msg *ClientEventMsg,
	subCh chan ServerMsg,
) ServerMsg {
	router.subs.Publish(msg.Event)
	return NewServerOKMsg(msg.Event.ID, true, ServerOKMsgPrefixNoPrefix, "")
}

func (router *Router) recvClientCloseMsg(
	ctx context.Context,
	reqID string,
	msg *ClientCloseMsg,
) ServerMsg {
	router.subs.Unsubscribe(reqID, msg.SubscriptionID)
	return nil
}

type subscriber struct {
	ReqID          string
	SubscriptionID string
	Matcher        EventMatcher
	Ch             chan ServerMsg
}

func newSubscriber(reqID string, msg *ClientReqMsg, ch chan ServerMsg) *subscriber {
	return &subscriber{
		ReqID:          reqID,
		SubscriptionID: msg.SubscriptionID,
		Matcher:        NewReqFiltersEventMatchers(msg.ReqFilters),
		Ch:             ch,
	}
}

func (sub *subscriber) SendIfMatch(event *Event) {
	if sub.Matcher.Match(event) {
		trySendCtx(context.TODO(), sub.Ch, ServerMsg(NewServerEventMsg(sub.SubscriptionID, event)))
	}
}

type subscribers struct {
	// map[reqID]map[subID]*subscriber
	subs chan map[string]chan map[string]chan *subscriber
}

func newSubscribers() *subscribers {
	subs := make(chan map[string]chan map[string]chan *subscriber, 1)
	subs <- make(map[string]chan map[string]chan *subscriber)
	return &subscribers{
		subs: subs,
	}
}

func (subs *subscribers) Subscribe(sub *subscriber) {
	m := <-subs.subs
	mch, ok := m[sub.ReqID]
	if ok {
		subs.subs <- m
	} else {
		mch = make(chan map[string]chan *subscriber, 1)
		m[sub.ReqID] = mch
		subs.subs <- m
		mch <- make(map[string]chan *subscriber)
	}

	mm := <-mch
	mmch, ok := mm[sub.SubscriptionID]
	if ok {
		mch <- mm
		<-mmch
	} else {
		mmch = make(chan *subscriber, 1)
		mm[sub.SubscriptionID] = mmch
		mch <- mm
	}

	mmch <- sub
}

func (subs *subscribers) Unsubscribe(reqID, subID string) {
	m := <-subs.subs
	mch, ok := m[subID]
	subs.subs <- m
	if !ok {
		return
	}
	mm := <-mch
	delete(mm, subID)
	mch <- mm
}

func (subs *subscribers) UnsubscribeAll(reqID string) {
	m := <-subs.subs
	delete(m, reqID)
	subs.subs <- m
}

func (subs *subscribers) Publish(event *Event) {
	m := <-subs.subs
	mchs := make([]chan map[string]chan *subscriber, 0, len(m))
	for _, mch := range m {
		mchs = append(mchs, mch)
	}
	subs.subs <- m

	var mmchs []chan *subscriber
	for _, mch := range mchs {
		mm := <-mch
		for _, mmch := range mm {
			mmchs = append(mmchs, mmch)
		}
		mch <- mm
	}

	for _, mmch := range mmchs {
		s := <-mmch
		s.SendIfMatch(event)
		mmch <- s
	}
}

type CacheHandler SimpleHandler

func NewCacheHandler(size int) CacheHandler {
	return CacheHandler(NewSimpleHandler(newSimpleCacheHandler(size)))
}

type simpleCacheHandler struct {
	sema chan struct{}
	c    *eventCache
}

func newSimpleCacheHandler(size int) *simpleCacheHandler {
	return &simpleCacheHandler{
		sema: make(chan struct{}, runtime.GOMAXPROCS(0)),
		c:    newEventCache(size),
	}
}

func (h *simpleCacheHandler) HandleStart(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (h *simpleCacheHandler) HandleStop(r *http.Request) error {
	return nil
}

func (h *simpleCacheHandler) HandleClientMsg(r *http.Request, msg ClientMsg) ([]ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientEventMsg:
		for i := 0; i < cap(h.sema); i++ {
			h.sema <- struct{}{}
		}
		defer func() {
			for i := 0; i < cap(h.sema); i++ {
				<-h.sema
			}
		}()

		ev := msg.Event
		if ev.Kind == 5 {
			for _, tag := range ev.Tags {
				if len(tag) < 2 {
					continue
				}
				switch tag[0] {
				case "e":
					h.c.DeleteID(tag[1], ev.Pubkey)
				case "a":
					h.c.DeleteNaddr(tag[1], ev.Pubkey)
				}
			}
		}

		var okMsg *ServerOKMsg
		if h.c.Add(ev) {
			okMsg = NewServerOKMsg(msg.Event.ID, true, "", "")
		} else {
			okMsg = NewServerOKMsg(msg.Event.ID, false, ServerOKMsgPrefixDuplicate, "already have this event")
		}
		return []ServerMsg{okMsg}, nil

	case *ClientReqMsg:
		h.sema <- struct{}{}
		defer func() { <-h.sema }()

		evs := h.c.Find(NewReqFiltersEventMatchers(msg.ReqFilters))
		var ret []ServerMsg
		for _, ev := range evs {
			ret = append(ret, NewServerEventMsg(msg.SubscriptionID, ev))
		}
		ret = append(ret, NewServerEOSEMsg(msg.SubscriptionID))
		return ret, nil

	case *ClientCountMsg:
		return []ServerMsg{NewServerCountMsg(msg.SubscriptionID, 0, nil)}, nil
	default:
		return nil, nil
	}
}

type MergeHandler struct {
	hs []Handler
}

func NewMergeHandler(handlers ...Handler) Handler {
	if len(handlers) < 2 {
		panic(fmt.Sprintf("handlers must be two or more but got %d", len(handlers)))
	}
	return &MergeHandler{
		hs: handlers,
	}
}

func (h *MergeHandler) Handle(
	r *http.Request,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	return newMergeHandlerSession(h).Handle(r, recv, send)
}

type mergeHandlerSession struct {
	h         *MergeHandler
	recvs     []chan ClientMsg
	sends     []chan ServerMsg
	preSendCh chan *mergeHandlerSessionSendMsg

	okStat    *mergeHandlerSessionOKState
	reqStat   *mergeHandlerSessionReqState
	countStat *mergeHandlerSessionCountState
}

func newMergeHandlerSession(h *MergeHandler) *mergeHandlerSession {
	size := len(h.hs)

	recvs := make([]chan ClientMsg, size)
	sends := make([]chan ServerMsg, size)
	for i := 0; i < len(h.hs); i++ {
		recvs[i] = make(chan ClientMsg, 1)
		sends[i] = make(chan ServerMsg, 1)
	}

	return &mergeHandlerSession{
		h:         h,
		recvs:     recvs,
		sends:     sends,
		preSendCh: make(chan *mergeHandlerSessionSendMsg, len(h.hs)+1),
		okStat:    newMergeHandlerSessionOKState(size),
		reqStat:   newMergeHandlerSessionReqState(size),
		countStat: newMergeHandlerSessionCountState(size),
	}
}

func (ss *mergeHandlerSession) Handle(
	r *http.Request,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	defer cancel()

	go func() {
		defer cancel()
		ss.mergeSends(ctx)
	}()
	go func() {
		defer ss.closeRecvs()
		defer cancel()
		ss.handleRecvSend(ctx, recv, send)
	}()

	return ss.runHandlers(r)
}

func (ss *mergeHandlerSession) runHandlers(r *http.Request) error {
	hs := ss.h.hs
	var wg sync.WaitGroup
	errs := make(chan error, len(hs))

	wg.Add(len(hs))
	for i := 0; i < len(ss.h.hs); i++ {
		go func(i int) {
			defer wg.Done()
			errs <- hs[i].Handle(r, ss.recvs[i], ss.sends[i])
		}(i)
	}
	wg.Wait()

	close(errs)
	var err error
	for e := range errs {
		err = errors.Join(err, e)
	}

	return err
}

func (ss *mergeHandlerSession) mergeSends(ctx context.Context) {
	ss.mergeSend(ctx, ss.sends)
}

func (ss *mergeHandlerSession) mergeSend(ctx context.Context, sends []chan ServerMsg) {
	if len(sends) == 0 {
		return
	}

	idx := len(sends) - 1

	go ss.mergeSend(ctx, sends[:idx])

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sends[idx]:
			sendCtx(ctx, ss.preSendCh, newMergeHandlerSessionSendMsg(idx, msg))
		}
	}
}

func (ss *mergeHandlerSession) closeRecvs() {
	for _, r := range ss.recvs {
		close(r)
	}
}

func (ss *mergeHandlerSession) handleRecvSend(
	ctx context.Context,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) {
	recvCh := recv
	sendCh := ss.preSendCh
	recvBuf := make(chan ClientMsg, 1)
	sendBuf := make(chan ServerMsg, 1)

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-recvCh:
			if !ok {
				return
			}
			msg = ss.handleRecvMsg(ctx, msg)
			sendClientMsgCtx(ctx, recvBuf, msg)
			recvCh = nil

		case msg, ok := <-recvBuf:
			if !ok {
				return
			}
			ss.broadcastRecvs(ctx, msg)
			recvCh = recv

		case msg := <-sendCh:
			m := ss.handleSendMsg(ctx, msg)
			if sendServerMsgCtx(ctx, sendBuf, m) {
				sendCh = nil
			}

		case msg := <-sendBuf:
			sendCtx(ctx, send, msg)
			sendCh = ss.preSendCh
		}
	}
}

func (ss *mergeHandlerSession) handleRecvMsg(
	ctx context.Context,
	msg ClientMsg,
) ClientMsg {
	switch msg := msg.(type) {
	case *ClientEventMsg:
		return ss.handleRecvEventMsg(ctx, msg)
	case *ClientReqMsg:
		return ss.handleRecvReqMsg(ctx, msg)
	case *ClientCloseMsg:
		return ss.handleRecvCloseMsg(ctx, msg)
	case *ClientCountMsg:
		return ss.handleRecvCountMsg(ctx, msg)
	default:
		return msg
	}
}

func (ss *mergeHandlerSession) handleRecvEventMsg(
	ctx context.Context,
	msg *ClientEventMsg,
) ClientMsg {
	ss.okStat.TrySetEventID(msg.Event.ID)
	return msg
}

func (ss *mergeHandlerSession) handleRecvReqMsg(
	ctx context.Context,
	msg *ClientReqMsg,
) ClientMsg {
	ss.reqStat.SetSubID(msg.SubscriptionID)
	return msg
}

func (ss *mergeHandlerSession) handleRecvCloseMsg(
	ctx context.Context,
	msg *ClientCloseMsg,
) ClientMsg {
	ss.reqStat.ClearSubID(msg.SubscriptionID)
	return msg
}

func (ss *mergeHandlerSession) handleRecvCountMsg(
	ctx context.Context,
	msg *ClientCountMsg,
) ClientMsg {
	ss.countStat.SetSubID(msg.SubscriptionID)
	return msg
}

func (ss *mergeHandlerSession) broadcastRecvs(ctx context.Context, msg ClientMsg) {
	for _, r := range ss.recvs {
		if !sendCtx(ctx, r, msg) {
			return
		}
	}
}

type mergeHandlerSessionSendMsg struct {
	Idx int
	Msg ServerMsg
}

func newMergeHandlerSessionSendMsg(idx int, msg ServerMsg) *mergeHandlerSessionSendMsg {
	return &mergeHandlerSessionSendMsg{
		Idx: idx,
		Msg: msg,
	}
}

func (ss *mergeHandlerSession) handleSendMsg(
	ctx context.Context,
	msg *mergeHandlerSessionSendMsg,
) ServerMsg {
	switch msg.Msg.(type) {
	case *ServerEOSEMsg:
		return ss.handleSendEOSEMsg(ctx, msg)
	case *ServerEventMsg:
		return ss.handleSendEventMsg(ctx, msg)
	case *ServerOKMsg:
		return ss.handleSendOKMsg(ctx, msg)
	case *ServerCountMsg:
		return ss.handleSendCountMsg(ctx, msg)
	default:
		return msg.Msg
	}
}

func (ss *mergeHandlerSession) handleSendEOSEMsg(
	ctx context.Context,
	msg *mergeHandlerSessionSendMsg,
) *ServerEOSEMsg {
	m := msg.Msg.(*ServerEOSEMsg)

	if ss.reqStat.AllEOSE(m.SubscriptionID) {
		return nil
	}
	ss.reqStat.SetEOSE(m.SubscriptionID, msg.Idx)
	if !ss.reqStat.AllEOSE(m.SubscriptionID) {
		return nil
	}

	return m
}

func (ss *mergeHandlerSession) handleSendEventMsg(
	ctx context.Context,
	msg *mergeHandlerSessionSendMsg,
) *ServerEventMsg {
	m := msg.Msg.(*ServerEventMsg)

	if !ss.reqStat.IsSendableEventMsg(msg.Idx, m) {
		return nil
	}

	return m
}

func (ss *mergeHandlerSession) handleSendOKMsg(
	ctx context.Context,
	msg *mergeHandlerSessionSendMsg,
) *ServerOKMsg {
	m := msg.Msg.(*ServerOKMsg)

	ss.okStat.SetMsg(msg.Idx, m)
	if !ss.okStat.Ready(m.EventID) {
		return nil
	}

	ret := ss.okStat.Msg(m.EventID)
	ss.okStat.ClearEventID(m.EventID)

	return ret
}

func (ss *mergeHandlerSession) handleSendCountMsg(
	ctx context.Context,
	msg *mergeHandlerSessionSendMsg,
) *ServerCountMsg {
	m := msg.Msg.(*ServerCountMsg)

	ss.countStat.SetCountMsg(msg.Idx, m)
	if !ss.countStat.Ready(m.SubscriptionID, msg.Idx) {
		return nil
	}

	ret := ss.countStat.Msg(m.SubscriptionID)
	ss.countStat.ClearSubID(m.SubscriptionID)

	return ret
}

type mergeHandlerSessionOKState struct {
	size int
	// map[eventID][chIdx]msg
	s map[string][]*ServerOKMsg
}

func newMergeHandlerSessionOKState(size int) *mergeHandlerSessionOKState {
	return &mergeHandlerSessionOKState{
		size: size,
		s:    make(map[string][]*ServerOKMsg),
	}
}

func (stat *mergeHandlerSessionOKState) TrySetEventID(eventID string) {
	if len(stat.s[eventID]) > 0 {
		return
	}
	stat.s[eventID] = make([]*ServerOKMsg, stat.size)
}

func (stat *mergeHandlerSessionOKState) SetMsg(chIdx int, msg *ServerOKMsg) {
	msgs := stat.s[msg.EventID]
	if len(msgs) == 0 {
		return
	}
	msgs[chIdx] = msg
}

func (stat *mergeHandlerSessionOKState) Ready(eventID string) bool {
	msgs := stat.s[eventID]
	if len(msgs) == 0 {
		return false
	}
	return !slices.Contains(msgs, nil)
}

func (stat *mergeHandlerSessionOKState) Msg(eventID string) *ServerOKMsg {
	msgs := stat.s[eventID]
	if len(msgs) == 0 {
		panic(fmt.Sprintf("invalid eventID %s", eventID))
	}

	var oks, ngs []*ServerOKMsg
	for _, msg := range msgs {
		if msg.Accepted {
			oks = append(oks, msg)
		} else {
			ngs = append(ngs, msg)
		}
	}

	if len(ngs) > 0 {
		return joinServerOKMsgs(ngs...)
	}

	return joinServerOKMsgs(oks...)
}

func joinServerOKMsgs(msgs ...*ServerOKMsg) *ServerOKMsg {
	b := new(strings.Builder)
	for _, msg := range msgs {
		b.WriteString(msg.Message())
	}
	return NewServerOKMsg(msgs[0].EventID, msgs[0].Accepted, "", b.String())
}

func (stat *mergeHandlerSessionOKState) ClearEventID(eventID string) {
	delete(stat.s, eventID)
}

type mergeHandlerSessionReqState struct {
	size int
	// map[subID][chIdx]eose?
	eose map[string][]bool
	// map[subID]event
	lastEvent map[string]*ServerEventMsg
	// map[subID]map[eventID]seen
	seen map[string]map[string]bool
}

func newMergeHandlerSessionReqState(size int) *mergeHandlerSessionReqState {
	return &mergeHandlerSessionReqState{
		size:      size,
		eose:      make(map[string][]bool),
		lastEvent: make(map[string]*ServerEventMsg),
		seen:      make(map[string]map[string]bool),
	}
}

func (stat *mergeHandlerSessionReqState) SetSubID(subID string) {
	stat.eose[subID] = make([]bool, stat.size)
	stat.seen[subID] = make(map[string]bool)
}

func (stat *mergeHandlerSessionReqState) SetEOSE(subID string, chIdx int) {
	if len(stat.eose[subID]) == 0 {
		return
	}
	stat.eose[subID][chIdx] = true
}

func (stat *mergeHandlerSessionReqState) AllEOSE(subID string) bool {
	eoses := stat.eose[subID]
	return len(eoses) == 0 || !slices.Contains(eoses, false)
}

func (stat *mergeHandlerSessionReqState) IsEOSE(subID string, chIdx int) bool {
	eoses := stat.eose[subID]
	return len(eoses) == 0 || eoses[chIdx]
}

func (stat *mergeHandlerSessionReqState) IsSendableEventMsg(
	chIdx int,
	msg *ServerEventMsg,
) bool {
	if stat.seen[msg.SubscriptionID] == nil || stat.seen[msg.SubscriptionID][msg.Event.ID] {
		return false
	}
	stat.seen[msg.SubscriptionID][msg.Event.ID] = true

	if stat.AllEOSE(msg.SubscriptionID) {
		return true
	}

	if stat.IsEOSE(msg.SubscriptionID, chIdx) {
		return false
	}

	old := stat.lastEvent[msg.SubscriptionID]
	if old == nil {
		stat.lastEvent[msg.SubscriptionID] = msg
		return true
	}
	if old.Event.CreatedAt >= msg.Event.CreatedAt {
		stat.lastEvent[msg.SubscriptionID] = msg
		return true
	}
	return false
}

func (stat *mergeHandlerSessionReqState) ClearSubID(subID string) {
	delete(stat.eose, subID)
	delete(stat.lastEvent, subID)
	delete(stat.seen, subID)
}

type mergeHandlerSessionCountState struct {
	size int
	// map[subID][chIDx]msg
	counts map[string][]*ServerCountMsg
}

func newMergeHandlerSessionCountState(size int) *mergeHandlerSessionCountState {
	return &mergeHandlerSessionCountState{
		size:   size,
		counts: make(map[string][]*ServerCountMsg),
	}
}

func (stat *mergeHandlerSessionCountState) SetSubID(subID string) {
	stat.counts[subID] = make([]*ServerCountMsg, stat.size)
}

func (stat *mergeHandlerSessionCountState) SetCountMsg(chIdx int, msg *ServerCountMsg) {
	counts := stat.counts[msg.SubscriptionID]
	if len(counts) == 0 {
		return
	}
	counts[chIdx] = msg
}

func (stat *mergeHandlerSessionCountState) Ready(subID string, chIdx int) bool {
	counts := stat.counts[subID]
	if len(counts) == 0 {
		return false
	}
	return !slices.Contains(counts, nil)
}

func (stat *mergeHandlerSessionCountState) Msg(subID string) *ServerCountMsg {
	return slices.MaxFunc(
		stat.counts[subID],
		func(a, b *ServerCountMsg) int { return cmp.Compare(a.Count, b.Count) },
	)
}

func (stat *mergeHandlerSessionCountState) ClearSubID(subID string) {
	delete(stat.counts, subID)
}

type Middleware func(Handler) Handler

type SimpleMiddleware Middleware

type SimpleMiddlewareInterface interface {
	HandleStart(*http.Request) (*http.Request, error)
	HandleStop(*http.Request) error
	HandleClientMsg(*http.Request, ClientMsg) (ClientMsg, []ServerMsg, error)
	HandleServerMsg(*http.Request, ServerMsg) ([]ServerMsg, error)
}

func NewSimpleMiddleware(m SimpleMiddlewareInterface) SimpleMiddleware {
	return func(handler Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) (err error) {
				errCh := make(chan error, 2)

				defer func() { err = errors.Join(err, <-errCh, <-errCh, m.HandleStop(r)) }()

				r, err = m.HandleStart(r)
				if err != nil {
					return
				}

				ctx := r.Context()
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				r = r.WithContext(ctx)

				rCh := make(chan ClientMsg)
				sCh := make(chan ServerMsg)

				go func() {
					defer cancel()

					var err error
					defer func() { errCh <- err }()

					defer close(rCh)

					var ccmsg ClientMsg
					var smsgs []ServerMsg

					for {
						select {
						case <-ctx.Done():
							return

						case cmsg, ok := <-recv:
							if !ok {
								return
							}

							ccmsg, smsgs, err = m.HandleClientMsg(r, cmsg)
							if err != nil {
								return
							}
							sendClientMsgCtx(ctx, rCh, ccmsg)
							for _, smsg := range smsgs {
								sendServerMsgCtx(ctx, send, smsg)
							}
						}
					}
				}()

				go func() {
					defer cancel()

					var err error
					defer func() { errCh <- err }()

					var smsgs []ServerMsg

					for {
						select {
						case <-ctx.Done():
							return

						case smsg := <-sCh:
							smsgs, err = m.HandleServerMsg(r, smsg)
							if err != nil {
								return
							}

							for _, smsg := range smsgs {
								sendServerMsgCtx(ctx, send, smsg)
							}
						}
					}
				}()

				return handler.Handle(r, rCh, sCh)
			},
		)
	}
}

type EventCreatedAtFilterMiddleware Middleware

func NewEventCreatedAtFilterMiddleware(
	from, to time.Duration,
) EventCreatedAtFilterMiddleware {
	m := newSimpleEventCreatedAtFilterMiddleware(from, to)
	return EventCreatedAtFilterMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareInterface = (*simpleEventCreatedAtFilterMiddleware)(nil)

type simpleEventCreatedAtFilterMiddleware struct {
	from, to time.Duration
}

func newSimpleEventCreatedAtFilterMiddleware(
	from, to time.Duration,
) *simpleEventCreatedAtFilterMiddleware {
	return &simpleEventCreatedAtFilterMiddleware{from: from, to: to}
}

func (m *simpleEventCreatedAtFilterMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleEventCreatedAtFilterMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleEventCreatedAtFilterMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (ClientMsg, []ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		sub := time.Until(msg.Event.CreatedAtTime())
		if sub < m.from || m.to < -sub {
			okMsg := NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOKMsgPrefixNoPrefix,
				"too old created_at",
			)
			return nil, []ServerMsg{okMsg}, nil
		}
	}
	return msg, nil, nil
}

func (m *simpleEventCreatedAtFilterMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) ([]ServerMsg, error) {
	return []ServerMsg{msg}, nil
}

type RecvEventUniquefyMiddleware Middleware

func NewRecvEventUniquefyMiddleware(buflen int) RecvEventUniquefyMiddleware {
	if buflen < 0 {
		panic(
			fmt.Sprintf("RecvEventUniquefyMiddleware buflen must be 0 or more but got %v", buflen),
		)
	}

	m := newSimpleRecvEventUniquefyMiddleware(buflen)
	return RecvEventUniquefyMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareInterface = (*simpleRecvEventUniquefyMiddleware)(nil)

type simpleRecvEventUniquefyMiddleware struct {
	sema chan struct{}
	ids  *ringBuffer[string]
	seen map[string]bool
}

func newSimpleRecvEventUniquefyMiddleware(buflen int) *simpleRecvEventUniquefyMiddleware {
	return &simpleRecvEventUniquefyMiddleware{
		ids:  newRingBuffer[string](buflen),
		seen: make(map[string]bool),
		sema: make(chan struct{}, 1),
	}
}

func (m *simpleRecvEventUniquefyMiddleware) HandleStart(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (m *simpleRecvEventUniquefyMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleRecvEventUniquefyMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (ClientMsg, []ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		m.sema <- struct{}{}
		defer func() { <-m.sema }()

		if m.seen[msg.Event.ID] {
			okMsg := NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOKMsgPrefixDuplicate,
				"duplicated id",
			)
			return nil, []ServerMsg{okMsg}, nil
		}

		if m.ids.Len() == m.ids.Cap {
			old := m.ids.Dequeue()
			delete(m.seen, old)
		}

		m.ids.Enqueue(msg.Event.ID)
		m.seen[msg.Event.ID] = true
	}
	return msg, nil, nil
}

func (m *simpleRecvEventUniquefyMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) ([]ServerMsg, error) {
	return []ServerMsg{msg}, nil
}

type SendEventUniquefyMiddleware Middleware

func NewSendEventUniquefyMiddleware(buflen int) SendEventUniquefyMiddleware {
	if buflen < 0 {
		panic(
			fmt.Sprintf("SendEventUniquefyMiddleware buflen must be 0 or more but got %v", buflen),
		)
	}

	m := newSimpleSendEventUniquefyMiddleware(buflen)
	return SendEventUniquefyMiddleware(NewSimpleMiddleware(m))
}

type simpleSendEventUniquefyMiddleware struct {
	sema chan struct{}
	keys *ringBuffer[string]
	seen map[string]bool
}

func newSimpleSendEventUniquefyMiddleware(buflen int) *simpleSendEventUniquefyMiddleware {
	return &simpleSendEventUniquefyMiddleware{
		sema: make(chan struct{}, 1),
		keys: newRingBuffer[string](buflen),
		seen: make(map[string]bool),
	}
}

func (m *simpleSendEventUniquefyMiddleware) HandleStart(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (m *simpleSendEventUniquefyMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleSendEventUniquefyMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (ClientMsg, []ServerMsg, error) {
	return msg, nil, nil
}

func (m *simpleSendEventUniquefyMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) ([]ServerMsg, error) {
	if msg, ok := msg.(*ServerEventMsg); ok {
		k := m.key(msg.SubscriptionID, msg.Event.ID)

		m.sema <- struct{}{}
		defer func() { <-m.sema }()

		if m.seen[k] {
			return nil, nil
		}

		if m.keys.Len() == m.keys.Cap {
			old := m.keys.Dequeue()
			delete(m.seen, old)
		}
		m.keys.Enqueue(k)
		m.seen[k] = true
	}

	return []ServerMsg{msg}, nil
}

func (*simpleSendEventUniquefyMiddleware) key(subID, eventID string) string {
	return subID + ":" + eventID
}

type PrometheusMiddleware SimpleMiddleware

func NewPrometheusMiddleware(reg prometheus.Registerer) PrometheusMiddleware {
	m := newSimplePrometheusMiddleware(reg)
	return PrometheusMiddleware(NewSimpleMiddleware(m))
}

type simplePrometheusMiddleware struct {
	connectionCount prometheus.Gauge
	recvMsgTotal    *prometheus.CounterVec
	recvEventTotal  *prometheus.CounterVec
	sendMsgTotal    *prometheus.CounterVec
}

func newSimplePrometheusMiddleware(reg prometheus.Registerer) *simplePrometheusMiddleware {
	m := &simplePrometheusMiddleware{
		connectionCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "connection_count",
			Help: "Current websocket connection count.",
		}),
		recvMsgTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "recv_msg_total",
				Help: "Number of received client messages.",
			},
			[]string{"type"},
		),
		recvEventTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "recv_event_total",
				Help: "Number of received client messages.",
			},
			[]string{"kind"},
		),
		sendMsgTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "send_msg_total",
				Help: "Number of sent server messages.",
			},
			[]string{"type"},
		),
	}

	reg.MustRegister(m.connectionCount)
	reg.MustRegister(m.recvMsgTotal)
	reg.MustRegister(m.recvEventTotal)
	reg.MustRegister(m.sendMsgTotal)

	return m
}

func (m *simplePrometheusMiddleware) HandleStart(r *http.Request) (*http.Request, error) {
	m.connectionCount.Inc()
	return r, nil
}

func (m *simplePrometheusMiddleware) HandleStop(r *http.Request) error {
	m.connectionCount.Dec()
	return nil
}

func (m *simplePrometheusMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (ClientMsg, []ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientUnknownMsg:
		m.recvMsgTotal.WithLabelValues("UNKNOWN").Inc()

	case *ClientEventMsg:
		m.recvMsgTotal.WithLabelValues("EVENT").Inc()
		k := strconv.FormatInt(msg.Event.Kind, 10)
		m.recvEventTotal.WithLabelValues(k).Inc()

	case *ClientReqMsg:
		m.recvMsgTotal.WithLabelValues("REQ").Inc()

	case *ClientCloseMsg:
		m.recvMsgTotal.WithLabelValues("CLOSE").Inc()

	case *ClientAuthMsg:
		m.recvMsgTotal.WithLabelValues("AUTH").Inc()

	case *ClientCountMsg:
		m.recvMsgTotal.WithLabelValues("COUNT").Inc()

	default:
		m.recvMsgTotal.WithLabelValues("UNDEFINED").Inc()
	}

	return msg, nil, nil
}

func (m *simplePrometheusMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) ([]ServerMsg, error) {
	switch msg.(type) {
	case *ServerEventMsg:
		m.sendMsgTotal.WithLabelValues("EVENT").Inc()

	case *ServerNoticeMsg:
		m.sendMsgTotal.WithLabelValues("NOTICE").Inc()

	case *ServerOKMsg:
		m.sendMsgTotal.WithLabelValues("OK").Inc()

	case *ServerAuthMsg:
		m.sendMsgTotal.WithLabelValues("AUTH").Inc()

	case *ServerCountMsg:
		m.sendMsgTotal.WithLabelValues("COUNT").Inc()

	default:
		m.sendMsgTotal.WithLabelValues("UNDEFINED").Inc()
	}

	return []ServerMsg{msg}, nil
}
