package mocrelay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/high-moctane/mocrelay/nostr"
	"github.com/high-moctane/mocrelay/utils"
)

var (
	ErrRecvClosed = errors.New("recv client msg channel has been closed")
	ErrSendClosed = errors.New("send server msg channel has been closed")
)

type Handler interface {
	Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error
}

type HandlerFunc func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error

func (f HandlerFunc) Handle(
	r *http.Request,
	recv <-chan nostr.ClientMsg,
	send chan<- nostr.ServerMsg,
) error {
	return f(r, recv, send)
}

var ErrRouterStop = errors.New("router stopped")

type RouterOption struct {
	BufLen int
	// TODO(high-moctane) Add logger
}

func (opt *RouterOption) bufLen() int {
	if opt == nil {
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
	recv <-chan nostr.ClientMsg,
	send chan<- nostr.ServerMsg,
) error {
	ctx, cancel := context.WithCancel(r.Context())

	connID := uuid.NewString()
	defer router.subs.UnsubscribeAll(connID)

	msgCh := utils.NewTryChan[nostr.ServerMsg](router.Option.bufLen())

	errCh := make(chan error, 2)

	go func() {
		defer cancel()
		errCh <- router.serveRecv(ctx, connID, recv, msgCh)
	}()

	go func() {
		defer cancel()
		errCh <- router.serveSend(ctx, connID, send, msgCh)
	}()

	return errors.Join(<-errCh, <-errCh, ErrRouterStop)
}

func (router *Router) serveRecv(
	ctx context.Context,
	connID string,
	recv <-chan nostr.ClientMsg,
	msgCh utils.TryChan[nostr.ServerMsg],
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				return ErrRecvClosed
			}
			router.recv(ctx, connID, msg, msgCh)
		}
	}
}

func (router *Router) recv(
	ctx context.Context,
	connID string,
	msg nostr.ClientMsg,
	msgCh utils.TryChan[nostr.ServerMsg],
) {
	switch m := msg.(type) {
	case *nostr.ClientReqMsg:
		router.recvClientReqMsg(ctx, connID, m, msgCh)

	case *nostr.ClientEventMsg:
		router.recvClientEventMsg(ctx, connID, m, msgCh)

	case *nostr.ClientCloseMsg:
		router.recvClientCloseMsg(ctx, connID, m)
	}
}

func (router *Router) recvClientReqMsg(
	ctx context.Context,
	connID string,
	msg *nostr.ClientReqMsg,
	msgCh utils.TryChan[nostr.ServerMsg],
) {
	sub := newSubscriber(connID, msg, msgCh)
	router.subs.Subscribe(sub)
	msgCh.TrySend(nostr.NewServerEOSEMsg(msg.SubscriptionID))
}

func (router *Router) recvClientEventMsg(
	ctx context.Context,
	connID string,
	msg *nostr.ClientEventMsg,
	msgCh utils.TryChan[nostr.ServerMsg],
) {
	router.subs.Publish(msg.Event)
	msgCh.TrySend(nostr.NewServerOKMsg(msg.Event.ID, true, nostr.ServerOKMsgPrefixNoPrefix, ""))
}

func (router *Router) recvClientCloseMsg(
	ctx context.Context,
	connID string,
	msg *nostr.ClientCloseMsg,
) {
	router.subs.Unsubscribe(connID, msg.SubscriptionID)
}

func (router *Router) serveSend(
	ctx context.Context,
	connID string,
	send chan<- nostr.ServerMsg,
	msgCh utils.TryChan[nostr.ServerMsg],
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case send <- <-msgCh:
		}
	}
}

type subscriber struct {
	ConnID         string
	SubscriptionID string
	Matcher        EventMatcher
	Ch             utils.TryChan[nostr.ServerMsg]
}

func newSubscriber(connID string, msg *nostr.ClientReqMsg, ch chan nostr.ServerMsg) *subscriber {
	return &subscriber{
		ConnID:         connID,
		SubscriptionID: msg.SubscriptionID,
		Matcher:        NewReqFiltersEventMatchers(msg.ReqFilters),
		Ch:             ch,
	}
}

func (sub *subscriber) SendIfMatch(event *nostr.Event) {
	if sub.Matcher.Match(event) {
		sub.Ch.TrySend(nostr.NewServerEventMsg(sub.SubscriptionID, event))
	}
}

type subscribers struct {
	mu sync.RWMutex

	// map[connID]map[subID]*subscriber
	subs map[string]map[string]*subscriber
}

