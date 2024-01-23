package mocrelay

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRecvClosed = errors.New("recv closed")
)

type Handler interface {
	Handle(ctx context.Context, recv <-chan ClientMsg, send chan<- ServerMsg) error
}

type HandlerFunc func(ctx context.Context, recv <-chan ClientMsg, send chan<- ServerMsg) error

func (f HandlerFunc) Handle(
	ctx context.Context,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	return f(ctx, recv, send)
}

type SimpleHandler Handler

type SimpleHandlerInterface interface {
	HandleStart(context.Context) (context.Context, error)
	HandleStop(context.Context) error
	HandleClientMsg(context.Context, ClientMsg) (<-chan ServerMsg, error)
}

func NewSimpleHandler(h SimpleHandlerInterface) SimpleHandler {
	return HandlerFunc(
		func(ctx context.Context, recv <-chan ClientMsg, send chan<- ServerMsg) (err error) {
			defer func() { err = errors.Join(err, h.HandleStop(ctx)) }()

			ctx, err = h.HandleStart(ctx)
			if err != nil {
				return
			}

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()

				case cmsg, ok := <-recv:
					if !ok {
						return ErrRecvClosed
					}
					smsgCh, err := h.HandleClientMsg(ctx, cmsg)
					if err != nil {
						return err
					}
					if smsgCh != nil {
					L:
						for {
							select {
							case <-ctx.Done():
								return ctx.Err()

							case smsg, ok := <-smsgCh:
								if !ok {
									break L
								}
								sendServerMsgCtx(ctx, send, smsg)
							}
						}
					}
				}
			}
		},
	)
}

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
	ctx context.Context,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	reqID := uuid.NewString()
	defer router.subs.UnsubscribeAll(reqID)

	subCh := make(chan ServerMsg, router.buflen)

	go func() {
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return

			case msg := <-subCh:
				sendCtx(ctx, send, msg)
			}
		}
	}()

	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				return ErrRecvClosed
			}
			m := router.recv(ctx, reqID, msg, subCh)
			sendServerMsgCtx(ctx, send, m)
		}
	}
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

type CacheHandler struct {
	h *simpleCacheHandler
}

func NewCacheHandler(size int) CacheHandler {
	return CacheHandler{h: newSimpleCacheHandler(size)}
}

func (h CacheHandler) Handle(
	ctx context.Context,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	return NewSimpleHandler(h.h).Handle(ctx, recv, send)
}

func (h CacheHandler) Dump(w io.Writer) error {
	return h.h.Dump(w)
}

func (h CacheHandler) Restore(r io.Reader) error {
	return h.h.Restore(r)
}

type simpleCacheHandler struct {
	c *EventCache
}

func newSimpleCacheHandler(size int) *simpleCacheHandler {
	return &simpleCacheHandler{
		c: NewEventCache(size),
	}
}

func (h *simpleCacheHandler) HandleStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (h *simpleCacheHandler) HandleStop(ctx context.Context) error {
	return nil
}

func (h *simpleCacheHandler) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientEventMsg:
		ev := msg.Event

		var okMsg ServerMsg
		if h.c.Add(ev) {
			okMsg = NewServerOKMsg(msg.Event.ID, true, "", "")
		} else {
			okMsg = NewServerOKMsg(msg.Event.ID, false, ServerOKMsgPrefixDuplicate, "already have this event")
		}
		return newClosedBufCh(okMsg), nil

	case *ClientReqMsg:
		evs := h.c.Find(msg.ReqFilters)

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

func (h *simpleCacheHandler) Dump(w io.Writer) error {
	events := h.c.Find([]*ReqFilter{{}})

	b, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("failed to write events: %w", err)
	}

	return nil
}

