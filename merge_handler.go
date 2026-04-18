package mocrelay

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// Defaults for [MergeHandlerOptions].
const (
	// DefaultMergeHandlerBroadcastTimeout is the default per-child broadcast
	// timeout used by [MergeHandler] when [MergeHandlerOptions.BroadcastTimeout]
	// is zero.
	DefaultMergeHandlerBroadcastTimeout = 30 * time.Second

	// DefaultMergeHandlerChildRecvBuffer is the default per-child recv
	// channel capacity used by [MergeHandler] when
	// [MergeHandlerOptions.ChildRecvBuffer] is zero.
	DefaultMergeHandlerChildRecvBuffer = 10
)

// sendAll forwards every message to send, honoring ctx cancellation. Returns
// false if ctx was cancelled before all messages were delivered.
func sendAll(ctx context.Context, send chan<- *ServerMsg, msgs []*ServerMsg) bool {
	for _, m := range msgs {
		select {
		case send <- m:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// broadcastAll fans msg out to every live child (entries where childRecvs[i]
// is nil have already been disabled and are skipped).
//
// EVENT messages are best-effort: a full child recv buffer drops the message
// silently, and the client learns of the loss via OK accepted=false (see
// handlerClosed) which prompts a retry.
//
// REQ / COUNT / CLOSE are control messages and must not be silently dropped:
// losing them desynchronizes downstream subscription state. broadcastAll
// blocks up to MergeHandlerOptions.BroadcastTimeout per child; a child that
// fails to drain in time is reported back as dead so the caller can retire it
// (close its case and advance any merged response that depended on it).
//
// The second return value is true if ctx was cancelled mid-broadcast — the
// caller should exit the main loop.
func (h *MergeHandler) broadcastAll(
	ctx context.Context,
	msg *ClientMsg,
	childRecvs []chan *ClientMsg,
) (dead []int, ctxDone bool) {
	if msg.Type == MsgTypeEvent {
		for _, ch := range childRecvs {
			if ch == nil {
				continue
			}
			select {
			case ch <- msg:
			default:
			}
		}
		return nil, false
	}

	// Control message: guaranteed delivery with per-child timeout.
	for i, ch := range childRecvs {
		if ch == nil {
			continue
		}
		// Fast path: try non-blocking send first to avoid allocating a
		// timer in the common case where the child is keeping up.
		select {
		case ch <- msg:
			continue
		default:
		}
		// Slow path: wait up to BroadcastTimeout.
		t := time.NewTimer(h.broadcastTimeout)
		select {
		case ch <- msg:
			t.Stop()
		case <-t.C:
			dead = append(dead, i)
		case <-ctx.Done():
			t.Stop()
			return dead, true
		}
	}
	return dead, false
}

// MergeHandlerOptions configures a [MergeHandler].
//
// All fields are optional. A nil *MergeHandlerOptions is equivalent to a
// zero-valued MergeHandlerOptions and means "use defaults for everything".
type MergeHandlerOptions struct {
	// BroadcastTimeout caps how long MergeHandler waits when forwarding a
	// control message (REQ / COUNT / CLOSE) to a single child handler whose
	// recv channel is full. EVENT messages are always best-effort and are
	// not affected by this timeout (a dropped EVENT is signalled to the
	// client via OK accepted=false, prompting a retry).
	//
	// When the timeout fires, the slow child is treated as dead: any pending
	// OK / EOSE / COUNT it owed is advanced as if the handler had exited.
	// The remaining healthy handlers continue to serve the client.
	//
	// A zero value selects [DefaultMergeHandlerBroadcastTimeout] (30s).
	BroadcastTimeout time.Duration

	// ChildRecvBuffer sizes the per-child recv channel that MergeHandler
	// uses to fan client messages out. A larger buffer absorbs brief
	// bursts; a smaller buffer surfaces stuck downstreams sooner via
	// BroadcastTimeout. Handlers that wrap slow external resources (e.g.
	// proxying upstream relays) should still absorb short-term contention
	// internally — MergeHandler's role is to detect persistent stalls,
	// not to be the only buffer in the chain.
	//
	// A zero value selects [DefaultMergeHandlerChildRecvBuffer] (10).
	ChildRecvBuffer int
}

// MergeHandler merges multiple handlers into one.
// It runs all handlers in parallel and merges their responses.
type MergeHandler struct {
	handlers         []Handler
	broadcastTimeout time.Duration
	childRecvBuffer  int
}

// NewMergeHandler creates a new MergeHandler that fans messages out to every
// handler in handlers and merges their responses. Pass nil for opts to use
// defaults.
func NewMergeHandler(handlers []Handler, opts *MergeHandlerOptions) Handler {
	timeout := DefaultMergeHandlerBroadcastTimeout
	buf := DefaultMergeHandlerChildRecvBuffer
	if opts != nil {
		if opts.BroadcastTimeout > 0 {
			timeout = opts.BroadcastTimeout
		}
		if opts.ChildRecvBuffer > 0 {
			buf = opts.ChildRecvBuffer
		}
	}
	return &MergeHandler{
		handlers:         handlers,
		broadcastTimeout: timeout,
		childRecvBuffer:  buf,
	}
}

// ServeNostr implements [Handler].
func (h *MergeHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	numHandlers := len(h.handlers)

	// Create channels for each child handler. childRecvs is sized by
	// MergeHandlerOptions.ChildRecvBuffer; childSends is fixed because the
	// merger drains it directly from the main loop and no buffering matters
	// beyond a small absorber.
	childRecvs := make([]chan *ClientMsg, numHandlers)
	childSends := make([]chan *ServerMsg, numHandlers)
	for i := range h.handlers {
		childRecvs[i] = make(chan *ClientMsg, h.childRecvBuffer)
		childSends[i] = make(chan *ServerMsg, 10)
	}

	// Error channel for collecting child handler errors
	errs := make(chan error, numHandlers)

	// Start child handlers
	var wg sync.WaitGroup
	for i, handler := range h.handlers {
		wg.Add(1)
		go func(i int, handler Handler) {
			defer wg.Done()
			defer close(childSends[i])
			if err := handler.ServeNostr(ctx, childSends[i], childRecvs[i]); err != nil {
				errs <- fmt.Errorf("handler %d: %w", i, err)
			}
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

	var loopErr error

loop:
	for {
		chosen, value, ok := reflect.Select(cases)

		switch chosen {
		case 0: // ctx.Done()
			loopErr = ctx.Err()
			break loop

		case 1: // recv
			if !ok {
				break loop
			}
			msg := value.Interface().(*ClientMsg)
			// Track pending responses BEFORE broadcasting. This ordering
			// matters: if broadcastAll declares a child dead (timeout), we
			// immediately call session.handlerClosed which advances any
			// pending OK/EOSE/COUNT for that handler. Those pendings must
			// already be registered.
			switch msg.Type {
			case MsgTypeEvent:
				session.startEventResponse(msg.Event.ID)
			case MsgTypeReq:
				session.startReqResponse(msg.SubscriptionID, msg.Filters)
			case MsgTypeCount:
				session.startCountResponse(msg.SubscriptionID)
			case MsgTypeClose:
				session.closeSubscription(msg.SubscriptionID)
			}
			// Fan out to all live children. Dead indices come back so we
			// can disable their cases and free the client from any pending
			// merged response that was waiting on them.
			deadIndices, ctxDone := h.broadcastAll(ctx, msg, childRecvs)
			if ctxDone {
				loopErr = ctx.Err()
				break loop
			}
			for _, idx := range deadIndices {
				cases[2+idx].Chan = reflect.ValueOf(nil)
				childRecvs[idx] = nil
				results := session.handlerClosed(idx)
				if !sendAll(ctx, send, results) {
					loopErr = ctx.Err()
					break loop
				}
			}

		default: // childSends[chosen-2]
			handlerIndex := chosen - 2
			if !ok {
				// Child closed: disable this case and advance any pending
				// state still waiting on this handler so the client isn't
				// left hanging on OK / EOSE / COUNT.
				cases[chosen].Chan = reflect.ValueOf(nil)
				results := session.handlerClosed(handlerIndex)
				if !sendAll(ctx, send, results) {
					loopErr = ctx.Err()
					break loop
				}
				continue
			}
			msg := value.Interface().(*ServerMsg)
			results := session.processResponse(msg, handlerIndex)
			if !sendAll(ctx, send, results) {
				loopErr = ctx.Err()
				break loop
			}
		}
	}

	// Wait for all child handlers to finish and collect their errors
	wg.Wait()
	close(errs)

	var handlerErrs error
	for e := range errs {
		handlerErrs = errors.Join(handlerErrs, e)
	}

	return errors.Join(loopErr, handlerErrs)
}

// mergeSession manages the state for merging responses.
type mergeSession struct {
	mu            sync.Mutex
	numHandlers   int
	pendingOKs    map[string]*pendingOK    // eventID -> pending OK state
	pendingEOSEs  map[string]*pendingEOSE  // subscriptionID -> pending EOSE state
	pendingCounts map[string]*pendingCount // subscriptionID -> pending COUNT state
	completedSubs map[string]bool          // subscriptionID -> true if EOSE already sent
}

type pendingOK struct {
	received         int
	accepted         bool
	message          string
	handlerResponded []bool // per-handler dedup; also used by handlerClosed
}

type pendingCount struct {
	received         int
	maxCount         uint64
	handlerResponded []bool // per-handler dedup; also used by handlerClosed
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
	// Track which handlers have sent EOSE (for real-time events after EOSE)
	handlerEOSESent []bool
}

func newMergeSession(numHandlers int) *mergeSession {
	return &mergeSession{
		numHandlers:   numHandlers,
		pendingOKs:    make(map[string]*pendingOK),
		pendingEOSEs:  make(map[string]*pendingEOSE),
		pendingCounts: make(map[string]*pendingCount),
		completedSubs: make(map[string]bool),
	}
}

func (s *mergeSession) startEventResponse(eventID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingOKs[eventID] = &pendingOK{
		received:         0,
		accepted:         true, // Start optimistic
		message:          "",
		handlerResponded: make([]bool, s.numHandlers),
	}
}

func (s *mergeSession) startCountResponse(subID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingCounts[subID] = &pendingCount{
		received:         0,
		maxCount:         0,
		handlerResponded: make([]bool, s.numHandlers),
	}
}

func (s *mergeSession) startReqResponse(subID string, filters []*ReqFilter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear any previous completion state for this subID. NIP-01 allows
	// clients to re-use a sub_id without sending CLOSE first; the new REQ
	// supersedes the previous subscription. Without this, dedup/sort/limit
	// would be bypassed on the second query because events would fall into
	// the completedSubs pass-through path.
	delete(s.completedSubs, subID)

	// Extract limit from the first filter (mocrelay's convention)
	var limit int64
	if len(filters) > 0 && filters[0].Limit != nil {
		limit = *filters[0].Limit
	}

	s.pendingEOSEs[subID] = &pendingEOSE{
		received:        0,
		seenEventIDs:    make(map[string]bool),
		limit:           limit,
		eventCount:      0,
		eoseSent:        false,
		handlerEOSESent: make([]bool, s.numHandlers),
	}
}

// closeSubscription cleans up all state for a subscription.
// Called when CLOSE message is received.
func (s *mergeSession) closeSubscription(subID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.pendingEOSEs, subID)
	delete(s.pendingCounts, subID)
	delete(s.completedSubs, subID)
}

// handlerClosed accounts for a child Handler that terminated before completing
// its share of the pending responses. Any pending OK / EOSE / COUNT still
// waiting on this handler is advanced as if "no further response will come";
// if that brings the response count up to numHandlers, the merged message is
// emitted now. Without this, a single child Handler returning early (disk
// error, network drop in a proxy setup, ...) would leave the client waiting
// forever for OK / EOSE / COUNT.
func (s *mergeSession) handlerClosed(handlerIndex int) []*ServerMsg {
	s.mu.Lock()
	defer s.mu.Unlock()

	var results []*ServerMsg

	for eventID, pending := range s.pendingOKs {
		if pending.handlerResponded[handlerIndex] {
			continue
		}
		pending.handlerResponded[handlerIndex] = true
		pending.received++
		if pending.received >= s.numHandlers {
			results = append(results,
				NewServerOKMsg(eventID, pending.accepted, pending.message))
			delete(s.pendingOKs, eventID)
		}
	}

	for subID, pending := range s.pendingEOSEs {
		if pending.eoseSent || pending.handlerEOSESent[handlerIndex] {
			continue
		}
		pending.handlerEOSESent[handlerIndex] = true
		pending.received++
		if pending.received >= s.numHandlers {
			results = append(results, NewServerEOSEMsg(subID))
			delete(s.pendingEOSEs, subID)
			s.completedSubs[subID] = true
		}
	}

	for subID, pending := range s.pendingCounts {
		if pending.handlerResponded[handlerIndex] {
			continue
		}
		pending.handlerResponded[handlerIndex] = true
		pending.received++
		if pending.received >= s.numHandlers {
			results = append(results,
				NewServerCountMsg(subID, pending.maxCount, nil))
			delete(s.pendingCounts, subID)
		}
	}

	return results
}

func (s *mergeSession) processResponse(msg *ServerMsg, handlerIndex int) []*ServerMsg {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch msg.Type {
	case MsgTypeOK:
		pending, ok := s.pendingOKs[msg.EventID]
		if !ok {
			return nil // Unexpected OK
		}
		// Guard against duplicate responses from the same handler (contract
		// violation), and required so handlerClosed can advance pending state
		// without double-counting.
		if pending.handlerResponded[handlerIndex] {
			return nil
		}
		pending.handlerResponded[handlerIndex] = true
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

		// If EOSE already sent (all handlers finished), pass through without dedup/sort (real-time events)
		if s.completedSubs[subID] {
			return []*ServerMsg{msg}
		}

		pending, ok := s.pendingEOSEs[subID]
		if !ok {
			// Unknown sub_id: no REQ has registered this subscription, or it
			// was already CLOSE'd. A well-behaved Handler only emits EVENTs
			// for active subscriptions, so this is either a stale message
			// arriving after CLOSE cleanup or a contract violation. Drop
			// silently — never resurrect subscription state from a child
			// message, otherwise CLOSE could not actually stop the flow and
			// limit/dedup state would be initialized without filter info.
			return nil
		}

		// If EOSE already sent (due to limit), drop the event
		if pending.eoseSent {
			return nil
		}

		// If this handler has already sent EOSE, pass through (real-time event).
		// We still consult seenEventIDs so a real-time event isn't sent twice
		// when another handler has already delivered (or later delivers) the
		// same event id.
		if pending.handlerEOSESent[handlerIndex] {
			if pending.seenEventIDs[eventID] {
				return nil
			}
			pending.seenEventIDs[eventID] = true
			return []*ServerMsg{msg}
		}

		// Check if we've already seen this event (deduplication)
		if pending.seenEventIDs[eventID] {
			return nil // Duplicate, drop
		}

		// Check sort order: created_at DESC, then id ASC for tiebreak.
		// Storage yields events in (created_at DESC, id ASC) order, so an
		// in-order event must be older than lastCreatedAt, or share
		// lastCreatedAt with an id strictly greater than lastID.
		if pending.hasSentEvent {
			if eventCreatedAt > pending.lastCreatedAt {
				// Newer event after older one - breaks DESC order, drop
				return nil
			}
			if eventCreatedAt == pending.lastCreatedAt && eventID < pending.lastID {
				// Same timestamp but lower id - breaks ASC tiebreak, drop
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
			// Unknown sub_id: no active REQ, or already CLOSE'd. Drop the
			// stale/late EOSE so it can't resurrect subscription state.
			// startReqResponse is the only place that creates pendingEOSEs.
			return nil
		}
		// If EOSE already sent (due to limit), ignore
		if pending.eoseSent {
			return nil
		}
		// Guard against duplicate EOSE from the same handler.
		if pending.handlerEOSESent[handlerIndex] {
			return nil
		}
		pending.handlerEOSESent[handlerIndex] = true
		pending.received++
		// Check if all handlers have sent EOSE
		if pending.received >= s.numHandlers {
			delete(s.pendingEOSEs, subID)
			s.completedSubs[subID] = true
			return []*ServerMsg{NewServerEOSEMsg(subID)}
		}
		return nil // Still waiting for more EOSEs

	case MsgTypeCount:
		subID := msg.SubscriptionID
		pending, ok := s.pendingCounts[subID]
		if !ok {
			// Unknown sub_id: no COUNT request was registered, or it was
			// already CLOSE'd. Same reasoning as the EVENT/EOSE branches —
			// startCountResponse is the only place that creates
			// pendingCounts.
			return nil
		}
		// Guard against duplicate responses from the same handler.
		if pending.handlerResponded[handlerIndex] {
			return nil
		}
		pending.handlerResponded[handlerIndex] = true
		pending.received++
		if msg.Count > pending.maxCount {
			pending.maxCount = msg.Count
		}
		// Check if all handlers have responded
		if pending.received >= s.numHandlers {
			maxCount := pending.maxCount
			delete(s.pendingCounts, subID)
			return []*ServerMsg{NewServerCountMsg(subID, maxCount, nil)}
		}
		return nil // Still waiting for more responses

	default:
		// For now, pass through other messages
		return []*ServerMsg{msg}
	}
}
