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
	lru "github.com/hashicorp/golang-lru/v2"
)

var (
	ErrRecvClosed = errors.New("recv closed")
)

type Handler interface {
	Handle(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) error
}

type HandlerFunc func(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) error

func (f HandlerFunc) Handle(
	ctx context.Context,
	send chan<- ServerMsg,
	recv <-chan ClientMsg,
) error {
	return f(ctx, send, recv)
}

type SimpleHandler Handler

type SimpleHandlerBase interface {
	HandleStart(context.Context) (context.Context, error)
	HandleEnd(context.Context) error
	HandleClientMsg(context.Context, ClientMsg) (<-chan ServerMsg, error)
}

func NewSimpleHandler(h SimpleHandlerBase) SimpleHandler {
	return HandlerFunc(
		func(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) (err error) {
			ctx, err = h.HandleStart(ctx)
			if err != nil {
				return
			}
			defer func() { err = errors.Join(err, h.HandleEnd(ctx)) }()

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

type DefaultSimpleHandlerBase struct{}

func NewDefaultHandler() Handler {
	return NewSimpleHandler(DefaultSimpleHandlerBase{})
}

func (DefaultSimpleHandlerBase) HandleStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (DefaultSimpleHandlerBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (DefaultSimpleHandlerBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ServerMsg, error) {
	switch msg := msg.(type) {
	case *ClientEventMsg:
		return newClosedBufCh[ServerMsg](NewServerOKMsg(msg.Event.ID, false, "", "")), nil

	case *ClientReqMsg:
		return newClosedBufCh[ServerMsg](NewServerClosedMsg(msg.SubscriptionID, "", "")), nil

	case *ClientCloseMsg:
		return nil, nil

	case *ClientAuthMsg:
		return nil, nil

	case *ClientCountMsg:
		return newClosedBufCh[ServerMsg](NewServerCountMsg(msg.SubscriptionID, 0, nil)), nil

	default:
		return nil, nil
	}
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
	send chan<- ServerMsg,
	recv <-chan ClientMsg,
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
	send chan<- ServerMsg,
	recv <-chan ClientMsg,
) error {
	return NewSimpleHandler(h.h).Handle(ctx, send, recv)
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

func (h *simpleCacheHandler) HandleEnd(ctx context.Context) error {
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
	send chan<- ServerMsg,
	recv <-chan ClientMsg,
) error {
	return newMergeHandlerSession(h).Handle(ctx, send, recv)
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
	send chan<- ServerMsg,
	recv <-chan ClientMsg,
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
	return handlers[l-1].Handle(ctx, ss.sends[l-1], ss.recvs[l-1])
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
	s.SetSubID(msg.SubscriptionID, msg.ReqFilters)
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
	// map[subID]EventMatcher
	matcher map[string]EventCountMatcher
}

func newMergeHandlerSessionReqState(size int) *mergeHandlerSessionReqState {
	return &mergeHandlerSessionReqState{
		size:      size,
		eose:      make(map[string][]bool),
		lastEvent: make(map[string]*ServerEventMsg),
		seen:      make(map[string]map[string]bool),
		matcher:   make(map[string]EventCountMatcher),
	}
}

func (stat *mergeHandlerSessionReqState) SetSubID(subID string, fs []*ReqFilter) {
	stat.eose[subID] = make([]bool, stat.size)
	stat.lastEvent[subID] = nil
	stat.seen[subID] = make(map[string]bool)
	stat.matcher[subID] = NewReqFiltersEventMatchers(fs)
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
		delete(stat.matcher, subID)
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

	if stat.matcher[msg.SubscriptionID].Done() {
		return false
	}
	if !stat.matcher[msg.SubscriptionID].CountMatch(msg.Event) {
		return false
	}

	return true
}

func (stat *mergeHandlerSessionReqState) ClearSubID(subID string) {
	delete(stat.eose, subID)
	delete(stat.lastEvent, subID)
	delete(stat.seen, subID)
	delete(stat.matcher, subID)
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

type SimpleMiddlewareBase interface {
	HandleStart(context.Context) (context.Context, error)
	HandleEnd(context.Context) error
	HandleClientMsg(context.Context, ClientMsg) (<-chan ClientMsg, <-chan ServerMsg, error)
	HandleServerMsg(context.Context, ServerMsg) (<-chan ServerMsg, error)
}

func NewSimpleMiddleware(base SimpleMiddlewareBase) SimpleMiddleware {
	return func(handler Handler) Handler {
		return HandlerFunc(
			func(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) (err error) {
				ctx, err = base.HandleStart(ctx)
				if err != nil {
					return
				}
				defer func() { err = errors.Join(err, base.HandleEnd(ctx)) }()

				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				rCh := make(chan ClientMsg)
				sCh := make(chan ServerMsg)

				errs := make(chan error, 2)
				defer func() { err = errors.Join(err, <-errs, <-errs) }()

				go func() {
					defer cancel()
					defer close(rCh)

					errs <- simpleMiddlewareHandleRecv(ctx, base, recv, send, rCh)
				}()

				go func() {
					defer cancel()

					errs <- simpleMiddlewareHandleSend(ctx, base, send, sCh)
				}()

				defer cancel()
				return handler.Handle(ctx, sCh, rCh)
			},
		)
	}
}

func simpleMiddlewareHandleRecv(
	ctx context.Context,
	base SimpleMiddlewareBase,
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
						sendClientMsgCtx(ctx, rCh, cmsg)
					}
				}
			}
		}
	}
}

func simpleMiddlewareHandleSend(
	ctx context.Context,
	base SimpleMiddlewareBase,
	send chan<- ServerMsg,
	sCh chan ServerMsg,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case smsg := <-sCh:
			smsgCh, err := base.HandleServerMsg(ctx, smsg)
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
		}
	}
}