func (h *simpleCacheHandler) Restore(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read events: %w", err)
	}

	var events []*Event
	if err := json.Unmarshal(b, &events); err != nil {
		return fmt.Errorf("failed to unmarshal events: %w", err)
	}

	for _, e := range events {
		h.c.Add(e)
	}

	return nil
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
	ctx context.Context,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	return newMergeHandlerSession(h).Handle(ctx, recv, send)
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
	ctx context.Context,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	ctx, cancel := context.WithCancel(ctx)
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

	return ss.runHandlers(ctx, ss.h.hs)
}

func (ss *mergeHandlerSession) runHandlers(ctx context.Context, handlers []Handler) (err error) {
	ctx, cancel := context.WithCancel(ctx)

	l := len(handlers)

	if l > 1 {
		errCh := make(chan error, 1)
		defer func() { err = errors.Join(err, <-errCh) }()

		go func() {
			defer cancel()
			errCh <- ss.runHandlers(ctx, handlers[:l-1])
		}()
	}

	defer cancel()
	return handlers[l-1].Handle(ctx, ss.recvs[l-1], ss.sends[l-1])
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

func (ss *mergeHandlerSession) handleRecv(ctx context.Context, recv <-chan ClientMsg) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-recv:
			if !ok {
				return
			}
			msg = ss.handleRecvMsg(msg)
			ss.broadcastRecvs(ctx, msg)
		}
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
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-ss.preSendCh:
			m := ss.handleSendMsg(msg)
			sendServerMsgCtx(ctx, send, m)
		}
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

type SimpleMiddlewareBaseInterface interface {
	HandleStart(context.Context) (context.Context, error)
	HandleStop(context.Context) error
	HandleClientMsg(context.Context, ClientMsg) (<-chan ClientMsg, <-chan ServerMsg, error)
	HandleServerMsg(context.Context, ServerMsg) (<-chan ServerMsg, error)
}

func NewSimpleMiddleware(bases ...SimpleMiddlewareBaseInterface) SimpleMiddleware {
	return func(handler Handler) Handler {
		return HandlerFunc(
			func(ctx context.Context, recv <-chan ClientMsg, send chan<- ServerMsg) (err error) {
				for _, base := range bases {
					defer func() { err = errors.Join(err, base.HandleStop(ctx)) }()

					ctx, err = base.HandleStart(ctx)
					if err != nil {
						return
					}
				}

				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				rCh := make(chan ClientMsg)
				sCh := make(chan ServerMsg)

				errs := make(chan error, 2)
				defer func() { err = errors.Join(err, <-errs, <-errs) }()

				go func() {
					defer cancel()
					defer close(rCh)

					errs <- simpleMiddlewareHandleRecv(ctx, bases, recv, send, rCh)
				}()

				go func() {
					defer cancel()

					errs <- simpleMiddlewareHandleSend(ctx, bases, send, sCh)
				}()

				defer cancel()
				return handler.Handle(ctx, rCh, sCh)
			},
		)
	}
}

func simpleMiddlewareHandleRecv(
	ctx context.Context,
	bases []SimpleMiddlewareBaseInterface,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
	rCh chan ClientMsg,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case cmsg, ok := <-recv:
			if !ok {
				return ErrRecvClosed
			}

			cmsgs := []ClientMsg{cmsg}

			for _, base := range bases {
				var nextCmsgs []ClientMsg
				for _, cmsg := range cmsgs {
					cmsgCh, smsgCh, err := base.HandleClientMsg(ctx, cmsg)
					if err != nil {
						return err
					}
					if smsgCh != nil {
					LoopSmsgCh:
						for {
							select {
							case <-ctx.Done():
								return ctx.Err()

							case smsg, ok := <-smsgCh:
								if !ok {
									break LoopSmsgCh
								}
								sendServerMsgCtx(ctx, send, smsg)
							}
						}
					}
					if cmsgCh != nil {
					LoopCmsgCh:
						for {
							select {
							case <-ctx.Done():
								return ctx.Err()

							case cmsg, ok := <-cmsgCh:
								if !ok {
									break LoopCmsgCh
								}
								nextCmsgs = append(nextCmsgs, cmsg)
							}
						}
					}
				}
				cmsgs = nextCmsgs
			}

			for _, cmsg := range cmsgs {
				sendClientMsgCtx(ctx, rCh, cmsg)
			}
		}
	}
}