func newSubscribers() *subscribers {
	return &subscribers{
		subs: make(map[string]map[string]*subscriber),
	}
}

func (subs *subscribers) Subscribe(sub *subscriber) {
	subs.mu.Lock()
	defer subs.mu.Unlock()

	if _, ok := subs.subs[sub.ConnID]; !ok {
		subs.subs[sub.ConnID] = make(map[string]*subscriber)
	}
	subs.subs[sub.ConnID][sub.SubscriptionID] = sub
}

func (subs *subscribers) Unsubscribe(connID, subID string) {
	subs.mu.Lock()
	defer subs.mu.Unlock()

	delete(subs.subs[connID], subID)
}

func (subs *subscribers) UnsubscribeAll(connID string) {
	subs.mu.Lock()
	defer subs.mu.Unlock()

	delete(subs.subs, connID)
}

func (subs *subscribers) Publish(event *nostr.Event) {
	subs.mu.RLock()
	defer subs.mu.RUnlock()

	for _, m := range subs.subs {
		for _, sub := range m {
			sub.SendIfMatch(event)
		}
	}
}

type CacheHandler struct {
	c *eventCache
}

var ErrCacheHandlerStop = errors.New("cache handler stopped")

func NewCacheHandler(capacity int) *CacheHandler {
	return &CacheHandler{
		c: newEventCache(capacity),
	}
}

func (h *CacheHandler) Handle(
	r *http.Request,
	recv <-chan nostr.ClientMsg,
	send chan<- nostr.ServerMsg,
) error {
	for msg := range recv {
		switch msg := msg.(type) {
		case *nostr.ClientEventMsg:
			e := msg.Event
			if e.Kind == 5 {
				h.kind5(e)
			}
			if h.c.Add(e) {
				send <- nostr.NewServerOKMsg(e.ID, true, "", "")
			} else {
				send <- nostr.NewServerOKMsg(e.ID, false, nostr.ServerOKMsgPrefixDuplicate, "already have this event")
			}

		case *nostr.ClientReqMsg:
			for _, e := range h.c.Find(msg.ReqFilters) {
				send <- nostr.NewServerEventMsg(msg.SubscriptionID, e)
			}
			send <- nostr.NewServerEOSEMsg(msg.SubscriptionID)
		}
	}
	return ErrCacheHandlerStop
}

func (h *CacheHandler) kind5(event *nostr.Event) {
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "e":
			h.c.DeleteID(tag[1], event.Pubkey)
		case "a":
			h.c.DeleteNaddr(tag[1], event.Pubkey)
		}
	}
}

type eventCache struct {
	mu   sync.RWMutex
	rb   *ringBuffer[*nostr.Event]
	ids  map[string]*nostr.Event
	keys map[string]*nostr.Event
}

func newEventCache(capacity int) *eventCache {
	return &eventCache{
		rb:   newRingBuffer[*nostr.Event](capacity),
		ids:  make(map[string]*nostr.Event, capacity),
		keys: make(map[string]*nostr.Event, capacity),
	}
}

func (*eventCache) eventKeyRegular(event *nostr.Event) string { return event.ID }

func (*eventCache) eventKeyReplaceable(event *nostr.Event) string {
	return fmt.Sprintf("%s:%d", event.Pubkey, event.Kind)
}

