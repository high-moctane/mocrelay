package mocrelay

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
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
	HandleClientMsg(*http.Request, ClientMsg) (<-chan ServerMsg, error)
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
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			smsgChCh := make(chan (<-chan ServerMsg))
			errs := make(chan error, 1)

			var wg sync.WaitGroup

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()
				defer close(smsgChCh)

				errs <- func() error {
					for cmsg := range recvCtx(ctx, recv) {
						smsgCh, err := h.HandleClientMsg(r, cmsg)
						if err != nil {
							return err
						}
						if smsgCh == nil {
							continue
						}
						sendCtx(ctx, smsgChCh, smsgCh)
					}

					if err := ctx.Err(); err != nil {
						return err
					}
					return ErrRecvClosed
				}()
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel()

				for smsgCh := range recvCtx(ctx, smsgChCh) {
					for smsg := range recvCtx(ctx, smsgCh) {
						sendServerMsgCtx(ctx, send, smsg)
					}
				}
			}()

			wg.Wait()

			close(errs)
			for e := range errs {
				err = errors.Join(err, e)
			}

			return
		},
	)
}

var ErrRouterHandlerStop = errors.New("router handler stopped")

type RouterHandler struct {
	buflen int
	subs   *subscribers
}

func NewRouterHandler(buflen int) *RouterHandler {
	if buflen <= 0 {
		panicf("router handler buflen must be a positive integer but got %d", buflen)
	}
	return &RouterHandler{
		buflen: buflen,
		subs:   newSubscribers(),
	}
}

func (router *RouterHandler) Handle(
	r *http.Request,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) (err error) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	reqID := GetRequestID(ctx)
	defer router.subs.UnsubscribeAll(reqID)

	subCh := make(chan ServerMsg, router.buflen)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for msg := range recvCtx(ctx, recv) {
			m := router.recv(ctx, reqID, msg, subCh)
			sendServerMsgCtx(ctx, send, m)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for msg := range recvCtx(ctx, subCh) {
			sendCtx(ctx, send, msg)
		}
	}()

	wg.Wait()
	return errors.Join(ErrRouterHandlerStop, ctx.Err())
}

func (router *RouterHandler) recv(
	ctx context.Context,
	reqID string,
	msg ClientMsg,
	subCh chan ServerMsg,
) ServerMsg {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		sub := newSubscriber(reqID, msg, subCh)
		router.subs.Subscribe(sub)
		return NewServerEOSEMsg(msg.SubscriptionID)

	case *ClientEventMsg:
		router.subs.Publish(msg.Event)
		return NewServerOKMsg(msg.Event.ID, true, ServerOKMsgPrefixNoPrefix, "")

	case *ClientCloseMsg:
		router.subs.Unsubscribe(reqID, msg.SubscriptionID)
		return nil

	case *ClientCountMsg:
		return NewServerCountMsg(msg.SubscriptionID, 0, nil)

	default:
		return nil
	}
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
	subs *safeMap[string, *safeMap[string, *subscriber]]
}

func newSubscribers() *subscribers {
	return &subscribers{
		subs: newSafeMap[string, *safeMap[string, *subscriber]](),
	}
}

func (subs *subscribers) Subscribe(sub *subscriber) {
	m, ok := subs.subs.TryGet(sub.ReqID)
	if !ok {
		m = newSafeMap[string, *subscriber]()
		subs.subs.Add(sub.ReqID, m)
	}

	m.Add(sub.SubscriptionID, sub)
}

func (subs *subscribers) Unsubscribe(reqID, subID string) {
	m, ok := subs.subs.TryGet(reqID)
	if !ok {
		return
	}
	m.Delete(subID)
}

func (subs *subscribers) UnsubscribeAll(reqID string) {
	subs.subs.Delete(reqID)
}

func (subs *subscribers) Publish(event *Event) {
	subs.subs.Loop(func(_ string, m *safeMap[string, *subscriber]) {
		m.Loop(func(_ string, mm *subscriber) {
			mm.SendIfMatch(event)
		})
	})
}

type CacheHandler SimpleHandler

