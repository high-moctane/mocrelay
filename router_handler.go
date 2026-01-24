//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// RouterHandler is a Handler that routes events between clients.
// It uses a shared Router to manage subscriptions and broadcast events.
type RouterHandler struct {
	router *Router
}

// NewRouterHandler creates a new RouterHandler with the given Router.
func NewRouterHandler(router *Router) *RouterHandler {
	return &RouterHandler{router: router}
}

// ServeNostr implements Handler.
func (h *RouterHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	// Register this connection
	connID := h.router.Register(send)
	defer h.router.Unregister(connID)

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
				if msg.Event != nil {
					// Accept the event
					select {
					case <-ctx.Done():
						return ctx.Err()
					case send <- NewServerOKMsg(msg.Event.ID, true, ""):
					}

					// Broadcast to all matching subscriptions
					h.router.Broadcast(msg.Event)
				}

			case MsgTypeReq:
				// Register subscription
				h.router.Subscribe(connID, msg.SubscriptionID, msg.Filters)

				// Send EOSE (no stored events for now)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case send <- NewServerEOSEMsg(msg.SubscriptionID):
				}

			case MsgTypeClose:
				// Unsubscribe
				h.router.Unsubscribe(connID, msg.SubscriptionID)

			case MsgTypeCount:
				// Return count of 0 (no storage)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case send <- NewServerCountMsg(msg.SubscriptionID, 0, nil):
				}
			}
		}
	}
}
