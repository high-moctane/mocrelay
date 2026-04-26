package mocrelay

import (
	"context"
	"errors"
	"time"
)

// DefaultStorageHandlerSlowQueryThreshold is the default threshold for
// logging a slow REQ / COUNT subscription when
// [StorageHandlerOptions.SlowQueryThreshold] is zero.
const DefaultStorageHandlerSlowQueryThreshold = 1 * time.Second

// queryTimeoutCLOSEDReason is the CLOSED message body sent when a REQ or
// COUNT is aborted by [StorageHandlerOptions.QueryTimeout].
const queryTimeoutCLOSEDReason = "error: query timeout"

// StorageHandlerOptions configures the storage handler returned by
// [NewStorageHandler].
//
// All fields are optional. A nil *StorageHandlerOptions is equivalent to a
// zero-valued StorageHandlerOptions and means "use defaults for everything".
type StorageHandlerOptions struct {
	// SlowQueryThreshold sets the total-duration threshold above which a REQ
	// or COUNT subscription is logged at Warn via [LoggerFromContext]. The
	// emitted log carries a breakdown (scan vs send-wait time for REQ) so
	// operators can distinguish a genuinely heavy query from a client that
	// is failing to drain its send buffer — the two are entangled because
	// iter.Seq is pull-driven and pauses the storage iterator while yield
	// is blocked, so having both numbers side-by-side is the most reliable
	// signal of which side owns the stall.
	//
	// A zero value selects [DefaultStorageHandlerSlowQueryThreshold] (1s).
	// A negative value disables slow-query logging entirely.
	SlowQueryThreshold time.Duration

	// QueryTimeout aborts a REQ or COUNT whose total duration exceeds it.
	// When the timeout fires, the storage handler stops iterating, sends a
	// CLOSED message with reason "error: query timeout", and returns —
	// neither the trailing EOSE (for REQ) nor the COUNT reply is emitted.
	// The slow-query log (if enabled) still fires with completed=false, so
	// the abort is recorded alongside other slow subscriptions.
	//
	// The context passed to Storage.Query is wrapped with
	// [context.WithTimeout], so any Storage implementation that respects
	// ctx can short-circuit its own scan; implementations that don't (e.g.
	// Pebble) are stopped at the next event boundary where the iteration
	// loop polls ctx.Err.
	//
	// The primary use case is a live but slow-consuming client — one that
	// keeps the WebSocket alive (so ping_timeout never fires) yet drains
	// EVENTs so slowly that a broad REQ can stall the handler's iterator
	// for hours. Setting QueryTimeout gives the relay a hard ceiling.
	//
	// A zero value disables the timeout (default, preserves prior
	// behavior). A negative value also disables it. When non-zero, it
	// should typically be set well above SlowQueryThreshold so the
	// threshold logs fire first and give operators a chance to observe
	// slow REQs before they are aborted.
	QueryTimeout time.Duration
}

// NewStorageHandler returns a [Handler] that serves EVENT and REQ messages
// against storage. Pass nil for opts to use defaults.
//
// Behavior:
//   - EVENT: Store the event, return OK
//   - REQ: Query the storage, return EVENT messages + EOSE
//   - CLOSE: No-op — the handler does not manage subscriptions; pair with
//     [NewRouterHandler] via [NewMergeHandler] to deliver real-time events
//   - COUNT: Query the storage, return COUNT with the count
//
// Once EOSE is sent for a REQ, the handler's job is done for that
// subscription.
func NewStorageHandler(storage Storage, opts *StorageHandlerOptions) Handler {
	threshold := DefaultStorageHandlerSlowQueryThreshold
	var queryTimeout time.Duration // zero == disabled
	if opts != nil {
		switch {
		case opts.SlowQueryThreshold < 0:
			threshold = 0 // disabled
		case opts.SlowQueryThreshold > 0:
			threshold = opts.SlowQueryThreshold
		}
		if opts.QueryTimeout > 0 {
			queryTimeout = opts.QueryTimeout
		}
	}
	return &storageHandler{
		storage:            storage,
		slowQueryThreshold: threshold,
		queryTimeout:       queryTimeout,
	}
}