func NewCacheHandler(size int) CacheHandler {
	return CacheHandler(NewSimpleHandler(newSimpleCacheHandler(size)))
}

type simpleCacheHandler struct {
	c *eventCache
}

func newSimpleCacheHandler(size int) *simpleCacheHandler {
	return &simpleCacheHandler{
		c: newEventCache(size),
	}
}

func (h *simpleCacheHandler) HandleStart(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (h *simpleCacheHandler) HandleStop(r *http.Request) error {
	return nil
}

func (h *simpleCacheHandler) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientEventMsg:
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

		var okMsg ServerMsg
		if h.c.Add(ev) {
			okMsg = NewServerOKMsg(msg.Event.ID, true, "", "")
		} else {
			okMsg = NewServerOKMsg(msg.Event.ID, false, ServerOKMsgPrefixDuplicate, "already have this event")
		}
		return newClosedBufCh(okMsg), nil

	case *ClientReqMsg:
		evs := h.c.Find(NewReqFiltersEventMatchers(msg.ReqFilters))

		smsgCh := make(chan ServerMsg, len(evs)+1)
		defer close(smsgCh)

		for _, ev := range evs {
			smsgCh <- NewServerEventMsg(msg.SubscriptionID, ev)
		}
		smsgCh <- NewServerEOSEMsg(msg.SubscriptionID)
		return smsgCh, nil

	case *ClientCountMsg:
		ret := NewServerCountMsg(msg.SubscriptionID, 0, nil)
		return newClosedBufCh[ServerMsg](ret), nil

	default:
		return nil, nil
	}
}

type MergeHandler struct {
	hs []Handler
}

func NewMergeHandler(handlers ...Handler) Handler {
	if len(handlers) < 2 {
		panicf("handlers must be two or more but got %d", len(handlers))
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

	okStat    chan *mergeHandlerSessionOKState
	reqStat   chan *mergeHandlerSessionReqState
	countStat chan *mergeHandlerSessionCountState
}

func newMergeHandlerSession(h *MergeHandler) *mergeHandlerSession {
	size := len(h.hs)

	recvs := make([]chan ClientMsg, size)
	sends := make([]chan ServerMsg, size)
	for i := 0; i < len(h.hs); i++ {
		recvs[i] = make(chan ClientMsg)
		sends[i] = make(chan ServerMsg)
	}

	ss := &mergeHandlerSession{
		h:         h,
		recvs:     recvs,
		sends:     sends,
		preSendCh: make(chan *mergeHandlerSessionSendMsg),
		okStat:    make(chan *mergeHandlerSessionOKState, 1),
		reqStat:   make(chan *mergeHandlerSessionReqState, 1),
		countStat: make(chan *mergeHandlerSessionCountState, 1),
	}

	ss.okStat <- newMergeHandlerSessionOKState(size)
	ss.reqStat <- newMergeHandlerSessionReqState(size)
	ss.countStat <- newMergeHandlerSessionCountState(size)

	return ss
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
		ss.handleRecv(ctx, recv)
	}()
	go func() {
		defer cancel()
		ss.handleSend(ctx, send)
	}()

	return ss.runHandlers(r)
}