func (*eventCache) eventKeyParameterized(event *nostr.Event) string {
	idx := slices.IndexFunc(event.Tags, func(t nostr.Tag) bool {
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

func (c *eventCache) eventKey(event *nostr.Event) (key string, ok bool) {
	if kind := event.Kind; kind == 0 || kind == 3 || 10000 <= kind && kind < 20000 {
		return c.eventKeyReplaceable(event), true
	} else if 20000 <= kind && kind < 30000 {
		return "", false
	} else if 30000 <= kind && kind < 40000 {
		key := c.eventKeyParameterized(event)
		return key, key != ""
	}

	return c.eventKeyRegular(event), true
}

func (c *eventCache) Add(event *nostr.Event) (added bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ids[event.ID] != nil {
		return
	}
	key, ok := c.eventKey(event)
	if !ok {
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
	c.mu.Lock()
	defer c.mu.Unlock()

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
	c.mu.Lock()
	defer c.mu.RUnlock()

	event := c.keys[naddr]
	if event == nil || event.Pubkey != pubkey {
		return
	}
	delete(c.ids, event.ID)
	delete(c.keys, naddr)
}

func (c *eventCache) Find(filters []*nostr.ReqFilter) []*nostr.Event {
	var ret []*nostr.Event
	matcher := NewReqFiltersEventMatchers(filters)

	c.mu.RLock()
	defer c.mu.RUnlock()

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
	recv <-chan nostr.ClientMsg,
	send chan<- nostr.ServerMsg,
) error {
	return newMergeHandlerSession(h).Handle(r, recv, send)
}

type mergeHandlerSession struct {
	h         *MergeHandler
	recvs     []chan nostr.ClientMsg
	sends     []chan nostr.ServerMsg
	preSendCh chan nostr.ServerMsg
}

func newMergeHandlerSession(h *MergeHandler) *mergeHandlerSession {
	recvs := make([]chan nostr.ClientMsg, len(h.hs))
	sends := make([]chan nostr.ServerMsg, len(h.hs))
	for i := 0; i < len(h.hs); i++ {
		recvs[i] = make(chan nostr.ClientMsg, 1)
		sends[i] = make(chan nostr.ServerMsg, 1)
	}

	return &mergeHandlerSession{
		h:         h,
		recvs:     recvs,
		sends:     sends,
		preSendCh: make(chan nostr.ServerMsg, len(h.hs)),
	}
}

func (ss *mergeHandlerSession) Handle(
	r *http.Request,
	recv <-chan nostr.ClientMsg,
	send chan<- nostr.ServerMsg,
) error {
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	defer cancel()

	var wg sync.WaitGroup

	errs := make(chan error, 3)

	wg.Add(4)
	func() {
		defer wg.Done()
		defer cancel()
		errs <- ss.runHandlers(r)
	}()
	func() {
		defer wg.Done()
		defer cancel()
		ss.mergeSends(ctx)
	}()
	func() {
		defer wg.Done()
		defer cancel()
		errs <- ss.handleRecv(ctx, recv)
	}()
	func() {
		defer wg.Done()
		defer cancel()
		errs <- ss.handleSend(ctx, send)
	}()
	wg.Wait()

	return errors.Join(<-errs, <-errs, <-errs)
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

func (ss *mergeHandlerSession) mergeSend(ctx context.Context, sends []chan nostr.ServerMsg) {
	if len(sends) == 0 {
		return
	}

	idx := len(sends) - 1

	go ss.mergeSend(ctx, sends[:idx])

	select {
	case <-ctx.Done():
		return
	case ss.preSendCh <- <-sends[idx]:
	}
}

func (ss *mergeHandlerSession) handleRecv(ctx context.Context, recv <-chan nostr.ClientMsg) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				return ErrRecvClosed
			}
			if err := ss.handleRecvMsg(ctx, msg); err != nil {
				return err
			}
		}
	}
}

func (ss *mergeHandlerSession) handleRecvMsg(ctx context.Context, msg nostr.ClientMsg) error {
	switch msg := msg.(type) {
	case *nostr.ClientEventMsg:
		return ss.handleRecvEventMsg(ctx, msg)
	case *nostr.ClientReqMsg:
		return ss.handleRecvReqMsg(ctx, msg)
	case *nostr.ClientCloseMsg:
		return ss.handleRecvCloseMsg(ctx, msg)
	case *nostr.ClientAuthMsg:
		return ss.handleRecvAuthMsg(ctx, msg)
	case *nostr.ClientCountMsg:
		return ss.handleRecvCountMsg(ctx, msg)
	default:
		return ss.broadcastRecvs(ctx, msg)
	}
}

func (ss *mergeHandlerSession) handleRecvEventMsg(
	ctx context.Context,
	msg *nostr.ClientEventMsg,
) error {
	panic("unimplemneted")
}

func (ss *mergeHandlerSession) handleRecvReqMsg(
	ctx context.Context,
	msg *nostr.ClientReqMsg,
) error {
	panic("unimplemneted")
}

func (ss *mergeHandlerSession) handleRecvCloseMsg(
	ctx context.Context,
	msg *nostr.ClientCloseMsg,
) error {
	panic("unimplemneted")
}

func (ss *mergeHandlerSession) handleRecvAuthMsg(
	ctx context.Context,
	msg *nostr.ClientAuthMsg,
) error {
	return ss.broadcastRecvs(ctx, msg)
}

func (ss *mergeHandlerSession) handleRecvCountMsg(
	ctx context.Context,
	msg *nostr.ClientCountMsg,
) error {
	panic("unimplemneted")
}

func (ss *mergeHandlerSession) broadcastRecvs(ctx context.Context, msg nostr.ClientMsg) error {
	for _, r := range ss.recvs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r <- msg:
		}
	}
	return nil
}