// storageHandler implements [Handler] directly rather than routing through
// [SimpleHandlerBase]. The reason is QueryTimeout: a per-REQ abort deadline
// must be visible at the send<-msg boundary as well as during scan. The
// SimpleHandlerBase path routes yields through a per-connection ctx that
// is not scoped to the in-flight REQ, so a blocked send (stalled receiver)
// survives past the timeout indefinitely — the "live but slow client"
// stall observed on salmon. Owning ServeNostr directly lets us attach the
// timeout ctx to the exact send sites that can stall.
type storageHandler struct {
	storage Storage
	// slowQueryThreshold is the effective threshold for slow-query logging.
	// Zero means disabled.
	slowQueryThreshold time.Duration
	// queryTimeout is the effective abort timeout for REQ / COUNT.
	// Zero means disabled.
	queryTimeout time.Duration
}

// storageHandlerMsgQueueBuffer sizes the internal queue between the
// recv-drain goroutine and the main loop. Any positive cap guarantees recv
// drain never fully stops while the main loop is parked on a yield's
// send<-msg. 10 mirrors [simpleHandlerMsgQueueBuffer] and
// MergeHandler.childRecvs so all three layers share the same backpressure
// budget.
const storageHandlerMsgQueueBuffer = 10

func (h *storageHandler) ServeNostr(
	ctx context.Context,
	send chan<- *ServerMsg,
	recv <-chan *ClientMsg,
) error {
	// Decouple recv drain from response forwarding. handleReq's iter.Seq
	// loop forwards events to send<-msg on this same goroutine, so a
	// stalled receiver would otherwise stop recv drain entirely. Under an
	// upstream MergeHandler (childRecvs cap 10) that wedges into the
	// "deadlock spring" shape recorded in CLAUDE.md and the diary entry
	// for 2026-04-24: childSends fills, yield blocks, recv stops draining,
	// childRecvs fills, broadcastAll trips BroadcastTimeout, child gets
	// retired. simpleHandler solved the same problem in #78; the CLAUDE.md
	// rule "Handlers that implement Handler directly and consume iter.Seq
	// responses must follow the same shape" applies here verbatim.
	//
	// drainCtx is a sub-ctx of the connection ctx. `defer drainCancel()`
	// runs on every main-loop return path (ctx cancel, msgQueue close /
	// recv close, handle* error, normal completion), so the drain
	// goroutine is reliably reaped without depending on caller-side ctx
	// cancel — important for third-party callers invoking ServeNostr
	// directly. Recv close additionally closes msgQueue, which drives the
	// main loop to return nil naturally.
	drainCtx, drainCancel := context.WithCancel(ctx)
	defer drainCancel()
	msgQueue := make(chan *ClientMsg, storageHandlerMsgQueueBuffer)
	go func() {
		defer close(msgQueue)
		for {
			select {
			case <-drainCtx.Done():
				return
			case msg, ok := <-recv:
				if !ok {
					return
				}
				select {
				case <-drainCtx.Done():
					return
				case msgQueue <- msg:
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgQueue:
			if !ok {
				// recv was closed (drain goroutine returned and
				// closed msgQueue); client disconnected.
				return nil
			}
			switch msg.Type {
			case MsgTypeEvent:
				if err := h.handleEvent(ctx, send, msg); err != nil {
					return err
				}
			case MsgTypeReq:
				if err := h.handleReq(ctx, send, msg); err != nil {
					return err
				}
			case MsgTypeCount:
				if err := h.handleCount(ctx, send, msg); err != nil {
					return err
				}
			case MsgTypeClose:
				// StorageHandler doesn't manage subscriptions, so
				// CLOSE is a no-op. Pair with RouterHandler via
				// MergeHandler to route CLOSE to the router.
			}
		}
	}
}

// sendServerMsg forwards m on send, returning ctx.Err() if ctx is
// cancelled before the send completes.
func sendServerMsg(ctx context.Context, send chan<- *ServerMsg, m *ServerMsg) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case send <- m:
		return nil
	}
}

