package mocrelay

import (
	"context"
	"iter"
	"time"
)

// DefaultStorageHandlerSlowQueryThreshold is the default threshold for
// logging a slow REQ / COUNT subscription when
// [StorageHandlerOptions.SlowQueryThreshold] is zero.
const DefaultStorageHandlerSlowQueryThreshold = 1 * time.Second

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
	if opts != nil {
		switch {
		case opts.SlowQueryThreshold < 0:
			threshold = 0 // disabled
		case opts.SlowQueryThreshold > 0:
			threshold = opts.SlowQueryThreshold
		}
	}
	return NewSimpleHandler(&storageHandler{
		storage:            storage,
		slowQueryThreshold: threshold,
	})
}

type storageHandler struct {
	storage Storage
	// slowQueryThreshold is the effective threshold for slow-query logging.
	// Zero means disabled.
	slowQueryThreshold time.Duration
}

func (h *storageHandler) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (h *storageHandler) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (h *storageHandler) HandleMsg(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
	switch msg.Type {
	case MsgTypeEvent:
		return h.handleEvent(ctx, msg)
	case MsgTypeReq:
		return h.handleReq(ctx, msg)
	case MsgTypeClose:
		// StorageHandler doesn't manage subscriptions, so CLOSE is a no-op
		return nil, nil
	case MsgTypeCount:
		return h.handleCount(ctx, msg)
	default:
		return nil, nil
	}
}

func (h *storageHandler) handleEvent(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
	return func(yield func(*ServerMsg) bool) {
		if msg.Event == nil {
			yield(NewServerOKMsg("", false, "error: no event provided"))
			return
		}

		stored, err := h.storage.Store(ctx, msg.Event)
		if err != nil {
			LoggerFromContext(ctx).ErrorContext(ctx, "store error", "error", err, "event_id", msg.Event.ID)
			yield(NewServerOKMsg(msg.Event.ID, false, "error: internal error"))
			return
		}

		if stored {
			yield(NewServerOKMsg(msg.Event.ID, true, ""))
		} else {
			// Not stored: could be duplicate, older replaceable, deleted, or ephemeral
			yield(NewServerOKMsg(msg.Event.ID, true, "duplicate: already have this event"))
		}
	}, nil
}

func (h *storageHandler) handleReq(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
	return func(yield func(*ServerMsg) bool) {
		if msg.SubscriptionID == "" {
			return
		}

		// start is taken before Query so the iterator-initialization cost
		// (index seek, snapshot creation, etc.) counts toward total.
		start := time.Now()

		events, errFn, closeFn := h.storage.Query(ctx, msg.Filters)
		defer closeFn()

		// Measure time spent inside yield (send wait) vs outside yield
		// (scan). iter.Seq is pull-driven so yield blocking also pauses
		// the storage iterator — the two are entangled. But the split
		// is still operationally meaningful: a large sendWait relative
		// to scan points at a stalled client; the reverse points at
		// filter / index / data-volume costs.
		var sendWait time.Duration
		eventsSent := 0

		// yieldTracked wraps yield so every emission contributes to
		// sendWait, including the trailing EOSE — which matters when
		// the stall is on the client side (EOSE never arrives either).
		yieldTracked := func(m *ServerMsg) bool {
			s := time.Now()
			ok := yield(m)
			sendWait += time.Since(s)
			return ok
		}

		for event := range events {
			if !yieldTracked(NewServerEventMsg(msg.SubscriptionID, event)) {
				h.maybeLogSlowReq(ctx, msg, start, sendWait, eventsSent, false)
				return
			}
			eventsSent++
		}

		// Check for errors after iteration
		if err := errFn(); err != nil {
			LoggerFromContext(ctx).WarnContext(ctx, "query error", "error", err, "subscription_id", msg.SubscriptionID)
			yieldTracked(NewServerEOSEMsg(msg.SubscriptionID))
			h.maybeLogSlowReq(ctx, msg, start, sendWait, eventsSent, true)
			return
		}

		// Send EOSE to signal end of stored events
		yieldTracked(NewServerEOSEMsg(msg.SubscriptionID))
		h.maybeLogSlowReq(ctx, msg, start, sendWait, eventsSent, true)
	}, nil
}

func (h *storageHandler) handleCount(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
	return func(yield func(*ServerMsg) bool) {
		if msg.SubscriptionID == "" {
			return
		}

		// COUNT emits a single terminal message, so there is no
		// meaningful send-wait component during iteration — total
		// duration is dominated by scan. start is taken before Query so
		// iterator-initialization cost counts toward total.
		start := time.Now()

		events, errFn, closeFn := h.storage.Query(ctx, msg.Filters)
		defer closeFn()

		count := uint64(0)
		for range events {
			count++
		}

		if err := errFn(); err != nil {
			LoggerFromContext(ctx).WarnContext(ctx, "count query error", "error", err, "subscription_id", msg.SubscriptionID)
			yield(NewServerCountMsg(msg.SubscriptionID, 0, nil))
			h.maybeLogSlowCount(ctx, msg, start, count)
			return
		}

		yield(NewServerCountMsg(msg.SubscriptionID, count, nil))
		h.maybeLogSlowCount(ctx, msg, start, count)
	}, nil
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
		"filters", msg.Filters,
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
		"filters", msg.Filters,
		"count", count,
		"total_ms", total.Milliseconds(),
	)
}
