//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"reflect"
	"sync"
)

// MergeHandler merges multiple handlers into one.
// It runs all handlers in parallel and merges their responses.
type MergeHandler struct {
	handlers []Handler
}

// NewMergeHandler creates a new MergeHandler.
func NewMergeHandler(handlers ...Handler) Handler {
	return &MergeHandler{handlers: handlers}
}

func (h *MergeHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	numHandlers := len(h.handlers)

	// Create channels for each child handler
	childRecvs := make([]chan *ClientMsg, numHandlers)
	childSends := make([]chan *ServerMsg, numHandlers)
	for i := range h.handlers {
		childRecvs[i] = make(chan *ClientMsg, 10)
		childSends[i] = make(chan *ServerMsg, 10)
	}

	// Start child handlers
	var wg sync.WaitGroup
	for i, handler := range h.handlers {
		wg.Add(1)
		go func(i int, handler Handler) {
			defer wg.Done()
			defer close(childSends[i])
			handler.ServeNostr(ctx, childSends[i], childRecvs[i])
		}(i, handler)
	}

	// Session state for merging
	session := newMergeSession(numHandlers)

	// Build select cases dynamically
	// Case 0: ctx.Done()
	// Case 1: recv
	// Case 2..N+1: childSends[0..N-1]
	cases := make([]reflect.SelectCase, 2+numHandlers)
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	}
	cases[1] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(recv),
	}
	for i := range numHandlers {
		cases[2+i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(childSends[i]),
		}
	}

	for {
		chosen, value, ok := reflect.Select(cases)

		switch chosen {
		case 0: // ctx.Done()
			return ctx.Err()

		case 1: // recv
			if !ok {
				return nil
			}
			msg := value.Interface().(*ClientMsg)
			// Broadcast to all children
			for _, ch := range childRecvs {
				select {
				case ch <- msg:
				default:
				}
			}
			// Track pending responses
			if msg.Type == MsgTypeEvent {
				session.startEventResponse(msg.Event.ID)
			}

		default: // childSends[chosen-2]
			if !ok {
				// Child closed, disable this case
				cases[chosen].Chan = reflect.ValueOf(nil)
				continue
			}
			msg := value.Interface().(*ServerMsg)
			// Process merged response
			result := session.processResponse(msg)
			if result != nil {
				select {
				case send <- result:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// mergeSession manages the state for merging responses.
type mergeSession struct {
	mu           sync.Mutex
	numHandlers  int
	pendingOKs   map[string]*pendingOK   // eventID -> pending OK state
	pendingEOSEs map[string]*pendingEOSE // subscriptionID -> pending EOSE state
}

type pendingOK struct {
	received int
	accepted bool
	message  string
}

type pendingEOSE struct {
	received     int
	seenEventIDs map[string]bool // for deduplication before EOSE
}

func newMergeSession(numHandlers int) *mergeSession {
	return &mergeSession{
		numHandlers:  numHandlers,
		pendingOKs:   make(map[string]*pendingOK),
		pendingEOSEs: make(map[string]*pendingEOSE),
	}
}

func (s *mergeSession) startEventResponse(eventID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingOKs[eventID] = &pendingOK{
		received: 0,
		accepted: true, // Start optimistic
		message:  "",
	}
}

func (s *mergeSession) processResponse(msg *ServerMsg) *ServerMsg {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch msg.Type {
	case MsgTypeOK:
		pending, ok := s.pendingOKs[msg.EventID]
		if !ok {
			return nil // Unexpected OK
		}
		pending.received++
		if !msg.Accepted {
			pending.accepted = false
			pending.message = msg.Message
		}
		// Check if all handlers have responded
		if pending.received >= s.numHandlers {
			delete(s.pendingOKs, msg.EventID)
			return NewServerOKMsg(msg.EventID, pending.accepted, pending.message)
		}
		return nil // Still waiting for more responses

	case MsgTypeEvent:
		// Deduplicate events before EOSE
		subID := msg.SubscriptionID
		pending, ok := s.pendingEOSEs[subID]
		if !ok {
			// First event for this subscription, create pending state
			s.pendingEOSEs[subID] = &pendingEOSE{
				received:     0,
				seenEventIDs: map[string]bool{msg.Event.ID: true},
			}
			return msg
		}
		// Check if we've already seen this event
		if pending.seenEventIDs[msg.Event.ID] {
			return nil // Duplicate, drop
		}
		pending.seenEventIDs[msg.Event.ID] = true
		return msg

	case MsgTypeEOSE:
		pending, ok := s.pendingEOSEs[msg.SubscriptionID]
		if !ok {
			// First EOSE for this subscription (no events received yet)
			s.pendingEOSEs[msg.SubscriptionID] = &pendingEOSE{
				received:     1,
				seenEventIDs: make(map[string]bool),
			}
			if s.numHandlers == 1 {
				delete(s.pendingEOSEs, msg.SubscriptionID)
				return msg
			}
			return nil
		}
		pending.received++
		// Check if all handlers have sent EOSE
		if pending.received >= s.numHandlers {
			delete(s.pendingEOSEs, msg.SubscriptionID)
			return NewServerEOSEMsg(msg.SubscriptionID)
		}
		return nil // Still waiting for more EOSEs

	default:
		// For now, pass through other messages
		return msg
	}
}