func (ss *mergeHandlerSession) runHandlers(r *http.Request) error {
	hs := ss.h.hs
	var wg sync.WaitGroup
	errs := make(chan error, len(hs))

	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r = r.WithContext(ctx)

	wg.Add(len(hs))
	for i := 0; i < len(ss.h.hs); i++ {
		go func(i int) {
			defer wg.Done()
			defer cancel()
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

	for msg := range recvCtx(ctx, sends[idx]) {
		sendCtx(ctx, ss.preSendCh, newMergeHandlerSessionSendMsg(idx, msg))
	}
}

func (ss *mergeHandlerSession) closeRecvs() {
	for _, r := range ss.recvs {
		close(r)
	}
}

func (ss *mergeHandlerSession) handleRecv(ctx context.Context, recv <-chan ClientMsg) {
	for msg := range recvCtx(ctx, recv) {
		msg = ss.handleRecvMsg(msg)
		ss.broadcastRecvs(ctx, msg)
	}
}

func (ss *mergeHandlerSession) handleRecvMsg(msg ClientMsg) ClientMsg {
	switch msg := msg.(type) {
	case *ClientEventMsg:
		return ss.handleRecvEventMsg(msg)
	case *ClientReqMsg:
		return ss.handleRecvReqMsg(msg)
	case *ClientCloseMsg:
		return ss.handleRecvCloseMsg(msg)
	case *ClientCountMsg:
		return ss.handleRecvCountMsg(msg)
	default:
		return msg
	}
}

func (ss *mergeHandlerSession) handleRecvEventMsg(msg *ClientEventMsg) ClientMsg {
	s := <-ss.okStat
	defer func() { ss.okStat <- s }()
	s.TrySetEventID(msg.Event.ID)
	return msg
}

func (ss *mergeHandlerSession) handleRecvReqMsg(msg *ClientReqMsg) ClientMsg {
	s := <-ss.reqStat
	defer func() { ss.reqStat <- s }()
	s.SetSubID(msg.SubscriptionID)
	return msg
}

func (ss *mergeHandlerSession) handleRecvCloseMsg(msg *ClientCloseMsg) ClientMsg {
	s := <-ss.reqStat
	defer func() { ss.reqStat <- s }()
	s.ClearSubID(msg.SubscriptionID)
	return msg
}

func (ss *mergeHandlerSession) handleRecvCountMsg(msg *ClientCountMsg) ClientMsg {
	s := <-ss.countStat
	defer func() { ss.countStat <- s }()
	s.SetSubID(msg.SubscriptionID)
	return msg
}

func (ss *mergeHandlerSession) broadcastRecvs(ctx context.Context, msg ClientMsg) {
	for _, r := range ss.recvs {
		sendClientMsgCtx(ctx, r, msg)
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

func (ss *mergeHandlerSession) handleSend(ctx context.Context, send chan<- ServerMsg) {
	for msg := range recvCtx(ctx, ss.preSendCh) {
		m := ss.handleSendMsg(msg)
		sendServerMsgCtx(ctx, send, m)
	}
}

func (ss *mergeHandlerSession) handleSendMsg(msg *mergeHandlerSessionSendMsg) ServerMsg {
	switch msg.Msg.(type) {
	case *ServerEOSEMsg:
		return ss.handleSendEOSEMsg(msg)
	case *ServerEventMsg:
		return ss.handleSendEventMsg(msg)
	case *ServerOKMsg:
		return ss.handleSendOKMsg(msg)
	case *ServerCountMsg:
		return ss.handleSendCountMsg(msg)
	default:
		return msg.Msg
	}
}

func (ss *mergeHandlerSession) handleSendEOSEMsg(msg *mergeHandlerSessionSendMsg) *ServerEOSEMsg {
	m := msg.Msg.(*ServerEOSEMsg)

	s := <-ss.reqStat
	defer func() { ss.reqStat <- s }()

	if s.AllEOSE(m.SubscriptionID) {
		return nil
	}
	s.SetEOSE(m.SubscriptionID, msg.Idx)
	if !s.AllEOSE(m.SubscriptionID) {
		return nil
	}

	return m
}

func (ss *mergeHandlerSession) handleSendEventMsg(msg *mergeHandlerSessionSendMsg) *ServerEventMsg {
	m := msg.Msg.(*ServerEventMsg)

	s := <-ss.reqStat
	defer func() { ss.reqStat <- s }()

	if !s.IsSendableEventMsg(msg.Idx, m) {
		return nil
	}

	return m
}

func (ss *mergeHandlerSession) handleSendOKMsg(msg *mergeHandlerSessionSendMsg) *ServerOKMsg {
	m := msg.Msg.(*ServerOKMsg)

	s := <-ss.okStat
	defer func() { ss.okStat <- s }()

	s.SetMsg(msg.Idx, m)
	if !s.Ready(m.EventID) {
		return nil
	}

	ret := s.Msg(m.EventID)
	s.ClearEventID(m.EventID)

	return ret
}

func (ss *mergeHandlerSession) handleSendCountMsg(msg *mergeHandlerSessionSendMsg) *ServerCountMsg {
	m := msg.Msg.(*ServerCountMsg)

	s := <-ss.countStat
	defer func() { ss.countStat <- s }()

	s.SetCountMsg(msg.Idx, m)
	if !s.Ready(m.SubscriptionID, msg.Idx) {
		return nil
	}

	ret := s.Msg(m.SubscriptionID)
	s.ClearSubID(m.SubscriptionID)

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
		panicf("invalid eventID %s", eventID)
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
	stat.lastEvent[subID] = nil
	stat.seen[subID] = make(map[string]bool)
}

func (stat *mergeHandlerSessionReqState) SetEOSE(subID string, chIdx int) {
	if len(stat.eose[subID]) == 0 {
		return
	}
	stat.eose[subID][chIdx] = true
}

func (stat *mergeHandlerSessionReqState) AllEOSE(subID string) bool {
	eoses, ok := stat.eose[subID]
	if !ok {
		return true
	}

	res := !slices.Contains(eoses, false)
	if res {
		delete(stat.eose, subID)
		delete(stat.lastEvent, subID)
		delete(stat.seen, subID)
	}
	return res
}

func (stat *mergeHandlerSessionReqState) IsEOSE(subID string, chIdx int) bool {
	eoses := stat.eose[subID]
	return len(eoses) == 0 || eoses[chIdx]
}

func (stat *mergeHandlerSessionReqState) IsSendableEventMsg(
	chIdx int,
	msg *ServerEventMsg,
) bool {
	if stat.AllEOSE(msg.SubscriptionID) {
		return true
	}

	if stat.IsEOSE(msg.SubscriptionID, chIdx) {
		return false
	}

	if last := stat.lastEvent[msg.SubscriptionID]; last != nil {
		if res := cmp.Compare(last.Event.CreatedAt, msg.Event.CreatedAt); res < 0 {
			return false
		} else if res > 0 {
			stat.seen[msg.SubscriptionID] = make(map[string]bool)
		}
	}
	stat.lastEvent[msg.SubscriptionID] = msg

	if stat.seen[msg.SubscriptionID] == nil || stat.seen[msg.SubscriptionID][msg.Event.ID] {
		return false
	}
	stat.seen[msg.SubscriptionID][msg.Event.ID] = true

	return true
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
	HandleClientMsg(*http.Request, ClientMsg) (<-chan ClientMsg, <-chan ServerMsg, error)
	HandleServerMsg(*http.Request, ServerMsg) (<-chan ServerMsg, error)
}

func NewSimpleMiddleware(m SimpleMiddlewareInterface) SimpleMiddleware {
	return func(handler Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) (err error) {
				defer func() { err = errors.Join(err, m.HandleStop(r)) }()

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

				errs := make(chan error, 3)

				var wg sync.WaitGroup

				wg.Add(1)
				go func() {
					defer wg.Done()
					defer cancel()
					defer close(rCh)

					errs <- func() error {
						for cmsg := range recvCtx(ctx, recv) {
							cmsgCh, smsgCh, err := m.HandleClientMsg(r, cmsg)
							if err != nil {
								return err
							}
							if cmsgCh != nil {
								for cmsg := range recvCtx(ctx, cmsgCh) {
									sendClientMsgCtx(ctx, rCh, cmsg)
								}
							}
							if smsgCh != nil {
								for smsg := range recvCtx(ctx, smsgCh) {
									sendServerMsgCtx(ctx, send, smsg)
								}
							}
						}

						if err := ctx.Err(); err != nil {
							return err
						}
						return ErrRecvClosed
					}()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					defer cancel()

					errs <- func() error {
						for smsg := range recvCtx(ctx, sCh) {
							smsgCh, err := m.HandleServerMsg(r, smsg)
							if err != nil {
								return err
							}
							if smsgCh == nil {
								continue
							}

							for smsg := range recvCtx(ctx, smsgCh) {
								sendServerMsgCtx(ctx, send, smsg)
							}
						}

						return ctx.Err()
					}()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					defer cancel()
					errs <- handler.Handle(r, rCh, sCh)
				}()

				wg.Wait()

				close(errs)
				for e := range errs {
					err = errors.Join(err, e)
				}

				return
			},
		)
	}
}

func BuildMiddlewareFromNIP11(nip11 *NIP11) Middleware {
	if nip11 == nil {
		return nil
	}

	return func(h Handler) Handler {
		if v := nip11.Limitation.MaxSubscriptions; v != 0 {
			h = NewMaxSubscriptionsMiddleware(v)(h)
		}
		if v := nip11.Limitation.MaxFilters; v != 0 {
			h = NewMaxReqFiltersMiddleware(v)(h)
		}
		if v := nip11.Limitation.MaxLimit; v != 0 {
			h = NewMaxLimitMiddleware(v)(h)
		}
		if v := nip11.Limitation.MaxEventTags; v != 0 {
			h = NewMaxEventTagsMiddleware(v)(h)
		}
		if v := nip11.Limitation.MaxContentLength; v != 0 {
			h = NewMaxContentLengthMiddleware(v)(h)
		}
		if v := nip11.Limitation.CreatedAtLowerLimit; v != 0 {
			h = NewCreatedAtLowerLimitMiddleware(v)(h)
		}
		if v := nip11.Limitation.CreatedAtUpperLimit; v != 0 {
			h = NewCreatedAtUpperLimitMiddleware(v)(h)
		}

		return h
	}
}

type EventCreatedAtMiddleware Middleware

func NewEventCreatedAtMiddleware(
	from, to time.Duration,
) EventCreatedAtMiddleware {
	m := newSimpleEventCreatedAtMiddleware(from, to)
	return EventCreatedAtMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareInterface = (*simpleEventCreatedAtMiddleware)(nil)

type simpleEventCreatedAtMiddleware struct {
	from, to time.Duration
}

func newSimpleEventCreatedAtMiddleware(
	from, to time.Duration,
) *simpleEventCreatedAtMiddleware {
	return &simpleEventCreatedAtMiddleware{from: from, to: to}
}

func (m *simpleEventCreatedAtMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleEventCreatedAtMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleEventCreatedAtMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		sub := time.Until(msg.Event.CreatedAtTime())
		if sub < m.from {
			smsgCh := newClosedBufCh[ServerMsg](NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOKMsgPrefixNoPrefix,
				"too old created_at",
			))
			return nil, smsgCh, nil
		} else if m.to < sub {
			smsgCh := newClosedBufCh[ServerMsg](NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOKMsgPrefixNoPrefix,
				"too far off created_at",
			))
			return nil, smsgCh, nil
		}
	}

	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleEventCreatedAtMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type MaxSubscriptionsMiddleware Middleware

func NewMaxSubscriptionsMiddleware(maxSubs int) MaxSubscriptionsMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				sm := newSimpleMaxSubscriptionsMiddleware(maxSubs)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(r, recv, send)
			},
		)
	}
}

