package mocrelay

import (
	"context"
)

// NewRouterHandler returns a [Handler] backed by router that broadcasts
// events between clients. The connection is registered with router on
// start and unregistered on return, so subscription lifecycle is managed
// automatically.
func NewRouterHandler(router *Router) Handler {
	return &routerHandler{router: router}
}

type routerHandler struct {
	router *Router
}

func (h *routerHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
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
