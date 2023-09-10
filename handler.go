package mocrelay

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/high-moctane/mocrelay/nostr"
)

var (
	ErrRecvClosed = errors.New("recv client msg channel has been closed")
	ErrSendClosed = errors.New("send server msg channel has been closed")
)

type Handler interface {
	Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error
}

type HandlerFunc func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error

func (f HandlerFunc) Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
	return f(r, recv, send)
}

var ErrRouterStop = errors.New("router stopped")

type RouterOption struct {
	// TODO(high-moctane) Add logger
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

func (router *Router) Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
	ctx, cancel := context.WithCancel(r.Context())

	connID := uuid.NewString()
	defer router.subs.UnsubscribeAll(connID)

	msgCh := make(chan nostr.ServerMsg, 1)

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

func (router *Router) serveRecv(ctx context.Context, connID string, recv <-chan nostr.ClientMsg, msgCh chan nostr.ServerMsg) error {
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

func (router *Router) recv(ctx context.Context, connID string, msg nostr.ClientMsg, msgCh chan nostr.ServerMsg) {
	switch m := msg.(type) {
	case *nostr.ClientReqMsg:
		router.recvClientReqMsg(ctx, connID, m, msgCh)

	case *nostr.ClientEventMsg:
		router.recvClientEventMsg(ctx, connID, m, msgCh)

	case *nostr.ClientCloseMsg:
		router.recvClientCloseMsg(ctx, connID, m)
	}
}

func (router *Router) recvClientReqMsg(ctx context.Context, connID string, msg *nostr.ClientReqMsg, msgCh chan nostr.ServerMsg) {
	sub := newSubscriber(connID, msg, msgCh)
	router.subs.Subscribe(sub)
	msgCh <- nostr.NewServerEOSEMsg(msg.SubscriptionID)
}

func (router *Router) recvClientEventMsg(ctx context.Context, connID string, msg *nostr.ClientEventMsg, msgCh chan nostr.ServerMsg) {
	router.subs.Publish(msg.Event)
	msgCh <- nostr.NewServerOKMsg(msg.Event.ID, true, nostr.ServerOKMsgPrefixNoPrefix, "")
}

func (router *Router) recvClientCloseMsg(ctx context.Context, connID string, msg *nostr.ClientCloseMsg) {
	router.subs.Unsubscribe(connID, msg.SubscriptionID)
}

func (router *Router) serveSend(ctx context.Context, connID string, send chan<- nostr.ServerMsg, msgCh chan nostr.ServerMsg) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg := <-msgCh:
			send <- msg
		}
	}
}

type subscriber struct {
	ConnID         string
	SubscriptionID string
	Matcher        nostr.Matchers
	Ch             chan nostr.ServerMsg
}

func newSubscriber(connID string, msg *nostr.ClientReqMsg, ch chan nostr.ServerMsg) *subscriber {
	return &subscriber{
		ConnID:         connID,
		SubscriptionID: msg.SubscriptionID,
		Matcher:        nostr.NewMatchers(msg.Filters),
		Ch:             ch,
	}
}