type simpleMaxSubscriptionsMiddleware struct {
	maxSubs int

	// map[subID]exist
	subs map[string]bool
}

func newSimpleMaxSubscriptionsMiddleware(
	maxSubs int,
) *simpleMaxSubscriptionsMiddleware {
	if maxSubs < 1 {
		panicf("max subscriptions must be a positive integer but got %d", maxSubs)
	}
	return &simpleMaxSubscriptionsMiddleware{
		maxSubs: maxSubs,
		subs:    make(map[string]bool, maxSubs+1),
	}
}

func (m *simpleMaxSubscriptionsMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleMaxSubscriptionsMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleMaxSubscriptionsMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		m.subs[msg.SubscriptionID] = true
		if len(m.subs) > m.maxSubs {
			delete(m.subs, msg.SubscriptionID)
			notice := NewServerNoticeMsgf("too many req: %s: max subscriptions is %d", msg.SubscriptionID, m.maxSubs)
			return nil, newClosedBufCh[ServerMsg](notice), nil
		}

	case *ClientCloseMsg:
		delete(m.subs, msg.SubscriptionID)
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *simpleMaxSubscriptionsMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxReqFiltersMiddleware Middleware

func NewMaxReqFiltersMiddleware(maxFilters int) MaxReqFiltersMiddleware {
	return MaxReqFiltersMiddleware(
		NewSimpleMiddleware(newSimpleMaxReqFiltersMiddleware(maxFilters)),
	)
}

var _ SimpleMiddlewareInterface = (*simpleMaxReqFiltersMiddleware)(nil)

type simpleMaxReqFiltersMiddleware struct {
	maxFilters int
}

func newSimpleMaxReqFiltersMiddleware(
	maxFilters int,
) *simpleMaxReqFiltersMiddleware {
	if maxFilters < 1 {
		panicf("max subscriptions must be a positive integer but got %d", maxFilters)
	}
	return &simpleMaxReqFiltersMiddleware{maxFilters: maxFilters}
}

func (m *simpleMaxReqFiltersMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleMaxReqFiltersMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleMaxReqFiltersMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		if len(msg.ReqFilters) > m.maxFilters {
			notice := NewServerNoticeMsgf("too many req filters: %s: max filters is %d", msg.SubscriptionID, m.maxFilters)
			return nil, newClosedBufCh[ServerMsg](notice), nil
		}

	case *ClientCountMsg:
		if len(msg.ReqFilters) > m.maxFilters {
			notice := NewServerNoticeMsgf("too many count filters: %s: max filters is %d", msg.SubscriptionID, m.maxFilters)
			return nil, newClosedBufCh[ServerMsg](notice), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *simpleMaxReqFiltersMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxLimitMiddleware Middleware

func NewMaxLimitMiddleware(maxLimit int) MaxLimitMiddleware {
	return MaxLimitMiddleware(
		NewSimpleMiddleware(newSimpleMaxLimitMiddleware(maxLimit)),
	)
}

var _ SimpleMiddlewareInterface = (*simpleMaxLimitMiddleware)(nil)

type simpleMaxLimitMiddleware struct {
	maxLimit int
}

func newSimpleMaxLimitMiddleware(
	maxLimit int,
) *simpleMaxLimitMiddleware {
	if maxLimit < 1 {
		panicf("max limit must be a positive integer but got %d", maxLimit)
	}
	return &simpleMaxLimitMiddleware{maxLimit: maxLimit}
}

func (m *simpleMaxLimitMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleMaxLimitMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleMaxLimitMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		found := slices.ContainsFunc(msg.ReqFilters, func(f *ReqFilter) bool { return f.Limit != nil && *f.Limit > int64(m.maxLimit) })
		if found {
			notice := NewServerNoticeMsgf("too large limit: %s: max limit is %d", msg.SubscriptionID, m.maxLimit)
			return nil, newClosedBufCh[ServerMsg](notice), nil
		}

	case *ClientCountMsg:
		found := slices.ContainsFunc(msg.ReqFilters, func(f *ReqFilter) bool { return f.Limit != nil && *f.Limit > int64(m.maxLimit) })
		if found {
			notice := NewServerNoticeMsgf("too large limit: %s: max limit is %d", msg.SubscriptionID, m.maxLimit)
			return nil, newClosedBufCh[ServerMsg](notice), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *simpleMaxLimitMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxSubIDLengthMiddleware Middleware

func NewMaxSubIDLengthMiddleware(maxSubIDLength int) MaxSubIDLengthMiddleware {
	return MaxSubIDLengthMiddleware(
		NewSimpleMiddleware(newSimpleMaxSubIDLengthMiddleware(maxSubIDLength)),
	)
}

var _ SimpleMiddlewareInterface = (*simpleMaxSubIDLengthMiddleware)(nil)

type simpleMaxSubIDLengthMiddleware struct {
	maxSubIDLength int
}

func newSimpleMaxSubIDLengthMiddleware(
	maxSubIDLength int,
) *simpleMaxSubIDLengthMiddleware {
	if maxSubIDLength < 1 {
		panicf("max subid length must be a positive integer but got %d", maxSubIDLength)
	}
	return &simpleMaxSubIDLengthMiddleware{maxSubIDLength: maxSubIDLength}
}

func (m *simpleMaxSubIDLengthMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleMaxSubIDLengthMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleMaxSubIDLengthMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		if len(msg.SubscriptionID) > m.maxSubIDLength {
			notice := NewServerNoticeMsgf("too long subid: %s: max subid length is %d", msg.SubscriptionID, m.maxSubIDLength)
			return nil, newClosedBufCh[ServerMsg](notice), nil
		}

	case *ClientCountMsg:
		if len(msg.SubscriptionID) > m.maxSubIDLength {
			notice := NewServerNoticeMsgf("too long subid: %s: max subid length is %d", msg.SubscriptionID, m.maxSubIDLength)
			return nil, newClosedBufCh[ServerMsg](notice), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *simpleMaxSubIDLengthMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxEventTagsMiddleware Middleware

func NewMaxEventTagsMiddleware(maxEventTags int) MaxEventTagsMiddleware {
	return MaxEventTagsMiddleware(
		NewSimpleMiddleware(newSimpleMaxEventTagsMiddleware(maxEventTags)),
	)
}

var _ SimpleMiddlewareInterface = (*simpleMaxEventTagsMiddleware)(nil)

type simpleMaxEventTagsMiddleware struct {
	maxEventTags int
}

func newSimpleMaxEventTagsMiddleware(
	maxEventTags int,
) *simpleMaxEventTagsMiddleware {
	if maxEventTags < 1 {
		panicf("max event tags must be a positive integer but got %d", maxEventTags)
	}
	return &simpleMaxEventTagsMiddleware{maxEventTags: maxEventTags}
}

func (m *simpleMaxEventTagsMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleMaxEventTagsMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleMaxEventTagsMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		if len(msg.Event.Tags) > m.maxEventTags {
			okMsg := NewServerOKMsg(
				msg.Event.ID,
				false,
				"",
				fmt.Sprintf("too many event tags: max event tags is %d", m.maxEventTags),
			)
			return nil, newClosedBufCh[ServerMsg](okMsg), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *simpleMaxEventTagsMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxContentLengthMiddleware Middleware

func NewMaxContentLengthMiddleware(maxContentLength int) MaxContentLengthMiddleware {
	return MaxContentLengthMiddleware(
		NewSimpleMiddleware(newSimpleMaxContentLengthMiddleware(maxContentLength)),
	)
}

var _ SimpleMiddlewareInterface = (*simpleMaxContentLengthMiddleware)(nil)

type simpleMaxContentLengthMiddleware struct {
	maxContentLength int
}

func newSimpleMaxContentLengthMiddleware(
	maxContentLength int,
) *simpleMaxContentLengthMiddleware {
	if maxContentLength < 1 {
		panicf("max content length must be a positive integer but got %d", maxContentLength)
	}
	return &simpleMaxContentLengthMiddleware{maxContentLength: maxContentLength}
}

func (m *simpleMaxContentLengthMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleMaxContentLengthMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleMaxContentLengthMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		if len(msg.Event.Content) > m.maxContentLength {
			okMsg := NewServerOKMsg(
				msg.Event.ID,
				false,
				"",
				fmt.Sprintf("too long content: max content length is %d", m.maxContentLength),
			)
			return nil, newClosedBufCh[ServerMsg](okMsg), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *simpleMaxContentLengthMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type CreatedAtLowerLimitMiddleware Middleware

func NewCreatedAtLowerLimitMiddleware(lower int64) CreatedAtLowerLimitMiddleware {
	m := newSimpleCreatedAtLowerLimitMiddleware(lower)
	return CreatedAtLowerLimitMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareInterface = (*simpleCreatedAtLowerLimitMiddleware)(nil)

type simpleCreatedAtLowerLimitMiddleware struct {
	lower int64
}

func newSimpleCreatedAtLowerLimitMiddleware(lower int64) *simpleCreatedAtLowerLimitMiddleware {
	return &simpleCreatedAtLowerLimitMiddleware{lower: lower}
}

func (m *simpleCreatedAtLowerLimitMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleCreatedAtLowerLimitMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleCreatedAtLowerLimitMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		if time.Since(msg.Event.CreatedAtTime()) > time.Duration(m.lower)*time.Second {
			smsgCh := newClosedBufCh[ServerMsg](NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOKMsgPrefixNoPrefix,
				"too old created_at",
			))
			return nil, smsgCh, nil
		}
	}

	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleCreatedAtLowerLimitMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type CreatedAtUpperLimitMiddleware Middleware

func NewCreatedAtUpperLimitMiddleware(upper int64) CreatedAtUpperLimitMiddleware {
	m := newSimpleCreatedAtUpperLimitMiddleware(upper)
	return CreatedAtUpperLimitMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareInterface = (*simpleCreatedAtUpperLimitMiddleware)(nil)

type simpleCreatedAtUpperLimitMiddleware struct {
	upper int64
}

func newSimpleCreatedAtUpperLimitMiddleware(upper int64) *simpleCreatedAtUpperLimitMiddleware {
	return &simpleCreatedAtUpperLimitMiddleware{upper: upper}
}

func (m *simpleCreatedAtUpperLimitMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleCreatedAtUpperLimitMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleCreatedAtUpperLimitMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		if time.Until(msg.Event.CreatedAtTime()) > time.Duration(m.upper)*time.Second {
			smsgCh := newClosedBufCh[ServerMsg](NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOKMsgPrefixNoPrefix,
				"too far off created_at",
			))
			return nil, smsgCh, nil
		}
	}

	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleCreatedAtUpperLimitMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type RecvEventUniqueFilterMiddleware Middleware

func NewRecvEventUniqueFilterMiddleware(size int) RecvEventUniqueFilterMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				sm := newSimpleRecvEventUniqueFilterMiddleware(size)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(r, recv, send)
			},
		)
	}
}

var _ SimpleMiddlewareInterface = (*simpleRecvEventUniqueFilterMiddleware)(nil)

type simpleRecvEventUniqueFilterMiddleware struct {
	c *randCache[string, struct{}]
}

func newSimpleRecvEventUniqueFilterMiddleware(size int) *simpleRecvEventUniqueFilterMiddleware {
	return &simpleRecvEventUniqueFilterMiddleware{
		c: newRandCache[string, struct{}](size),
	}
}

func (m *simpleRecvEventUniqueFilterMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleRecvEventUniqueFilterMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleRecvEventUniqueFilterMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		if _, found := m.c.Get(msg.Event.ID); found {
			okMsg := NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOKMsgPrefixDuplicate,
				"the event already found",
			)
			return nil, newClosedBufCh[ServerMsg](okMsg), nil
		}
		m.c.Set(msg.Event.ID, struct{}{})
	}

	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleRecvEventUniqueFilterMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type SendEventUniqueFilterMiddleware Middleware

func NewSendEventUniqueFilterMiddleware(size int) SendEventUniqueFilterMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				sm := newSimpleSendEventUniqueFilterMiddleware(size)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(r, recv, send)
			},
		)
	}
}

var _ SimpleMiddlewareInterface = (*simpleSendEventUniqueFilterMiddleware)(nil)

type simpleSendEventUniqueFilterMiddleware struct {
	c *randCache[string, struct{}]
}

func newSimpleSendEventUniqueFilterMiddleware(size int) *simpleSendEventUniqueFilterMiddleware {
	return &simpleSendEventUniqueFilterMiddleware{
		c: newRandCache[string, struct{}](size),
	}
}

func (m *simpleSendEventUniqueFilterMiddleware) HandleStart(
	r *http.Request,
) (*http.Request, error) {
	return r, nil
}

func (m *simpleSendEventUniqueFilterMiddleware) HandleStop(r *http.Request) error {
	return nil
}

func (m *simpleSendEventUniqueFilterMiddleware) HandleClientMsg(
	r *http.Request,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleSendEventUniqueFilterMiddleware) HandleServerMsg(
	r *http.Request,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	if msg, ok := msg.(*ServerEventMsg); ok {
		if _, found := m.c.Get(msg.Event.ID); found {
			return nil, nil
		}
		m.c.Set(msg.Event.ID, struct{}{})
	}

	return newClosedBufCh[ServerMsg](msg), nil
}
