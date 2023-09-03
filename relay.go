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

		case msg := <-recv:
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

func newMatchFunc(filters nostr.Filters) func(*nostr.Event) bool {
	return func(event *nostr.Event) bool {
		return slices.ContainsFunc(filters, func(f *nostr.Filter) bool {
			match := true

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
		})
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

type EventCreatedAtFilterMiddleware func(next Handler) Handler

func NewEventCreatedAtFilterMiddleware(from, to time.Duration) EventCreatedAtFilterMiddleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
			ctx := r.Context()

			ch := make(chan nostr.ClientMsg, 1)

			go func() {
				for {
					select {
					case <-ctx.Done():
						return

					case msg := <-recv:
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
				for {
					select {
					case <-ctx.Done():
						return

					case msg := <-recv:
						if m, ok := msg.(*nostr.ClientEventMsg); ok {
							if seen[m.Event.ID] {
								continue
							}

							if len(ids) >= buflen && len(ids) > 0 {
								old := ids[0]
								delete(seen, old)
								ids = ids[1:]
							}
							ids = append(ids, m.Event.ID)
							seen[m.Event.ID] = true
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

	var ids []string
	seen := make(map[string]bool)

	return func(handler Handler) Handler {
		return HandlerFunc(func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
			ctx := r.Context()

			ch := make(chan nostr.ServerMsg, 1)

			go func() {
				for {
					select {
					case <-ctx.Done():
						return

					case msg := <-ch:
						if m, ok := msg.(*nostr.ServerEventMsg); ok {
							if seen[m.Event.ID] {
								continue
							}

							if len(ids) >= buflen && len(ids) > 0 {
								old := ids[0]
								delete(seen, old)
								ids = ids[1:]
							}
							ids = append(ids, m.Event.ID)
							seen[m.Event.ID] = true
						}
						send <- msg
					}
				}
			}()

			return handler.Handle(r, recv, ch)
		})
	}
}
