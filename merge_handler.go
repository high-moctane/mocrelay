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
			if msg.Type == MsgTypeReq {
				session.startReqResponse(msg.SubscriptionID, msg.Filters)
			}

		default: // childSends[chosen-2]
			if !ok {
				// Child closed, disable this case
				cases[chosen].Chan = reflect.ValueOf(nil)
				continue
			}
			msg := value.Interface().(*ServerMsg)
			// Process merged response
			results := session.processResponse(msg)
			for _, result := range results {
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
	mu            sync.Mutex
	numHandlers   int
	pendingOKs    map[string]*pendingOK   // eventID -> pending OK state
	pendingEOSEs  map[string]*pendingEOSE // subscriptionID -> pending EOSE state
	completedSubs map[string]bool         // subscriptionID -> true if EOSE already sent
}

type pendingOK struct {
	received int
	accepted bool
	message  string
}

type pendingEOSE struct {
	received     int
	seenEventIDs map[string]bool // for deduplication before EOSE
	// For sort order enforcement (created_at DESC, id ASC for tiebreak)
	lastCreatedAt int64
	lastID        string
	hasSentEvent  bool
	// For limit enforcement
	limit      int64 // 0 means no limit
	eventCount int64
	eoseSent   bool // true if EOSE already sent (due to limit)
}

func newMergeSession(numHandlers int) *mergeSession {
	return &mergeSession{
		numHandlers:   numHandlers,
		pendingOKs:    make(map[string]*pendingOK),
		pendingEOSEs:  make(map[string]*pendingEOSE),
		completedSubs: make(map[string]bool),
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

func (s *mergeSession) startReqResponse(subID string, filters []*ReqFilter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Extract limit from the first filter (mocrelay's convention)
	var limit int64
	if len(filters) > 0 && filters[0].Limit != nil {
		limit = *filters[0].Limit
	}

	s.pendingEOSEs[subID] = &pendingEOSE{
		received:     0,
		seenEventIDs: make(map[string]bool),
		limit:        limit,
		eventCount:   0,
		eoseSent:     false,
	}
}

func (s *mergeSession) processResponse(msg *ServerMsg) []*ServerMsg {
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
			return []*ServerMsg{NewServerOKMsg(msg.EventID, pending.accepted, pending.message)}
		}
		return nil // Still waiting for more responses

	case MsgTypeEvent:
		// Deduplicate and enforce sort order for events before EOSE
		subID := msg.SubscriptionID
		eventCreatedAt := msg.Event.CreatedAt.Unix()
		eventID := msg.Event.ID

		// If EOSE already sent (due to limit), drop the event
		if s.completedSubs[subID] {
			return nil
		}

		pending, ok := s.pendingEOSEs[subID]
		if !ok {
			// First event for this subscription, create pending state
			s.pendingEOSEs[subID] = &pendingEOSE{
				received:      0,
				seenEventIDs:  map[string]bool{eventID: true},
				lastCreatedAt: eventCreatedAt,
				lastID:        eventID,
				hasSentEvent:  true,
				eventCount:    1,
			}
			return []*ServerMsg{msg}
		}

		// If EOSE already sent (due to limit), drop the event
		if pending.eoseSent {
			return nil
		}

		// Check if we've already seen this event (deduplication)
		if pending.seenEventIDs[eventID] {
			return nil // Duplicate, drop
		}

		// Check sort order: created_at DESC, then id ASC for tiebreak
		if pending.hasSentEvent {
			if eventCreatedAt > pending.lastCreatedAt {
				// Newer event after older one - breaks DESC order, drop
				return nil
			}
			if eventCreatedAt == pending.lastCreatedAt && eventID > pending.lastID {
				// Same timestamp but higher ID - breaks ASC tiebreak, drop
				return nil
			}
		}

		// Event passes all checks
		pending.seenEventIDs[eventID] = true
		pending.lastCreatedAt = eventCreatedAt
		pending.lastID = eventID
		pending.hasSentEvent = true
		pending.eventCount++

		// Check if limit is reached
		if pending.limit > 0 && pending.eventCount >= pending.limit {
			// Send EVENT + EOSE together
			pending.eoseSent = true
			delete(s.pendingEOSEs, subID)
			s.completedSubs[subID] = true
			return []*ServerMsg{msg, NewServerEOSEMsg(subID)}
		}

		return []*ServerMsg{msg}

	case MsgTypeEOSE:
		subID := msg.SubscriptionID

		// If EOSE already sent (due to limit), ignore
		if s.completedSubs[subID] {
			return nil
		}

		pending, ok := s.pendingEOSEs[subID]
		if !ok {
			// First EOSE for this subscription (no events received yet)
			s.pendingEOSEs[subID] = &pendingEOSE{
				received:     1,
				seenEventIDs: make(map[string]bool),
			}
			if s.numHandlers == 1 {
				delete(s.pendingEOSEs, subID)
				s.completedSubs[subID] = true
				return []*ServerMsg{msg}
			}
			return nil
		}
		// If EOSE already sent (due to limit), ignore
		if pending.eoseSent {
			return nil
		}
		pending.received++
		// Check if all handlers have sent EOSE
		if pending.received >= s.numHandlers {
			delete(s.pendingEOSEs, subID)
			s.completedSubs[subID] = true
			return []*ServerMsg{NewServerEOSEMsg(subID)}
		}
		return nil // Still waiting for more EOSEs

	default:
		// For now, pass through other messages
		return []*ServerMsg{msg}
	}
}