func BuildMiddlewareFromNIP11(nip11 *NIP11) Middleware {
	if nip11 == nil {
		return func(h Handler) Handler { return h }
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
	m := newSimpleEventCreatedAtMiddlewareBase(from, to)
	return EventCreatedAtMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBase = (*simpleEventCreatedAtMiddlewareBase)(nil)

type simpleEventCreatedAtMiddlewareBase struct {
	from, to time.Duration
}

func newSimpleEventCreatedAtMiddlewareBase(
	from, to time.Duration,
) *simpleEventCreatedAtMiddlewareBase {
	return &simpleEventCreatedAtMiddlewareBase{from: from, to: to}
}

func (m *simpleEventCreatedAtMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleEventCreatedAtMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleEventCreatedAtMiddlewareBase) HandleClientMsg(
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

func (m *simpleEventCreatedAtMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type MaxSubscriptionsMiddleware Middleware

func NewMaxSubscriptionsMiddleware(maxSubs int) MaxSubscriptionsMiddleware {
	return MaxSubscriptionsMiddleware(
		NewSimpleMiddleware(newSimpleMaxSubscriptionsMiddlewareBase(maxSubs)),
	)
}

type simpleMaxSubscriptionsMiddlewareBase struct {
	maxSubs int
}

type simpleMaxSubscriptionsMiddlewareBaseCtxKeyType struct{}

var simpleMaxSubscriptionsMiddlewareBaseCtxKey = simpleMaxSubscriptionsMiddlewareBaseCtxKeyType{}

type simpleMaxSubscriptionsMiddlewareBaseCtxValue struct {
	mu sync.Mutex
	// map[subID]bool
	subs map[string]bool
}

func newSimpleMaxSubscriptionsMiddlewareBase(
	maxSubs int,
) *simpleMaxSubscriptionsMiddlewareBase {
	if maxSubs < 1 {
		panicf("max subscriptions must be a positive integer but got %d", maxSubs)
	}
	return &simpleMaxSubscriptionsMiddlewareBase{
		maxSubs: maxSubs,
	}
}

func (m *simpleMaxSubscriptionsMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	v := &simpleMaxSubscriptionsMiddlewareBaseCtxValue{
		subs: make(map[string]bool, m.maxSubs+1),
	}

	ctx = context.WithValue(ctx, simpleMaxSubscriptionsMiddlewareBaseCtxKey, v)
	return ctx, nil
}

func (m *simpleMaxSubscriptionsMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleMaxSubscriptionsMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (cmsgCh <-chan ClientMsg, smsgCh <-chan ServerMsg, err error) {
	switch msg := msg.(type) {
	case *ClientReqMsg:
		return m.handleClientReqMsg(ctx, msg)

	case *ClientCloseMsg:
		return m.handleClientCloseMsg(ctx, msg)

	default:
		return newClosedBufCh(msg), nil, nil
	}
}

func (m *simpleMaxSubscriptionsMiddlewareBase) handleClientReqMsg(
	ctx context.Context,
	msg *ClientReqMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	v := ctx.Value(simpleMaxSubscriptionsMiddlewareBaseCtxKey).(*simpleMaxSubscriptionsMiddlewareBaseCtxValue)
	v.mu.Lock()
	defer v.mu.Unlock()

	v.subs[msg.SubscriptionID] = true
	if len(v.subs) > m.maxSubs {
		delete(v.subs, msg.SubscriptionID)
		closed := NewServerClosedMsgf(
			msg.SubscriptionID,
			"",
			"too many req: max subscriptions is %d",
			m.maxSubs,
		)
		return nil, newClosedBufCh[ServerMsg](closed), nil
	}

	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleMaxSubscriptionsMiddlewareBase) handleClientCloseMsg(
	ctx context.Context,
	msg *ClientCloseMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	v := ctx.Value(simpleMaxSubscriptionsMiddlewareBaseCtxKey).(*simpleMaxSubscriptionsMiddlewareBaseCtxValue)
	v.mu.Lock()
	defer v.mu.Unlock()

	delete(v.subs, msg.SubscriptionID)

	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleMaxSubscriptionsMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxReqFiltersMiddleware Middleware

func NewMaxReqFiltersMiddleware(maxFilters int) MaxReqFiltersMiddleware {
	return MaxReqFiltersMiddleware(
		NewSimpleMiddleware(newSimpleMaxReqFiltersMiddlewareBase(maxFilters)),
	)
}

var _ SimpleMiddlewareBase = (*simpleMaxReqFiltersMiddlewareBase)(nil)

type simpleMaxReqFiltersMiddlewareBase struct {
	maxFilters int
}

func newSimpleMaxReqFiltersMiddlewareBase(
	maxFilters int,
) *simpleMaxReqFiltersMiddlewareBase {
	if maxFilters < 1 {
		panicf("max subscriptions must be a positive integer but got %d", maxFilters)
	}
	return &simpleMaxReqFiltersMiddlewareBase{maxFilters: maxFilters}
}

func (m *simpleMaxReqFiltersMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleMaxReqFiltersMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleMaxReqFiltersMiddlewareBase) HandleClientMsg(
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

func (m *simpleMaxReqFiltersMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxLimitMiddleware Middleware

func NewMaxLimitMiddleware(maxLimit int) MaxLimitMiddleware {
	return MaxLimitMiddleware(
		NewSimpleMiddleware(newSimpleMaxLimitMiddlewareBase(maxLimit)),
	)
}

var _ SimpleMiddlewareBase = (*simpleMaxLimitMiddlewareBase)(nil)

type simpleMaxLimitMiddlewareBase struct {
	maxLimit int
}

func newSimpleMaxLimitMiddlewareBase(
	maxLimit int,
) *simpleMaxLimitMiddlewareBase {
	if maxLimit < 1 {
		panicf("max limit must be a positive integer but got %d", maxLimit)
	}
	return &simpleMaxLimitMiddlewareBase{maxLimit: maxLimit}
}

func (m *simpleMaxLimitMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleMaxLimitMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleMaxLimitMiddlewareBase) HandleClientMsg(
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

func (m *simpleMaxLimitMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxSubIDLengthMiddleware Middleware

func NewMaxSubIDLengthMiddleware(maxSubIDLength int) MaxSubIDLengthMiddleware {
	return MaxSubIDLengthMiddleware(
		NewSimpleMiddleware(newSimpleMaxSubIDLengthMiddlewareBase(maxSubIDLength)),
	)
}

var _ SimpleMiddlewareBase = (*simpleMaxSubIDLengthMiddlewareBase)(nil)

type simpleMaxSubIDLengthMiddlewareBase struct {
	maxSubIDLength int
}

func newSimpleMaxSubIDLengthMiddlewareBase(
	maxSubIDLength int,
) *simpleMaxSubIDLengthMiddlewareBase {
	if maxSubIDLength < 1 {
		panicf("max subid length must be a positive integer but got %d", maxSubIDLength)
	}
	return &simpleMaxSubIDLengthMiddlewareBase{maxSubIDLength: maxSubIDLength}
}

func (m *simpleMaxSubIDLengthMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleMaxSubIDLengthMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleMaxSubIDLengthMiddlewareBase) HandleClientMsg(
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

func (m *simpleMaxSubIDLengthMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxEventTagsMiddleware Middleware

func NewMaxEventTagsMiddleware(maxEventTags int) MaxEventTagsMiddleware {
	return MaxEventTagsMiddleware(
		NewSimpleMiddleware(newSimpleMaxEventTagsMiddlewareBase(maxEventTags)),
	)
}

var _ SimpleMiddlewareBase = (*simpleMaxEventTagsMiddlewareBase)(nil)

type simpleMaxEventTagsMiddlewareBase struct {
	maxEventTags int
}

func newSimpleMaxEventTagsMiddlewareBase(
	maxEventTags int,
) *simpleMaxEventTagsMiddlewareBase {
	if maxEventTags < 1 {
		panicf("max event tags must be a positive integer but got %d", maxEventTags)
	}
	return &simpleMaxEventTagsMiddlewareBase{maxEventTags: maxEventTags}
}

func (m *simpleMaxEventTagsMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleMaxEventTagsMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleMaxEventTagsMiddlewareBase) HandleClientMsg(
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

func (m *simpleMaxEventTagsMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type MaxContentLengthMiddleware Middleware

func NewMaxContentLengthMiddleware(maxContentLength int) MaxContentLengthMiddleware {
	return MaxContentLengthMiddleware(
		NewSimpleMiddleware(newSimpleMaxContentLengthMiddlewareBase(maxContentLength)),
	)
}

var _ SimpleMiddlewareBase = (*simpleMaxContentLengthMiddlewareBase)(nil)

type simpleMaxContentLengthMiddlewareBase struct {
	maxContentLength int
}

func newSimpleMaxContentLengthMiddlewareBase(
	maxContentLength int,
) *simpleMaxContentLengthMiddlewareBase {
	if maxContentLength < 1 {
		panicf("max content length must be a positive integer but got %d", maxContentLength)
	}
	return &simpleMaxContentLengthMiddlewareBase{maxContentLength: maxContentLength}
}

func (m *simpleMaxContentLengthMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleMaxContentLengthMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleMaxContentLengthMiddlewareBase) HandleClientMsg(
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

func (m *simpleMaxContentLengthMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh(msg), nil
}

type CreatedAtLowerLimitMiddleware Middleware

func NewCreatedAtLowerLimitMiddleware(lower int64) CreatedAtLowerLimitMiddleware {
	m := newSimpleCreatedAtLowerLimitMiddlewareBase(lower)
	return CreatedAtLowerLimitMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBase = (*simpleCreatedAtLowerLimitMiddlewareBase)(nil)

type simpleCreatedAtLowerLimitMiddlewareBase struct {
	lower int64
}

func newSimpleCreatedAtLowerLimitMiddlewareBase(
	lower int64,
) *simpleCreatedAtLowerLimitMiddlewareBase {
	return &simpleCreatedAtLowerLimitMiddlewareBase{lower: lower}
}

func (m *simpleCreatedAtLowerLimitMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleCreatedAtLowerLimitMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleCreatedAtLowerLimitMiddlewareBase) HandleClientMsg(
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

func (m *simpleCreatedAtLowerLimitMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type CreatedAtUpperLimitMiddleware Middleware

func NewCreatedAtUpperLimitMiddleware(upper int64) CreatedAtUpperLimitMiddleware {
	m := newSimpleCreatedAtUpperLimitMiddlewareBase(upper)
	return CreatedAtUpperLimitMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBase = (*simpleCreatedAtUpperLimitMiddlewareBase)(nil)

type simpleCreatedAtUpperLimitMiddlewareBase struct {
	upper int64
}

func newSimpleCreatedAtUpperLimitMiddlewareBase(
	upper int64,
) *simpleCreatedAtUpperLimitMiddlewareBase {
	return &simpleCreatedAtUpperLimitMiddlewareBase{upper: upper}
}

func (m *simpleCreatedAtUpperLimitMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleCreatedAtUpperLimitMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleCreatedAtUpperLimitMiddlewareBase) HandleClientMsg(
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

func (m *simpleCreatedAtUpperLimitMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type RecvEventUniqueFilterMiddleware Middleware

func NewRecvEventUniqueFilterMiddleware(size int) RecvEventUniqueFilterMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) error {
				sm := newSimpleRecvEventUniqueFilterMiddlewareBase(size)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(ctx, send, recv)
			},
		)
	}
}

var _ SimpleMiddlewareBase = (*simpleRecvEventUniqueFilterMiddlewareBase)(nil)

type simpleRecvEventUniqueFilterMiddlewareBase struct {
	c *lru.Cache[string, struct{}]
}

func newSimpleRecvEventUniqueFilterMiddlewareBase(
	size int,
) *simpleRecvEventUniqueFilterMiddlewareBase {
	c, err := lru.New[string, struct{}](size)
	if err != nil {
		panicf("failed to create simpleRecvEventUniqueFilterMiddlewareBase: %v", err)
	}
	return &simpleRecvEventUniqueFilterMiddlewareBase{
		c: c,
	}
}

func (m *simpleRecvEventUniqueFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleRecvEventUniqueFilterMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleRecvEventUniqueFilterMiddlewareBase) HandleClientMsg(
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
		m.c.Add(msg.Event.ID, struct{}{})
	}

	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleRecvEventUniqueFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type SendEventUniqueFilterMiddleware Middleware

func NewSendEventUniqueFilterMiddleware(size int) SendEventUniqueFilterMiddleware {
	return func(h Handler) Handler {
		return HandlerFunc(
			func(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) error {
				sm := newSimpleSendEventUniqueFilterMiddlewareBase(size)
				m := NewSimpleMiddleware(sm)
				return m(h).Handle(ctx, send, recv)
			},
		)
	}
}

var _ SimpleMiddlewareBase = (*simpleSendEventUniqueFilterMiddlewareBase)(nil)

type simpleSendEventUniqueFilterMiddlewareBase struct {
	c *lru.Cache[string, struct{}]
}

func newSimpleSendEventUniqueFilterMiddlewareBase(
	size int,
) *simpleSendEventUniqueFilterMiddlewareBase {
	c, err := lru.New[string, struct{}](size)
	if err != nil {
		panicf("failed to create simpleSendEventUniqueFilterMiddlewareBase: %v", err)
	}
	return &simpleSendEventUniqueFilterMiddlewareBase{
		c: c,
	}
}

func (m *simpleSendEventUniqueFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleSendEventUniqueFilterMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleSendEventUniqueFilterMiddlewareBase) HandleClientMsg(
	ctx context.Context,
	msg ClientMsg,
) (<-chan ClientMsg, <-chan ServerMsg, error) {
	return newClosedBufCh[ClientMsg](msg), nil, nil
}

func (m *simpleSendEventUniqueFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	if msg, ok := msg.(*ServerEventMsg); ok {
		if _, found := m.c.Get(msg.Event.ID); found {
			return nil, nil
		}
		m.c.Add(msg.Event.ID, struct{}{})
	}

	return newClosedBufCh[ServerMsg](msg), nil
}

type RecvEventAllowFilterMiddleware Middleware

func NewRecvEventAllowFilterMiddleware(matcher EventMatcher) RecvEventAllowFilterMiddleware {
	m := newSimpleRecvEventAllowFilterMiddlewareBase(matcher)
	return RecvEventAllowFilterMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBase = (*simpleRecvEventAllowFilterMiddlewareBase)(nil)

type simpleRecvEventAllowFilterMiddlewareBase struct {
	matcher EventMatcher
}

func newSimpleRecvEventAllowFilterMiddlewareBase(
	matcher EventMatcher,
) *simpleRecvEventAllowFilterMiddlewareBase {
	return &simpleRecvEventAllowFilterMiddlewareBase{matcher: matcher}
}

func (m *simpleRecvEventAllowFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleRecvEventAllowFilterMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleRecvEventAllowFilterMiddlewareBase) HandleClientMsg(
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

func (m *simpleRecvEventAllowFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}

type RecvEventDenyFilterMiddleware Middleware

func NewRecvEventDenyFilterMiddleware(matcher EventMatcher) RecvEventDenyFilterMiddleware {
	m := newSimpleRecvEventDenyFilterMiddlewareBase(matcher)
	return RecvEventDenyFilterMiddleware(NewSimpleMiddleware(m))
}

var _ SimpleMiddlewareBase = (*simpleRecvEventDenyFilterMiddlewareBase)(nil)

type simpleRecvEventDenyFilterMiddlewareBase struct {
	matcher EventMatcher
}

func newSimpleRecvEventDenyFilterMiddlewareBase(
	matcher EventMatcher,
) *simpleRecvEventDenyFilterMiddlewareBase {
	return &simpleRecvEventDenyFilterMiddlewareBase{matcher: matcher}
}

func (m *simpleRecvEventDenyFilterMiddlewareBase) HandleStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (m *simpleRecvEventDenyFilterMiddlewareBase) HandleEnd(ctx context.Context) error {
	return nil
}

func (m *simpleRecvEventDenyFilterMiddlewareBase) HandleClientMsg(
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

func (m *simpleRecvEventDenyFilterMiddlewareBase) HandleServerMsg(
	ctx context.Context,
	msg ServerMsg,
) (<-chan ServerMsg, error) {
	return newClosedBufCh[ServerMsg](msg), nil
}