func (sub *subscriber) SendIfMatch(event *nostr.Event) {
	if sub.Matcher.Match(event) {
		sub.Ch <- nostr.NewServerEventMsg(sub.SubscriptionID, event)
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

func (h *CacheHandler) Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
	for msg := range recv {
		switch m := msg.(type) {
		case *nostr.ClientEventMsg:
			e := m.Event
			if e.Kind == 5 {
				h.kind5(e)
			}
			h.c.Add(e)

		case *nostr.ClientReqMsg:
			for _, e := range h.c.Find(m.Filters) {
				send <- nostr.NewServerEventMsg(m.SubscriptionID, e)
			}
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

func (c *eventCache) Add(event *nostr.Event) {
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

func (c *eventCache) Find(filters nostr.Filters) []*nostr.Event {
	var ret []*nostr.Event
	matcher := nostr.NewMatchers(filters)

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

func NewMergeHandler(h1, h2 Handler, hs ...Handler) Handler {
	return &MergeHandler{
		hs: append([]Handler{h1, h2}, hs...),
	}
}

func (h *MergeHandler) Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	recvs := make([]chan nostr.ClientMsg, len(h.hs))
	sends := make([]chan nostr.ServerMsg, len(h.hs))
	smerge := make(chan *mergeHandlerSendMsg, 1)
	errs := make(chan error, len(h.hs))

	for i := 0; i < len(h.hs); i++ {
		recvs[i] = make(chan nostr.ClientMsg, 1)
		sends[i] = make(chan nostr.ServerMsg, 1)
	}

	var wg sync.WaitGroup
	wg.Add(len(h.hs))
	for i := 0; i < len(h.hs); i++ {
		go func(i int) {
			defer wg.Done()
			defer cancel()
			errs <- h.hs[i].Handle(r.WithContext(ctx), recvs[i], sends[i])
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		h.mergeSends(ctx, smerge, sends)
	}()

	h.serve(ctx, recv, recvs, send, smerge)

	wg.Wait()
	close(errs)
	var err error
	for e := range errs {
		err = errors.Join(err, e)
	}

	return err
}

type mergeHandlerSendMsg struct {
	Idx int
	Msg nostr.ServerMsg
}

type mergeHandlerReqState struct {
	EOSE      []bool
	LastEvent *nostr.Event
	SeenIDs   map[string]bool
	Matcher   nostr.Matchers
}

type mergeHandlerCountState struct {
	counts []*nostr.ServerCountMsg
}

func newMergeHandlerCountState(length int) *mergeHandlerCountState {
	return &mergeHandlerCountState{
		counts: make([]*nostr.ServerCountMsg, length),
	}
}

func (stat *mergeHandlerCountState) AddCount(idx int, c *nostr.ServerCountMsg) {
	if stat == nil {
		return
	}
	stat.counts[idx] = c
}

func (stat *mergeHandlerCountState) Ready() bool {
	return stat != nil && !slices.Contains(stat.counts, nil)
}

func (stat *mergeHandlerCountState) Max() *nostr.ServerCountMsg {
	return slices.MaxFunc(stat.counts, func(a, b *nostr.ServerCountMsg) int {
		return cmp.Compare(a.Count, b.Count)
	})
}

func newMergeHandlerReqState(length int, filters nostr.Filters) *mergeHandlerReqState {
	return &mergeHandlerReqState{
		EOSE:      make([]bool, length),
		LastEvent: nil,
		SeenIDs:   make(map[string]bool),
		Matcher:   nostr.NewMatchers(filters),
	}
}

func (stat *mergeHandlerReqState) AllEOSE() bool {
	if stat == nil {
		return true
	}
	return !slices.Contains(stat.EOSE, false)
}

func (stat *mergeHandlerReqState) AddEOSE(idx int) {
	if stat == nil {
		return
	}
	stat.EOSE[idx] = true
}

func (stat *mergeHandlerReqState) Match(msg *mergeHandlerSendMsg) bool {
	if stat == nil {
		return true
	}

	m := msg.Msg.(*nostr.ServerEventMsg)
	ev := m.Event

	if stat.EOSE[msg.Idx] {
		return false
	}

	if stat.LastEvent != nil && ev.CreatedAt < stat.LastEvent.CreatedAt {
		return false
	}

	if stat.SeenIDs[ev.ID] {
		return false
	}
	stat.SeenIDs[ev.ID] = true

	if stat.Matcher.Done() {
		return false
	}

	if !stat.Matcher.CountMatch(ev) {
		return false
	}

	stat.LastEvent = ev
	return true
}

func (h *MergeHandler) serve(
	ctx context.Context,
	recv <-chan nostr.ClientMsg,
	recvs []chan nostr.ClientMsg,
	send chan<- nostr.ServerMsg,
	smerge chan *mergeHandlerSendMsg,
) {
	reqStats := make(map[string]*mergeHandlerReqState)
	wantOK := make(map[string]bool)
	countStats := make(map[string]*mergeHandlerCountState)

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-recv:
			if !ok {
				for _, r := range recvs {
					close(r)
				}
				return
			}

			switch msg := msg.(type) {
			case *nostr.ClientReqMsg:
				reqStats[msg.SubscriptionID] = newMergeHandlerReqState(len(h.hs), msg.Filters)

			case *nostr.ClientCloseMsg:
				delete(reqStats, msg.SubscriptionID)

			case *nostr.ClientEventMsg:
				wantOK[msg.Event.ID] = true

			case *nostr.ClientCountMsg:
				countStats[msg.SubscriptionID] = newMergeHandlerCountState(len(h.hs))

			}

			for _, r := range recvs {
				r <- msg
			}

		case msg := <-smerge:
			switch m := msg.Msg.(type) {
			case *nostr.ServerEventMsg:
				if reqStats[m.SubscriptionID].Match(msg) {
					send <- msg.Msg
				}

			case *nostr.ServerOKMsg:
				if wantOK[m.EventID] {
					delete(wantOK, m.EventID)
					send <- msg.Msg
				}

			case *nostr.ServerEOSEMsg:
				stat := reqStats[m.SubscriptionID]
				stat.AddEOSE(msg.Idx)
				if stat.AllEOSE() {
					delete(reqStats, m.SubscriptionID)
					send <- msg.Msg
				}

			case *nostr.ServerCountMsg:
				stat := countStats[m.SubscriptionID]
				stat.AddCount(msg.Idx, m)
				if stat.Ready() {
					maxMsg := stat.Max()
					delete(countStats, m.SubscriptionID)
					send <- maxMsg
				}

			default:
				send <- msg.Msg
			}
		}
	}
}

func (h *MergeHandler) mergeSends(ctx context.Context, smerge chan *mergeHandlerSendMsg, sends []chan nostr.ServerMsg) chan struct{} {
	if len(sends) == 0 {
		return nil
	}

	idx := len(sends) - 1

	for {
		select {
		case <-ctx.Done():
			return nil

		case msg := <-sends[idx]:
			smerge <- &mergeHandlerSendMsg{
				Idx: idx,
				Msg: msg,
			}

		case <-h.mergeSends(ctx, smerge, sends[:idx]):
		}
	}
}

type EventCreatedAtFilterMiddleware func(next Handler) Handler

func NewEventCreatedAtFilterMiddleware(from, to time.Duration) EventCreatedAtFilterMiddleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
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
		})
	}
}

type RecvEventUniquefyMiddleware func(Handler) Handler

func NewRecvEventUniquefyMiddleware(buflen int) RecvEventUniquefyMiddleware {
	if buflen < 0 {
		panic(fmt.Sprintf("RecvEventUniquefyMiddleware buflen must be 0 or more but got %v", buflen))
	}

	var ids []string
	seen := make(map[string]bool)

	return func(handler Handler) Handler {
		return HandlerFunc(func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
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
		})
	}
}

type SendEventUniquefyMiddleware func(Handler) Handler

func NewSendEventUniquefyMiddleware(buflen int) SendEventUniquefyMiddleware {
	if buflen < 0 {
		panic(fmt.Sprintf("SendEventUniquefyMiddleware buflen must be 0 or more but got %v", buflen))
	}

	var keys []string
	seen := make(map[string]bool)

	key := func(subID, eventID string) string { return subID + ":" + eventID }

	return func(handler Handler) Handler {
		return HandlerFunc(func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
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
		})
	}
}
