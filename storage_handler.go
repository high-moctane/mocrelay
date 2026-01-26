//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// StorageHandler wraps a Storage as a Handler.
// It handles EVENT and REQ messages using the underlying storage.
//
// Behavior:
//   - EVENT: Store the event, return OK
//   - REQ: Query the storage, return EVENT messages + EOSE
//   - CLOSE: No-op (StorageHandler doesn't manage subscriptions)
//   - COUNT: Query the storage, return COUNT with the count
//
// Note: StorageHandler does NOT manage subscriptions. Once EOSE is sent,
// its job is done for that REQ. Use RouterHandler for live subscriptions.
type StorageHandler struct {
	storage Storage
}

// NewStorageHandler creates a new StorageHandler.
func NewStorageHandler(storage Storage) Handler {
	return NewSimpleHandler(&StorageHandler{storage: storage})
}

func (h *StorageHandler) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (h *StorageHandler) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (h *StorageHandler) HandleMsg(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
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

func (h *StorageHandler) handleEvent(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
	ch := make(chan *ServerMsg, 1)

	go func() {
		defer close(ch)

		if msg.Event == nil {
			ch <- NewServerOKMsg("", false, "error: no event provided")
			return
		}

		stored, err := h.storage.Store(ctx, msg.Event)
		if err != nil {
			ch <- NewServerOKMsg(msg.Event.ID, false, "error: "+err.Error())
			return
		}

		if stored {
			ch <- NewServerOKMsg(msg.Event.ID, true, "")
		} else {
			// Not stored: could be duplicate, older replaceable, deleted, or ephemeral
			ch <- NewServerOKMsg(msg.Event.ID, true, "duplicate: already have this event")
		}
	}()

	return ch, nil
}

func (h *StorageHandler) handleReq(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
	// Buffer size: we'll send events + EOSE
	// Use unbuffered to avoid memory issues with large queries
	ch := make(chan *ServerMsg)

	go func() {
		defer close(ch)

		if msg.SubscriptionID == "" {
			return
		}

		events, errFn, closeFn := h.storage.Query(ctx, msg.Filters)
		defer closeFn()

		// Send all events using for-range over iterator
		for event := range events {
			select {
			case <-ctx.Done():
				return
			case ch <- NewServerEventMsg(msg.SubscriptionID, event):
			}
		}

		// Check for errors after iteration
		if err := errFn(); err != nil {
			// On error, just send EOSE and continue
			ch <- NewServerEOSEMsg(msg.SubscriptionID)
			return
		}

		// Send EOSE to signal end of stored events
		select {
		case <-ctx.Done():
			return
		case ch <- NewServerEOSEMsg(msg.SubscriptionID):
		}
	}()

	return ch, nil
}

func (h *StorageHandler) handleCount(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
	ch := make(chan *ServerMsg, 1)

	go func() {
		defer close(ch)

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
			ch <- NewServerCountMsg(msg.SubscriptionID, 0, nil)
			return
		}

		ch <- NewServerCountMsg(msg.SubscriptionID, count, nil)
	}()

	return ch, nil
}
