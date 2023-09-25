package mocrelay

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRecvClosed = errors.New("recv client msg channel has been closed")
	ErrSendClosed = errors.New("send server msg channel has been closed")
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

	connID := uuid.NewString()
	defer router.subs.UnsubscribeAll(connID)

	subCh := NewTryChan[ServerMsg](router.Option.bufLen())

	sendCh := make(chan ServerMsg, 1)
Loop:
	for {
		select {
		case <-ctx.Done():
			break Loop

		case msg, ok := <-recv:
			if !ok {
				break Loop
			}
			m := router.recv(ctx, connID, msg, subCh)
			if m == nil || reflect.ValueOf(m).IsNil() {
				continue
			}
			sendCh <- m

		case msg := <-subCh:
			sendCh <- msg
		}

		select {
		case <-ctx.Done():
			break Loop

		case send <- <-sendCh:
		}
	}

	return ErrRouterStop
}

func (router *Router) recv(
	ctx context.Context,
	connID string,
	msg ClientMsg,
	subCh TryChan[ServerMsg],
) ServerMsg {
	switch m := msg.(type) {
	case *ClientReqMsg:
		return router.recvClientReqMsg(ctx, connID, m, subCh)

	case *ClientEventMsg:
		return router.recvClientEventMsg(ctx, connID, m, subCh)

	case *ClientCloseMsg:
		return router.recvClientCloseMsg(ctx, connID, m)

	default:
		return nil
	}
}

func (router *Router) recvClientReqMsg(
	ctx context.Context,
	connID string,
	msg *ClientReqMsg,
	subCh TryChan[ServerMsg],
) ServerMsg {
	sub := newSubscriber(connID, msg, subCh)
	router.subs.Subscribe(sub)
	return NewServerEOSEMsg(msg.SubscriptionID)
}

func (router *Router) recvClientEventMsg(
	ctx context.Context,
	connID string,
	msg *ClientEventMsg,
	subCh TryChan[ServerMsg],
) ServerMsg {
	router.subs.Publish(msg.Event)
	return NewServerOKMsg(msg.Event.ID, true, ServerOKMsgPrefixNoPrefix, "")
}

func (router *Router) recvClientCloseMsg(
	ctx context.Context,
	connID string,
	msg *ClientCloseMsg,
) ServerMsg {
	router.subs.Unsubscribe(connID, msg.SubscriptionID)
	return nil
}

type subscriber struct {
	ConnID         string
	SubscriptionID string
	Matcher        EventMatcher
	Ch             TryChan[ServerMsg]
}

func newSubscriber(connID string, msg *ClientReqMsg, ch chan ServerMsg) *subscriber {
	return &subscriber{
		ConnID:         connID,
		SubscriptionID: msg.SubscriptionID,
		Matcher:        NewReqFiltersEventMatchers(msg.ReqFilters),
		Ch:             ch,
	}
}

func (sub *subscriber) SendIfMatch(event *Event) {
	if sub.Matcher.Match(event) {
		sub.Ch.TrySend(NewServerEventMsg(sub.SubscriptionID, event))
	}
}

type subscribers struct {
	// map[connID]map[subID]*subscriber
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
	mch, ok := m[sub.ConnID]
	if ok {
		subs.subs <- m
	} else {
		mch = make(chan map[string]chan *subscriber, 1)
		m[sub.ConnID] = mch
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

func (subs *subscribers) Unsubscribe(connID, subID string) {
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

func (subs *subscribers) UnsubscribeAll(connID string) {
	m := <-subs.subs
	delete(m, connID)
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

type CacheHandler struct {
	opEventCh chan *cacheHandlerOpEvent
	opReqCh   chan *cacheHandlerOpReq
	cancel    context.CancelFunc
}

type cacheHandlerOpEvent struct {
	ev  *Event
	ret chan bool
}

type cacheHandlerOpReq struct {
	matcher EventCountMatcher
	ret     chan []*Event
}

var ErrCacheHandlerStop = errors.New("cache handler stopped")

func NewCacheHandler(ctx context.Context, capacity int) *CacheHandler {
	ctx, cancel := context.WithCancel(ctx)
	c := &CacheHandler{
		opEventCh: make(chan *cacheHandlerOpEvent, 1),
		opReqCh:   make(chan *cacheHandlerOpReq, 1),
		cancel:    cancel,
	}
	c.start(ctx, capacity)
	return c
}

func (h *CacheHandler) Handle(
	r *http.Request,
	recv <-chan ClientMsg,
	send chan<- ServerMsg,
) error {
	for msg := range recv {
		switch msg := msg.(type) {
		case *ClientEventMsg:
			ch := make(chan bool)
			h.opEventCh <- &cacheHandlerOpEvent{
				ev:  msg.Event,
				ret: ch,
			}
			if <-ch {
				send <- NewServerOKMsg(msg.Event.ID, true, "", "")
			} else {
				send <- NewServerOKMsg(msg.Event.ID, false, ServerOKMsgPrefixDuplicate, "already have this event")
			}

		case *ClientReqMsg:
			ch := make(chan []*Event)
			h.opReqCh <- &cacheHandlerOpReq{
				matcher: NewReqFiltersEventMatchers(msg.ReqFilters),
				ret:     ch,
			}
			for _, e := range <-ch {
				send <- NewServerEventMsg(msg.SubscriptionID, e)
			}
			send <- NewServerEOSEMsg(msg.SubscriptionID)
		}
	}
	return ErrCacheHandlerStop
}

func (h *CacheHandler) start(ctx context.Context, capacity int) {
	c := newEventCache(capacity)

	num := runtime.GOMAXPROCS(0)
	sema := make(chan struct{}, num)

	for i := 0; i < num; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return

				case op := <-h.opEventCh:
					e := op.ev

					for i := 0; i < num; i++ {
						sema <- struct{}{}
					}

					if e.Kind == 5 {
						h.kind5(c, e)
					}
					ret := c.Add(e)

					for i := 0; i < num; i++ {
						<-sema
					}

					op.ret <- ret

				case op := <-h.opReqCh:
					sema <- struct{}{}
					ret := c.Find(op.matcher)
					<-sema
					op.ret <- ret
				}
			}
		}()
	}
}

