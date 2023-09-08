package mocrelay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
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
	MatchFunc      func(event *nostr.Event) bool
	Ch             chan nostr.ServerMsg
}

func newSubscriber(connID string, msg *nostr.ClientReqMsg, ch chan nostr.ServerMsg) *subscriber {
	return &subscriber{
		ConnID:         connID,
		SubscriptionID: msg.SubscriptionID,
		MatchFunc:      newMatchFunc(msg.Filters),
		Ch:             ch,
	}
}

func (sub *subscriber) SendIfMatch(event *nostr.Event) {
	if sub.MatchFunc(event) {
		sub.Ch <- nostr.NewServerEventMsg(sub.SubscriptionID, event)
	}
}

func filterMatch(f *nostr.Filter, event *nostr.Event) (match bool) {
	match = true

	if f.IDs != nil {
		match = match && slices.ContainsFunc(*f.IDs, func(id string) bool {
			return strings.HasPrefix(event.ID, id)
		})
	}

	if f.Kinds != nil {
		match = match && slices.ContainsFunc(*f.Kinds, func(kind int64) bool {
			return event.Kind == kind
		})
	}

	if f.Authors != nil {
		match = match && slices.ContainsFunc(*f.Authors, func(author string) bool {
			return strings.HasPrefix(event.Pubkey, author)
		})
	}

	if f.Tags != nil {
		for tag, vs := range *f.Tags {
			match = match && slices.ContainsFunc(vs, func(v string) bool {
				return slices.ContainsFunc(event.Tags, func(tagArr nostr.Tag) bool {
					return len(tagArr) >= 1 && tagArr[0] == string(tag[1]) && (len(tagArr) == 1 || strings.HasPrefix(tagArr[1], v))
				})
			})
		}
	}

	if f.Since != nil {
		match = match && *f.Since <= event.CreatedAt
	}

	if f.Until != nil {
		match = match && event.CreatedAt <= *f.Until
	}

	return match

}

func newMatchFunc(filters nostr.Filters) func(*nostr.Event) bool {
	return func(event *nostr.Event) bool {
		return slices.ContainsFunc(filters, func(f *nostr.Filter) bool {
			return filterMatch(f, event)
		})
	}
}

func newMatchCountFunc(filters nostr.Filters) func(*nostr.Event) (match bool, done bool) {
	limited := false
	counter := make([]int64, len(filters))
	for i := 0; i < len(filters); i++ {
		if l := filters[i].Limit; l == nil {
			counter[i] = -1
		} else {
			counter[i] = *l
			limited = true
		}
	}
	d := false

	return func(event *nostr.Event) (match bool, done bool) {
		if d {
			done = true
			return
		}

		done = true
		for i := 0; i < len(filters); i++ {
			m := filterMatch(filters[i], event)
			if m {
				counter[i]--
			}

			done = done && limited && counter[i] < 0
			match = !done && (match || m)
		}

		d = done

		return
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
	cmatch := newMatchCountFunc(filters)

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

		match, done := cmatch(ev)
		if done {
			break
		}
		if match {
			ret = append(ret, ev)
		}
	}

	return ret
}

type MergeHandler struct {
	h1, h2 Handler
}

func NewMergeHandler(h1, h2 Handler) Handler {
	return &MergeHandler{
		h1: h1,
		h2: h2,
	}
}

func (h *MergeHandler) Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
	recv1 := make(chan nostr.ClientMsg, 1)
	recv2 := make(chan nostr.ClientMsg, 1)
	send1 := make(chan nostr.ServerMsg, 1)
	send2 := make(chan nostr.ServerMsg, 1)

	subIDs := make(map[string]int)
	cmatchs := make(map[string]func(*nostr.Event) (bool, bool))
	errs := make(chan error, 2)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- h.h1.Handle(r, recv1, send1)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- h.h2.Handle(r, recv2, send2)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(recv1)
		defer close(recv2)
		h.mergeRecv(recv, recv1, recv2, subIDs, cmatchs)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		h.mergeSend(send, send1, send2, subIDs, cmatchs)
	}()

	wg.Wait()

	return errors.Join(<-errs, <-errs)
}

const (
	mergeHandlerSubIDEOSE = 0
	mergeHandlerSubIDIn   = 1
	mergeHandlerSubIDOut1 = 2
	mergeHandlerSubIDOut2 = 3
)

func (h *MergeHandler) mergeRecv(recv <-chan nostr.ClientMsg, r1, r2 chan nostr.ClientMsg, subIDs map[string]int, cmatchs map[string]func(*nostr.Event) (bool, bool)) {
	for msg := range recv {
		if m, ok := msg.(*nostr.ClientReqMsg); ok {
			subIDs[m.SubscriptionID] = mergeHandlerSubIDIn
			cmatchs[m.SubscriptionID] = newMatchCountFunc(m.Filters)
		}

		r1 <- msg
		r2 <- msg
	}
}

func (h *MergeHandler) mergeSend(send chan<- nostr.ServerMsg, s1, s2 chan nostr.ServerMsg, subIDs map[string]int, cmatchs map[string]func(*nostr.Event) (bool, bool)) {
	lastEvents := make(map[string]*nostr.Event)

	for {
		select {
		case msg, ok := <-s1:
			if !ok {
				h.mergeSend(send, nil, s2, subIDs, cmatchs)
				return
			}
			h.send(send, msg, subIDs, cmatchs, lastEvents)

		case msg, ok := <-s2:
			if !ok {
				h.mergeSend(send, s1, nil, subIDs, cmatchs)
				return
			}
			h.send(send, msg, subIDs, cmatchs, lastEvents)
		}
	}
}

func (h *MergeHandler) send(send chan<- nostr.ServerMsg, msg nostr.ServerMsg, subIDs map[string]int, cmatchs map[string]func(*nostr.Event) (bool, bool), lastEvents map[string]*nostr.Event) {
	switch m := msg.(type) {
	case *nostr.ServerEOSEMsg:
		subIDs[m.SubscriptionID]++
		if subIDs[m.SubscriptionID] == mergeHandlerSubIDOut2 {
			delete(subIDs, m.SubscriptionID)
			delete(lastEvents, m.SubscriptionID)
			delete(cmatchs, m.SubscriptionID)
			send <- msg
		}

	case *nostr.ServerEventMsg:
		if subIDs[m.SubscriptionID] != mergeHandlerSubIDEOSE {
			match, ok := cmatchs[m.SubscriptionID](m.Event)
			if !match || !ok {
				return
			}
		}
		send <- msg

	default:
		send <- msg
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
