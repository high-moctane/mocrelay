//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// NopHandler is a handler that does nothing but respond correctly.
// It accepts all events and returns EOSE for all subscriptions.
// Useful for connection testing and as a base for custom handlers.
type NopHandler struct{}

// NewNopHandler creates a new NopHandler.
func NewNopHandler() *NopHandler {
	return &NopHandler{}
}

// ServeNostr implements Handler.
func (h *NopHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				return nil
			}

			switch msg.Type {
			case MsgTypeEvent:
				// Accept all events
				if msg.Event != nil {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case send <- NewServerOKMsg(msg.Event.ID, true, ""):
					}
				}

			case MsgTypeReq:
				// Immediately send EOSE (no stored events)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case send <- NewServerEOSEMsg(msg.SubscriptionID):
				}

			case MsgTypeClose:
				// Nothing to do

			case MsgTypeCount:
				// Return count of 0
				select {
				case <-ctx.Done():
					return ctx.Err()
				case send <- NewServerCountMsg(msg.SubscriptionID, 0, nil):
				}
			}
		}
	}
}