func (h *CacheHandler) Stop() {
	h.cancel()
}

func (h *CacheHandler) kind5(c *eventCache, event *Event) {
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "e":
			c.DeleteID(tag[1], event.Pubkey)
		case "a":
			c.DeleteNaddr(tag[1], event.Pubkey)
		}
	}
}

type eventCache struct {
	rb   *ringBuffer[*Event]
	ids  map[string]*Event
	keys map[string]*Event
}

func newEventCache(capacity int) *eventCache {
	return &eventCache{
		rb:   newRingBuffer[*Event](capacity),
		ids:  make(map[string]*Event, capacity),
		keys: make(map[string]*Event, capacity),
	}
}

func (*eventCache) eventKeyRegular(event *Event) string { return event.ID }

func (*eventCache) eventKeyReplaceable(event *Event) string {
	return fmt.Sprintf("%s:%d", event.Pubkey, event.Kind)
}

func (*eventCache) eventKeyParameterized(event *Event) string {
	idx := slices.IndexFunc(event.Tags, func(t Tag) bool {
		return len(t) >= 1 && t[0] == "d"
	})
	if idx < 0 {
		return ""
	}

	d := ""
	if len(event.Tags[idx]) > 1 {
		d = event.Tags[idx][1]
	}

	return fmt.Sprintf("%s:%d:%s", event.Pubkey, event.Kind, d)
}

func (c *eventCache) eventKey(event *Event) (key string, ok bool) {
	switch event.EventType() {
	case EventTypeRegular:
		return c.eventKeyRegular(event), true
	case EventTypeReplaceable:
		return c.eventKeyReplaceable(event), true
	case EventTypeParamReplaceable:
		key := c.eventKeyParameterized(event)
		return key, key != ""
	default:
		return "", false
	}
}

func (c *eventCache) Add(event *Event) (added bool) {
	if c.ids[event.ID] != nil {
		return
	}
	key, ok := c.eventKey(event)
	if !ok {
		return
	}
	if old, ok := c.keys[key]; ok && old.CreatedAt > event.CreatedAt {
		return
	}

	idx := c.rb.IdxFunc(func(v *Event) bool {
		return v.CreatedAt < event.CreatedAt
	})
	if c.rb.Len() == c.rb.Cap && idx < 0 {
		return
	}

	c.ids[event.ID] = event
	c.keys[key] = event

	if c.rb.Len() == c.rb.Cap {
		old := c.rb.Dequeue()
		if k, _ := c.eventKey(old); c.keys[k] == old {
			delete(c.keys, k)
		}
		delete(c.ids, old.ID)
	}
	c.rb.Enqueue(event)

	for i := 0; i+1 < c.rb.Len(); i++ {
		if c.rb.At(i).CreatedAt < c.rb.At(i+1).CreatedAt {
			c.rb.Swap(i, i+1)
		}
	}

	added = true
	return
}

func (c *eventCache) DeleteID(id, pubkey string) {
	event := c.ids[id]
	if event == nil || event.Pubkey != pubkey {
		return
	}

	if k, _ := c.eventKey(event); c.keys[k] == event {
		delete(c.keys, k)
	}
	delete(c.ids, id)
}