func (h *storageHandler) handleEvent(
	ctx context.Context,
	send chan<- *ServerMsg,
	msg *ClientMsg,
) error {
	if msg.Event == nil {
		return sendServerMsg(ctx, send, NewServerOKMsg("", false, "error: no event provided"))
	}
	stored, err := h.storage.Store(ctx, msg.Event)
	if err != nil {
		LoggerFromContext(ctx).ErrorContext(ctx, "store error", "error", err, "event_id", msg.Event.ID)
		return sendServerMsg(ctx, send, NewServerOKMsg(msg.Event.ID, false, "error: internal error"))
	}
	if stored {
		return sendServerMsg(ctx, send, NewServerOKMsg(msg.Event.ID, true, ""))
	}
	// Not stored: could be duplicate, older replaceable, deleted, or ephemeral.
	return sendServerMsg(ctx, send, NewServerOKMsg(msg.Event.ID, true, "duplicate: already have this event"))
}

func (h *storageHandler) handleReq(
	ctx context.Context,
	send chan<- *ServerMsg,
	msg *ClientMsg,
) error {
	if msg.SubscriptionID == "" {
		return nil
	}

	// start is taken before Query so the iterator-initialization cost
	// (index seek, snapshot creation, etc.) counts toward total.
	start := time.Now()

	// queryCtx derives from the connection ctx and expires at QueryTimeout.
	// It is the abort signal for BOTH scan and send<-msg below. Keeping
	// the timeout scoped to this function means a stalled receiver can't
	// outlive the deadline — the "live but slow client" regression.
	queryCtx := ctx
	if h.queryTimeout > 0 {
		var cancel context.CancelFunc
		queryCtx, cancel = context.WithTimeout(ctx, h.queryTimeout)
		defer cancel()
	}

	events, errFn, closeFn := h.storage.Query(queryCtx, msg.Filters)
	defer closeFn()

	// sendWait accumulates time spent blocked on send<-msg (send wait) as
	// opposed to scan; they partition total (iter.Seq is pull-driven so a
	// blocked send also pauses the iterator). Both are still reported
	// separately because which side owns a stall determines the fix.
	var sendWait time.Duration
	eventsSent := 0

	// trySendEvent forwards m on send, watching queryCtx (QueryTimeout)
	// alongside the normal delivery path. Return values:
	//   (true,  nil) → queryCtx fired; caller should break and emit CLOSED
	//   (false, err) → outer ctx cancelled or other fatal; caller returns
	//   (false, nil) → event delivered; caller continues
	trySendEvent := func(m *ServerMsg) (aborted bool, err error) {
		s := time.Now()
		defer func() { sendWait += time.Since(s) }()
		select {
		case send <- m:
			return false, nil
		case <-queryCtx.Done():
			// queryCtx fires for timeout *or* outer-ctx cancel (it is
			// derived from ctx). Distinguish via ctx.Err.
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return true, nil
		}
	}

	for event := range events {
		// Fast path: timeout fired while scanning / waiting to send the
		// previous event. Abort before emitting another one.
		if queryCtx.Err() != nil {
			break
		}
		aborted, err := trySendEvent(NewServerEventMsg(msg.SubscriptionID, event))
		if err != nil {
			h.maybeLogSlowReq(ctx, msg, start, sendWait, eventsSent, false)
			return err
		}
		if aborted {
			break
		}
		eventsSent++
	}

	// Capture the query result once. Storage.Query's errFn contract does
	// not promise idempotence across calls, so we must not re-invoke it
	// after using its return value in the abort-decision predicate below.
	queryErr := errFn()

	// Terminal-message decision. A queryCtx failure while outer ctx is
	// still alive means QueryTimeout fired → send CLOSED. Storage impls
	// that respect ctx may also surface context.DeadlineExceeded via
	// errFn; treat that the same as a queryCtx-driven abort.
	if (queryCtx.Err() != nil && ctx.Err() == nil) ||
		errors.Is(queryErr, context.DeadlineExceeded) {
		// Emit CLOSED on the outer ctx. If outer ctx is cancelled the
		// send will just fail and we'll return its err — the connection
		// is going away anyway.
		s := time.Now()
		err := sendServerMsg(ctx, send, NewServerClosedMsg(msg.SubscriptionID, queryTimeoutCLOSEDReason))
		sendWait += time.Since(s)
		h.maybeLogSlowReq(ctx, msg, start, sendWait, eventsSent, false)
		return err
	}

	if queryErr != nil {
		LoggerFromContext(ctx).WarnContext(ctx, "query error", "error", queryErr, "subscription_id", msg.SubscriptionID)
		// Even on query error we still try to send EOSE so the client
		// isn't left hanging — matches prior behavior.
	}

	// Send EOSE to signal end of stored events. The slow-query
	// `completed` flag below is `err == nil`, i.e. true only when the
	// EOSE actually made it onto send. Treat it as "the relay finished
	// the subscription cleanly" rather than "the client received it" —
	// the latter can't be observed from this side. An EOSE that fails
	// because the outer ctx was cancelled mid-send is logged with
	// completed=false, matching the abort path's semantics.
	s := time.Now()
	err := sendServerMsg(ctx, send, NewServerEOSEMsg(msg.SubscriptionID))
	sendWait += time.Since(s)
	h.maybeLogSlowReq(ctx, msg, start, sendWait, eventsSent, err == nil)
	return err
}