func (ss *mergeHandlerSession) handleSend(ctx context.Context, send chan<- nostr.ServerMsg) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg := <-ss.preSendCh:
			if err := ss.handleSendMsg(ctx, send, msg); err != nil {
				return err
			}
		}
	}
}

func (ss *mergeHandlerSession) handleSendMsg(
	ctx context.Context,
	send chan<- nostr.ServerMsg,
	msg nostr.ServerMsg,
) error {
	switch msg := msg.(type) {
	case *nostr.ServerEOSEMsg:
		return ss.handleSendEOSEMsg(ctx, send, msg)
	case *nostr.ServerEventMsg:
		return ss.handleSendEventMsg(ctx, send, msg)
	case *nostr.ServerNoticeMsg:
		return ss.handleSendNoticeMsg(ctx, send, msg)
	case *nostr.ServerOKMsg:
		return ss.handleSendOKMsg(ctx, send, msg)
	case *nostr.ServerAuthMsg:
		return ss.handleSendAuthMsg(ctx, send, msg)
	case *nostr.ServerCountMsg:
		return ss.handleSendCountMsg(ctx, send, msg)
	default:
		send <- msg
		return nil
	}
}

func (ss *mergeHandlerSession) handleSendEOSEMsg(
	ctx context.Context,
	send chan<- nostr.ServerMsg,
	msg *nostr.ServerEOSEMsg,
) error {
	panic("unimplemented")
}

func (ss *mergeHandlerSession) handleSendEventMsg(
	ctx context.Context,
	send chan<- nostr.ServerMsg,
	msg *nostr.ServerEventMsg,
) error {
	panic("unimplemented")
}

func (ss *mergeHandlerSession) handleSendNoticeMsg(
	ctx context.Context,
	send chan<- nostr.ServerMsg,
	msg *nostr.ServerNoticeMsg,
) error {
	panic("unimplemented")
}

func (ss *mergeHandlerSession) handleSendOKMsg(
	ctx context.Context,
	send chan<- nostr.ServerMsg,
	msg *nostr.ServerOKMsg,
) error {
	panic("unimplemented")
}

func (ss *mergeHandlerSession) handleSendAuthMsg(
	ctx context.Context,
	send chan<- nostr.ServerMsg,
	msg *nostr.ServerAuthMsg,
) error {
	panic("unimplemented")
}

func (ss *mergeHandlerSession) handleSendCountMsg(
	ctx context.Context,
	send chan<- nostr.ServerMsg,
	msg *nostr.ServerCountMsg,
) error {
	panic("unimplemented")
}

type EventCreatedAtReqFilterMiddleware func(next Handler) Handler

func NewEventCreatedAtReqFilterMiddleware(
	from, to time.Duration,
) EventCreatedAtReqFilterMiddleware {
	return func(next Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
				ctx := r.Context()

				ch := make(chan nostr.ClientMsg, 1)

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
							if m, ok := msg.(*nostr.ClientEventMsg); ok {
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

	var ids []string
	seen := make(map[string]bool)

	return func(handler Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
				ctx := r.Context()

				ch := make(chan nostr.ClientMsg, 1)

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
							if m, ok := msg.(*nostr.ClientEventMsg); ok {
								if seen[m.Event.ID] {
									continue
								}

								if len(ids) >= buflen {
									if len(ids) > 0 {
										old := ids[0]
										delete(seen, old)
										ids = ids[1:]
									}
								} else {
									ids = append(ids, m.Event.ID)
									seen[m.Event.ID] = true
								}
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

	var keys []string
	seen := make(map[string]bool)

	key := func(subID, eventID string) string { return subID + ":" + eventID }

	return func(handler Handler) Handler {
		return HandlerFunc(
			func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
				ctx := r.Context()

				ch := make(chan nostr.ServerMsg, 1)
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
							if m, ok := msg.(*nostr.ServerEventMsg); ok {
								k := key(m.SubscriptionID, m.Event.ID)

								if seen[k] {
									continue
								}

								if len(keys) >= buflen {
									if len(keys) > 0 {
										old := keys[0]
										delete(seen, old)
										keys = keys[1:]
									}
								} else {
									keys = append(keys, k)
									seen[k] = true
								}
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
