package mocrelay

import (
	"context"
	"iter"
)

// NewStorageHandler returns a [Handler] that serves EVENT and REQ messages
// against storage.
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
func NewStorageHandler(storage Storage) Handler {
	return NewSimpleHandler(&storageHandler{storage: storage})
}

type storageHandler struct {
	storage Storage
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

		events, errFn, closeFn := h.storage.Query(ctx, msg.Filters)
		defer closeFn()

		// Send all events using for-range over iterator
		for event := range events {
			if !yield(NewServerEventMsg(msg.SubscriptionID, event)) {
				return
			}
		}

		// Check for errors after iteration
		if err := errFn(); err != nil {
			LoggerFromContext(ctx).WarnContext(ctx, "query error", "error", err, "subscription_id", msg.SubscriptionID)
			yield(NewServerEOSEMsg(msg.SubscriptionID))
			return
		}

		// Send EOSE to signal end of stored events
		yield(NewServerEOSEMsg(msg.SubscriptionID))
	}, nil
}

func (h *storageHandler) handleCount(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
	return func(yield func(*ServerMsg) bool) {
		if msg.SubscriptionID == "" {
			return
		}

		events, errFn, closeFn := h.storage.Query(ctx, msg.Filters)
		defer closeFn()

		// Count events by iterating
		count := uint64(0)
		for range events {
			count++
		}

		if err := errFn(); err != nil {
			LoggerFromContext(ctx).WarnContext(ctx, "count query error", "error", err, "subscription_id", msg.SubscriptionID)
			yield(NewServerCountMsg(msg.SubscriptionID, 0, nil))
			return
		}

		yield(NewServerCountMsg(msg.SubscriptionID, count, nil))
	}, nil
}