func simpleMiddlewareHandleSend(
	ctx context.Context,
	bases []SimpleMiddlewareBaseInterface,
	send chan<- ServerMsg,
	sCh chan ServerMsg,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case smsg := <-sCh:
			smsgs := []ServerMsg{smsg}
			for i := len(bases) - 1; 0 <= i; i-- {
				var nextSmsgs []ServerMsg
				for _, smsg := range smsgs {
					smsgCh, err := bases[i].HandleServerMsg(ctx, smsg)
					if err != nil {
						return err
					}
					if smsgCh != nil {
					LoopSmsgCh:
						for {
							select {
							case <-ctx.Done():
								return ctx.Err()

							case smsg, ok := <-smsgCh:
								if !ok {
									break LoopSmsgCh
								}
								nextSmsgs = append(nextSmsgs, smsg)
							}
						}
					}
				}
				smsgs = nextSmsgs
			}

			for _, smsg := range smsgs {
				sendServerMsgCtx(ctx, send, smsg)
			}
		}
	}
}

func BuildMiddlewareFromNIP11(nip11 *NIP11) Middleware {
	if nip11 == nil {
		return func(h Handler) Handler { return h }
	}

	return func(h Handler) Handler {
		bases := make([]SimpleMiddlewareBaseInterface, 0, 7)
		if v := nip11.Limitation.MaxSubscriptions; v != 0 {
			bases = append(bases, NewSimpleMaxSubscriptionsMiddlewareBase(v))
		}
		if v := nip11.Limitation.MaxFilters; v != 0 {
			bases = append(bases, NewSimpleMaxReqFiltersMiddlewareBase(v))
		}
		if v := nip11.Limitation.MaxLimit; v != 0 {
			bases = append(bases, NewSimpleMaxLimitMiddlewareBase(v))
		}
		if v := nip11.Limitation.MaxEventTags; v != 0 {
			bases = append(bases, NewSimpleMaxEventTagsMiddlewareBase(v))
		}
		if v := nip11.Limitation.MaxContentLength; v != 0 {
			bases = append(bases, NewSimpleMaxContentLengthMiddlewareBase(v))
		}
		if v := nip11.Limitation.CreatedAtLowerLimit; v != 0 {
			bases = append(bases, NewSimpleCreatedAtLowerLimitMiddlewareBase(v))
		}
		if v := nip11.Limitation.CreatedAtUpperLimit; v != 0 {
			bases = append(bases, NewSimpleCreatedAtUpperLimitMiddlewareBase(v))
		}
		if len(bases) == 0 {
			return h
		}
		return NewSimpleMiddleware(bases...)(h)
	}
}

type EventCreatedAtMiddleware Middleware

func NewEventCreatedAtMiddleware(
	from, to time.Duration,
) EventCreatedAtMiddleware {
	m := NewSimpleEventCreatedAtMiddlewareBase(from, to)
	return EventCreatedAtMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBaseInterface = (*SimpleEventCreatedAtMiddlewareBase)(nil)

type SimpleEventCreatedAtMiddlewareBase struct {
	from, to time.Duration
}

func NewSimpleEventCreatedAtMiddlewareBase(
	from, to time.Duration,
) *SimpleEventCreatedAtMiddlewareBase {
	return &SimpleEventCreatedAtMiddlewareBase{from: from, to: to}
}

func (m *SimpleEventCreatedAtMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleEventCreatedAtMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleEventCreatedAtMiddlewareBase) HandleClientMsg(
	ctx context.Context,
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

func (m *SimpleEventCreatedAtMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type MaxSubscriptionsMiddleware Middleware

func NewMaxSubscriptionsMiddleware(maxSubs int) MaxSubscriptionsMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(ctx context.Context, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				sm := NewSimpleMaxSubscriptionsMiddlewareBase(maxSubs)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(ctx, recv, send)
			},
		)
	}
}

