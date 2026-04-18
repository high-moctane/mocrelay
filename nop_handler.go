package mocrelay

import (
	"context"
)

// NewNopHandler returns a [Handler] that accepts every EVENT and immediately
// returns EOSE for every REQ. It is useful for connection testing and as a
// starting point for custom handlers.
func NewNopHandler() Handler {
	return &nopHandler{}
}

type nopHandler struct{}

func (h *nopHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
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