func (h *storageHandler) handleCount(
	ctx context.Context,
	send chan<- *ServerMsg,
	msg *ClientMsg,
) error {
	if msg.SubscriptionID == "" {
		return nil
	}

	// COUNT emits a single terminal message, so there is no meaningful
	// send-wait component during iteration — total duration is dominated
	// by scan. start is taken before Query so iterator-initialization
	// cost counts toward total.
	start := time.Now()

	queryCtx := ctx
	if h.queryTimeout > 0 {
		var cancel context.CancelFunc
		queryCtx, cancel = context.WithTimeout(ctx, h.queryTimeout)
		defer cancel()
	}

	events, errFn, closeFn := h.storage.Query(queryCtx, msg.Filters)
	defer closeFn()

	var count uint64
	for range events {
		if queryCtx.Err() != nil {
			break
		}
		count++
	}

	// Capture the query result once; see handleReq for why errFn must not
	// be re-invoked.
	queryErr := errFn()

	if (queryCtx.Err() != nil && ctx.Err() == nil) ||
		errors.Is(queryErr, context.DeadlineExceeded) {
		err := sendServerMsg(ctx, send, NewServerClosedMsg(msg.SubscriptionID, queryTimeoutCLOSEDReason))
		h.maybeLogSlowCount(ctx, msg, start, count)
		return err
	}

	if queryErr != nil {
		LoggerFromContext(ctx).WarnContext(ctx, "count query error", "error", queryErr, "subscription_id", msg.SubscriptionID)
		sendErr := sendServerMsg(ctx, send, NewServerCountMsg(msg.SubscriptionID, 0, nil))
		h.maybeLogSlowCount(ctx, msg, start, count)
		return sendErr
	}

	err := sendServerMsg(ctx, send, NewServerCountMsg(msg.SubscriptionID, count, nil))
	h.maybeLogSlowCount(ctx, msg, start, count)
	return err
}

// maybeLogSlowReq emits a Warn log when the REQ total duration exceeds the
// configured slowQueryThreshold. A zero threshold disables logging.
func (h *storageHandler) maybeLogSlowReq(
	ctx context.Context,
	msg *ClientMsg,
	start time.Time,
	sendWait time.Duration,
	eventsSent int,
	completed bool,
) {
	if h.slowQueryThreshold <= 0 {
		return
	}
	total := time.Since(start)
	if total < h.slowQueryThreshold {
		return
	}
	scan := max(total-sendWait, 0)
	LoggerFromContext(ctx).WarnContext(ctx, "storage: slow REQ",
		"subscription_id", msg.SubscriptionID,
		"filters", reqFiltersLogValue(msg.Filters),
		"events_sent", eventsSent,
		"total_ms", total.Milliseconds(),
		"scan_ms", scan.Milliseconds(),
		"send_wait_ms", sendWait.Milliseconds(),
		"completed", completed,
	)
}

// maybeLogSlowCount emits a Warn log when the COUNT total duration exceeds
// the configured slowQueryThreshold. A zero threshold disables logging.
func (h *storageHandler) maybeLogSlowCount(
	ctx context.Context,
	msg *ClientMsg,
	start time.Time,
	count uint64,
) {
	if h.slowQueryThreshold <= 0 {
		return
	}
	total := time.Since(start)
	if total < h.slowQueryThreshold {
		return
	}
	LoggerFromContext(ctx).WarnContext(ctx, "storage: slow COUNT",
		"subscription_id", msg.SubscriptionID,
		"filters", reqFiltersLogValue(msg.Filters),
		"count", count,
		"total_ms", total.Milliseconds(),
	)
}
