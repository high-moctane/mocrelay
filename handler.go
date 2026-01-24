//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// Handler is the interface for processing Nostr client connections.
// It receives client messages and sends server messages through channels.
//
// The handler runs for the lifetime of a single WebSocket connection.
// When the handler returns:
//   - error == nil: normal termination (client disconnected gracefully)
//   - error != nil: abnormal termination (connection will be closed, error logged)
//
// The decision to shut down the entire relay is NOT the handler's responsibility.
// That should be handled at the HTTP handler level.
type Handler interface {
	ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error
}

// HandlerFunc is an adapter to allow the use of ordinary functions as Handler.
type HandlerFunc func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error

// ServeNostr calls f(ctx, send, recv).
func (f HandlerFunc) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	return f(ctx, send, recv)
}

// SimpleHandlerBase is the interface for handlers that process messages one at a time.
// This is easier to implement than Handler for most use cases.
//
// The lifecycle is:
//  1. OnStart is called when the connection is established
//  2. HandleMsg is called for each client message
//  3. OnEnd is called when the connection is closing (always called, even on error)
type SimpleHandlerBase interface {
	// OnStart is called when a new connection is established.
	// The returned context is passed to subsequent calls.
	// Return an error to reject the connection.
	OnStart(ctx context.Context) (context.Context, error)

	// OnEnd is called when the connection is closing.
	// This is always called, even if HandleMsg returned an error.
	// Use this for cleanup (e.g., removing subscriptions).
	OnEnd(ctx context.Context) error

	// HandleMsg is called for each client message.
	// Return a channel of server messages to send back.
	// The channel should be closed when all responses are sent.
	// Return an error to terminate the connection.
	HandleMsg(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error)
}

// SimpleHandler wraps a SimpleHandlerBase to implement Handler.
type SimpleHandler struct {
	base SimpleHandlerBase
}

// NewSimpleHandler creates a new SimpleHandler from a SimpleHandlerBase.
func NewSimpleHandler(base SimpleHandlerBase) *SimpleHandler {
	return &SimpleHandler{base: base}
}

// ServeNostr implements Handler.
func (h *SimpleHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	ctx, err := h.base.OnStart(ctx)
	if err != nil {
		return err
	}
	defer h.base.OnEnd(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				// recv channel closed, client disconnected
				return nil
			}

			respCh, err := h.base.HandleMsg(ctx, msg)
			if err != nil {
				return err
			}

			// Forward all responses to send channel
			if respCh != nil {
				for resp := range respCh {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case send <- resp:
					}
				}
			}
		}
	}
}