type SimpleMaxSubscriptionsMiddlewareBase struct {
	maxSubs int

	// map[subID]exist
	mu   sync.Mutex
	subs map[string]bool
}

func NewSimpleMaxSubscriptionsMiddlewareBase(
	maxSubs int,
) *SimpleMaxSubscriptionsMiddlewareBase {
	if maxSubs < 1 {
		panicf("max subscriptions must be a positive integer but got %d", maxSubs)
	}
	return &SimpleMaxSubscriptionsMiddlewareBase{
		maxSubs: maxSubs,
		subs:    make(map[string]bool, maxSubs+1),
	}
}

func (m *SimpleMaxSubscriptionsMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleMaxSubscriptionsMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleMaxSubscriptionsMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	// TODO(high-moctane) small critical section
	m.mu.Lock()
	defer m.mu.Unlock()

	switch msg := msg.(type) {
	case *ClientReqMsg:
		m.subs[msg.SubscriptionID] = true
		if len(m.subs) > m.maxSubs {
			delete(m.subs, msg.SubscriptionID)
			closed := NewServerClosedMsgf(msg.SubscriptionID, "", "too many req: max subscriptions is %d", m.maxSubs)
			return nil, newClosedBufCh[ServerMsg](closed), nil
		}

	case *ClientCloseMsg:
		delete(m.subs, msg.SubscriptionID)
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *SimpleMaxSubscriptionsMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxReqFiltersMiddleware Middleware

func NewMaxReqFiltersMiddleware(maxFilters int) MaxReqFiltersMiddleware {
	return MaxReqFiltersMiddleware(
		NewSimpleMiddleware(NewSimpleMaxReqFiltersMiddlewareBase(maxFilters)),
	)
}

var _ SimpleMiddlewareBaseInterface = (*SimpleMaxReqFiltersMiddlewareBase)(nil)

type SimpleMaxReqFiltersMiddlewareBase struct {
	maxFilters int
}

func NewSimpleMaxReqFiltersMiddlewareBase(
	maxFilters int,
) *SimpleMaxReqFiltersMiddlewareBase {
	if maxFilters < 1 {
		panicf("max subscriptions must be a positive integer but got %d", maxFilters)
	}
	return &SimpleMaxReqFiltersMiddlewareBase{maxFilters: maxFilters}
}

func (m *SimpleMaxReqFiltersMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleMaxReqFiltersMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleMaxReqFiltersMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		if len(msg.ReqFilters) > m.maxFilters {
			closed := NewServerClosedMsgf(msg.SubscriptionID, "", "too many req filters: max filters is %d", m.maxFilters)
			return nil, newClosedBufCh[ServerMsg](closed), nil
		}

	case *ClientCountMsg:
		if len(msg.ReqFilters) > m.maxFilters {
			closed := NewServerClosedMsgf(msg.SubscriptionID, "", "too many count filters: max filters is %d", m.maxFilters)
			return nil, newClosedBufCh[ServerMsg](closed), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *SimpleMaxReqFiltersMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxLimitMiddleware Middleware

func NewMaxLimitMiddleware(maxLimit int) MaxLimitMiddleware {
	return MaxLimitMiddleware(
		NewSimpleMiddleware(NewSimpleMaxLimitMiddlewareBase(maxLimit)),
	)
}

var _ SimpleMiddlewareBaseInterface = (*SimpleMaxLimitMiddlewareBase)(nil)

type SimpleMaxLimitMiddlewareBase struct {
	maxLimit int
}

func NewSimpleMaxLimitMiddlewareBase(
	maxLimit int,
) *SimpleMaxLimitMiddlewareBase {
	if maxLimit < 1 {
		panicf("max limit must be a positive integer but got %d", maxLimit)
	}
	return &SimpleMaxLimitMiddlewareBase{maxLimit: maxLimit}
}

func (m *SimpleMaxLimitMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleMaxLimitMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleMaxLimitMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		found := slices.ContainsFunc(msg.ReqFilters, func(f *ReqFilter) bool { return f.Limit != nil && *f.Limit > int64(m.maxLimit) })
		if found {
			closed := NewServerClosedMsgf(msg.SubscriptionID, "", "too large limit: max limit is %d", m.maxLimit)
			return nil, newClosedBufCh[ServerMsg](closed), nil
		}

	case *ClientCountMsg:
		found := slices.ContainsFunc(msg.ReqFilters, func(f *ReqFilter) bool { return f.Limit != nil && *f.Limit > int64(m.maxLimit) })
		if found {
			closed := NewServerClosedMsgf(msg.SubscriptionID, "", "too large limit: max limit is %d", m.maxLimit)
			return nil, newClosedBufCh[ServerMsg](closed), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *SimpleMaxLimitMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxSubIDLengthMiddleware Middleware

func NewMaxSubIDLengthMiddleware(maxSubIDLength int) MaxSubIDLengthMiddleware {
	return MaxSubIDLengthMiddleware(
		NewSimpleMiddleware(NewSimpleMaxSubIDLengthMiddlewareBase(maxSubIDLength)),
	)
}

var _ SimpleMiddlewareBaseInterface = (*SimpleMaxSubIDLengthMiddlewareBase)(nil)

type SimpleMaxSubIDLengthMiddlewareBase struct {
	maxSubIDLength int
}

func NewSimpleMaxSubIDLengthMiddlewareBase(
	maxSubIDLength int,
) *SimpleMaxSubIDLengthMiddlewareBase {
	if maxSubIDLength < 1 {
		panicf("max subid length must be a positive integer but got %d", maxSubIDLength)
	}
	return &SimpleMaxSubIDLengthMiddlewareBase{maxSubIDLength: maxSubIDLength}
}

func (m *SimpleMaxSubIDLengthMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleMaxSubIDLengthMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleMaxSubIDLengthMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		if len(msg.SubscriptionID) > m.maxSubIDLength {
			closed := NewServerClosedMsgf(msg.SubscriptionID, "", "too long subid: max subid length is %d", m.maxSubIDLength)
			return nil, newClosedBufCh[ServerMsg](closed), nil
		}

	case *ClientCountMsg:
		if len(msg.SubscriptionID) > m.maxSubIDLength {
			closed := NewServerClosedMsgf(msg.SubscriptionID, "", "too long subid: max subid length is %d", m.maxSubIDLength)
			return nil, newClosedBufCh[ServerMsg](closed), nil
		}
	}

	return newClosedBufCh(msg), nil, nil
}

func (m *SimpleMaxSubIDLengthMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxEventTagsMiddleware Middleware

func NewMaxEventTagsMiddleware(maxEventTags int) MaxEventTagsMiddleware {
	return MaxEventTagsMiddleware(
		NewSimpleMiddleware(NewSimpleMaxEventTagsMiddlewareBase(maxEventTags)),
	)
}

var _ SimpleMiddlewareBaseInterface = (*SimpleMaxEventTagsMiddlewareBase)(nil)

type SimpleMaxEventTagsMiddlewareBase struct {
	maxEventTags int
}

func NewSimpleMaxEventTagsMiddlewareBase(
	maxEventTags int,
) *SimpleMaxEventTagsMiddlewareBase {
	if maxEventTags < 1 {
		panicf("max event tags must be a positive integer but got %d", maxEventTags)
	}
	return &SimpleMaxEventTagsMiddlewareBase{maxEventTags: maxEventTags}
}

func (m *SimpleMaxEventTagsMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleMaxEventTagsMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleMaxEventTagsMiddlewareBase) HandleClientMsg(
	ctx context.Context,
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

func (m *SimpleMaxEventTagsMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxContentLengthMiddleware Middleware

func NewMaxContentLengthMiddleware(maxContentLength int) MaxContentLengthMiddleware {
	return MaxContentLengthMiddleware(
		NewSimpleMiddleware(NewSimpleMaxContentLengthMiddlewareBase(maxContentLength)),
	)
}

var _ SimpleMiddlewareBaseInterface = (*SimpleMaxContentLengthMiddlewareBase)(nil)

type SimpleMaxContentLengthMiddlewareBase struct {
	maxContentLength int
}

func NewSimpleMaxContentLengthMiddlewareBase(
	maxContentLength int,
) *SimpleMaxContentLengthMiddlewareBase {
	if maxContentLength < 1 {
		panicf("max content length must be a positive integer but got %d", maxContentLength)
	}
	return &SimpleMaxContentLengthMiddlewareBase{maxContentLength: maxContentLength}
}

func (m *SimpleMaxContentLengthMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleMaxContentLengthMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleMaxContentLengthMiddlewareBase) HandleClientMsg(
	ctx context.Context,
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

func (m *SimpleMaxContentLengthMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type CreatedAtLowerLimitMiddleware Middleware

func NewCreatedAtLowerLimitMiddleware(lower int64) CreatedAtLowerLimitMiddleware {
	m := NewSimpleCreatedAtLowerLimitMiddlewareBase(lower)
	return CreatedAtLowerLimitMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBaseInterface = (*SimpleCreatedAtLowerLimitMiddlewareBase)(nil)

type SimpleCreatedAtLowerLimitMiddlewareBase struct {
	lower int64
}

func NewSimpleCreatedAtLowerLimitMiddlewareBase(
	lower int64,
) *SimpleCreatedAtLowerLimitMiddlewareBase {
	return &SimpleCreatedAtLowerLimitMiddlewareBase{lower: lower}
}

func (m *SimpleCreatedAtLowerLimitMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleCreatedAtLowerLimitMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleCreatedAtLowerLimitMiddlewareBase) HandleClientMsg(
	ctx context.Context,
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

func (m *SimpleCreatedAtLowerLimitMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type CreatedAtUpperLimitMiddleware Middleware

func NewCreatedAtUpperLimitMiddleware(upper int64) CreatedAtUpperLimitMiddleware {
	m := NewSimpleCreatedAtUpperLimitMiddlewareBase(upper)
	return CreatedAtUpperLimitMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBaseInterface = (*SimpleCreatedAtUpperLimitMiddlewareBase)(nil)

type SimpleCreatedAtUpperLimitMiddlewareBase struct {
	upper int64
}

func NewSimpleCreatedAtUpperLimitMiddlewareBase(
	upper int64,
) *SimpleCreatedAtUpperLimitMiddlewareBase {
	return &SimpleCreatedAtUpperLimitMiddlewareBase{upper: upper}
}

func (m *SimpleCreatedAtUpperLimitMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleCreatedAtUpperLimitMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleCreatedAtUpperLimitMiddlewareBase) HandleClientMsg(
	ctx context.Context,
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

func (m *SimpleCreatedAtUpperLimitMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type RecvEventUniqueFilterMiddleware Middleware

func NewRecvEventUniqueFilterMiddleware(size int) RecvEventUniqueFilterMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(ctx context.Context, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				sm := NewSimpleRecvEventUniqueFilterMiddlewareBase(size)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(ctx, recv, send)
			},
		)
	}
}

var _ SimpleMiddlewareBaseInterface = (*SimpleRecvEventUniqueFilterMiddlewareBase)(nil)

type SimpleRecvEventUniqueFilterMiddlewareBase struct {
	c *randCache[string, struct{}]
}

func NewSimpleRecvEventUniqueFilterMiddlewareBase(
	size int,
) *SimpleRecvEventUniqueFilterMiddlewareBase {
	return &SimpleRecvEventUniqueFilterMiddlewareBase{
		c: newRandCache[string, struct{}](size),
	}
}

func (m *SimpleRecvEventUniqueFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleRecvEventUniqueFilterMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleRecvEventUniqueFilterMiddlewareBase) HandleClientMsg(
	ctx context.Context,
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

func (m *SimpleRecvEventUniqueFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type SendEventUniqueFilterMiddleware Middleware

func NewSendEventUniqueFilterMiddleware(size int) SendEventUniqueFilterMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(ctx context.Context, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				sm := NewSimpleSendEventUniqueFilterMiddlewareBase(size)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(ctx, recv, send)
			},
		)
	}
}

var _ SimpleMiddlewareBaseInterface = (*SimpleSendEventUniqueFilterMiddlewareBase)(nil)

type SimpleSendEventUniqueFilterMiddlewareBase struct {
	c *randCache[string, struct{}]
}

func NewSimpleSendEventUniqueFilterMiddlewareBase(
	size int,
) *SimpleSendEventUniqueFilterMiddlewareBase {
	return &SimpleSendEventUniqueFilterMiddlewareBase{
		c: newRandCache[string, struct{}](size),
	}
}

func (m *SimpleSendEventUniqueFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleSendEventUniqueFilterMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleSendEventUniqueFilterMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *SimpleSendEventUniqueFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
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

type RecvEventAllowFilterMiddleware Middleware

func NewRecvEventAllowFilterMiddleware(matcher EventMatcher) RecvEventAllowFilterMiddleware {
	m := NewSimpleRecvEventAllowFilterMiddlewareBase(matcher)
	return RecvEventAllowFilterMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBaseInterface = (*SimpleRecvEventAllowFilterMiddlewareBase)(nil)

type SimpleRecvEventAllowFilterMiddlewareBase struct {
	matcher EventMatcher
}

func NewSimpleRecvEventAllowFilterMiddlewareBase(
	matcher EventMatcher,
) *SimpleRecvEventAllowFilterMiddlewareBase {
	return &SimpleRecvEventAllowFilterMiddlewareBase{matcher: matcher}
}

func (m *SimpleRecvEventAllowFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleRecvEventAllowFilterMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleRecvEventAllowFilterMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		if !m.matcher.Match(msg.Event) {
			okMsg := NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOkMsgPrefixBlocked,
				"the event is not allowed",
			)
			return nil, newClosedBufCh[ServerMsg](okMsg), nil
		}
	}
	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *SimpleRecvEventAllowFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type RecvEventDenyFilterMiddleware Middleware

func NewRecvEventDenyFilterMiddleware(matcher EventMatcher) RecvEventDenyFilterMiddleware {
	m := NewSimpleRecvEventDenyFilterMiddlewareBase(matcher)
	return RecvEventDenyFilterMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBaseInterface = (*SimpleRecvEventDenyFilterMiddlewareBase)(nil)

type SimpleRecvEventDenyFilterMiddlewareBase struct {
	matcher EventMatcher
}

func NewSimpleRecvEventDenyFilterMiddlewareBase(
	matcher EventMatcher,
) *SimpleRecvEventDenyFilterMiddlewareBase {
	return &SimpleRecvEventDenyFilterMiddlewareBase{matcher: matcher}
}

func (m *SimpleRecvEventDenyFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *SimpleRecvEventDenyFilterMiddlewareBase) HandleStop(ctx context.Context) error {
	return nil
}

func (m *SimpleRecvEventDenyFilterMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	if msg, ok := msg.(*ClientEventMsg); ok {
		if m.matcher.Match(msg.Event) {
			okMsg := NewServerOKMsg(
				msg.Event.ID,
				false,
				ServerOkMsgPrefixBlocked,
				"the event is not allowed",
			)
			return nil, newClosedBufCh[ServerMsg](okMsg), nil
		}
	}
	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *SimpleRecvEventDenyFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}