func (c *eventCache) DeleteNaddr(naddr, pubkey string) {
	event := c.keys[naddr]
	if event == nil || event.Pubkey != pubkey {
		return
	}
	delete(c.ids, event.ID)
	delete(c.keys, naddr)
}

func (c *eventCache) Find(matcher EventCountMatcher) []*Event {
	var ret []*Event

	for i := 0; i < c.rb.Len(); i++ {
		ev := c.rb.At(i)

		if c.ids[ev.ID] == nil {
			continue
		}
		if k, _ := c.eventKey(ev); c.keys[k] != ev {
			continue
		}

		if matcher.Done() {
			break
		}
		if matcher.CountMatch(ev) {
			ret = append(ret, ev)
		}
	}

	return ret
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
			ss.preSendCh <- newMergeHandlerSessionSendMsg(idx, msg)
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
			if msg == nil || reflect.ValueOf(msg).IsNil() {
				continue
			}
			recvBuf <- msg
			recvCh = nil

		case msg, ok := <-recvBuf:
			if !ok {
				return
			}
			ss.broadcastRecvs(ctx, msg)
			recvCh = recv

		case msg := <-sendCh:
			m := ss.handleSendMsg(ctx, msg)
			if m == nil || reflect.ValueOf(m).IsNil() {
				continue
			}
			sendBuf <- msg.Msg
			sendCh = nil

		case msg := <-sendBuf:
			send <- msg
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
		select {
		case <-ctx.Done():
			return

		case r <- msg:
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
	if stat.seen[msg.SubscriptionID][msg.Event.ID] {
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

type EventCreatedAtReqFilterMiddleware func(next Handler) Handler

func NewEventCreatedAtReqFilterMiddleware(
	from, to time.Duration,
) EventCreatedAtReqFilterMiddleware {
	return func(next Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				ctx := r.Context()

				ch := make(chan ClientMsg, 1)

				go func() {
					defer close(ch)

					for {
						select {
						case <-ctx.Done():
							return

						case msg, ok := <-recv:
							if !ok {
								return
							}
							if m, ok := msg.(*ClientEventMsg); ok {
								t := m.Event.CreatedAtTime()
								now := time.Now()
								if t.Sub(now) < from || to < t.Sub(now) {
									continue
								}
							}
							ch <- msg
						}
					}
				}()

				return next.Handle(r, ch, send)
			},
		)
	}
}

type RecvEventUniquefyMiddleware func(Handler) Handler

func NewRecvEventUniquefyMiddleware(buflen int) RecvEventUniquefyMiddleware {
	if buflen < 0 {
		panic(
			fmt.Sprintf("RecvEventUniquefyMiddleware buflen must be 0 or more but got %v", buflen),
		)
	}

	ids := newRingBuffer[string](buflen)
	seen := make(map[string]bool)

	return func(handler Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				ctx := r.Context()

				ch := make(chan ClientMsg, 1)

				go func() {
					defer close(ch)

					for {
						select {
						case <-ctx.Done():
							return

						case msg, ok := <-recv:
							if !ok {
								return
							}
							if m, ok := msg.(*ClientEventMsg); ok {
								if seen[m.Event.ID] {
									continue
								}

								if ids.Len() == ids.Cap {
									old := ids.Dequeue()
									delete(seen, old)
								}

								ids.Enqueue(m.Event.ID)
								seen[m.Event.ID] = true
							}
							ch <- msg
						}
					}
				}()

				return handler.Handle(r, ch, send)
			},
		)
	}
}

type SendEventUniquefyMiddleware func(Handler) Handler

func NewSendEventUniquefyMiddleware(buflen int) SendEventUniquefyMiddleware {
	if buflen < 0 {
		panic(
			fmt.Sprintf("SendEventUniquefyMiddleware buflen must be 0 or more but got %v", buflen),
		)
	}

	keys := newRingBuffer[string](buflen)
	seen := make(map[string]bool)

	key := func(subID, eventID string) string { return subID + ":" + eventID }

	return func(handler Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan ClientMsg, send chan<- ServerMsg) error {
				ctx := r.Context()

				ch := make(chan ServerMsg, 1)
				defer close(ch)

				go func() {
					for {
						select {
						case <-ctx.Done():
							return

						case msg, ok := <-ch:
							if !ok {
								return
							}
							if m, ok := msg.(*ServerEventMsg); ok {
								k := key(m.SubscriptionID, m.Event.ID)

								if seen[k] {
									continue
								}

								if keys.Len() == keys.Cap {
									old := keys.Dequeue()
									delete(seen, old)
								}
								keys.Enqueue(k)
								seen[k] = true
							}
							send <- msg
						}
					}
				}()

				return handler.Handle(r, recv, ch)
			},
		)
	}
}
