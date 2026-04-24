package mocrelay

import (
	"context"
	"errors"
	"iter"
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
	// The returned ServerMsg (if non-nil) is sent to the client (e.g., AUTH challenge).
	// Return an error to reject the connection.
	OnStart(ctx context.Context) (context.Context, *ServerMsg, error)

	// OnEnd is called when the connection is closing.
	// This is always called, even if HandleMsg returned an error.
	// The returned ServerMsg (if non-nil) is sent to the client.
	// Use this for cleanup (e.g., removing subscriptions).
	OnEnd(ctx context.Context) (*ServerMsg, error)

	// HandleMsg is called for each client message.
	// Return an iterator of server messages to send back.
	// Return an error to terminate the connection.
	HandleMsg(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error)
}

// NewSimpleHandler wraps a [SimpleHandlerBase] as a [Handler]. See
// [SimpleHandlerBase] for the lifecycle contract.
func NewSimpleHandler(base SimpleHandlerBase) Handler {
	return &simpleHandler{base: base}
}

type simpleHandler struct {
	base SimpleHandlerBase
}

// simpleHandlerMsgQueueBuffer sizes the internal queue between the recv-drain
// goroutine and the main loop. Any positive cap guarantees recv drain never
// fully stops while the main loop is blocked yielding a response. 10 mirrors
// MergeHandler.childRecvs so both layers share the same backpressure budget.
const simpleHandlerMsgQueueBuffer = 10

func (h *simpleHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) (err error) {
	ctx, startMsg, startErr := h.base.OnStart(ctx)
	if startErr != nil {
		return startErr
	}
	defer func() {
		endMsg, endErr := h.base.OnEnd(ctx)
		err = errors.Join(err, endErr)
		if endMsg != nil {
			select {
			case send <- endMsg:
			default:
				// send channel might be full or closed, best effort
			}
		}
	}()

	// Send OnStart message if provided
	if startMsg != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case send <- startMsg:
		}
	}

	// Decouple recv drain from response forwarding. A blocked `send <- msg`
	// in the main loop below must not stop recv drain — otherwise a slow
	// downstream wedges the whole handler and upstream broadcasters (e.g.
	// MergeHandler) pile up on their own child-recv buffers. This is the
	// "deadlock spring" shape observed at production scale
	// (diary 2026-04-24).
	//
	// The drain goroutine exits on either:
	//   - ctx cancel (caller-driven shutdown — every ServeNostr caller in
	//     this module cancels ctx immediately after ServeNostr returns, so
	//     a main-loop return reliably reaps this goroutine).
	//   - recv close (client disconnect); closing msgQueue then drives the
	//     main loop to return nil naturally.
	msgQueue := make(chan *ClientMsg, simpleHandlerMsgQueueBuffer)
	go func() {
		defer close(msgQueue)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-recv:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
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
				// recv channel closed, client disconnected
				return nil
			}

			resp, err := h.base.HandleMsg(ctx, msg)
			if err != nil {
				return err
			}

			// Forward all responses to send channel
			if resp != nil {
				for msg := range resp {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case send <- msg:
					}
				}
			}
		}
	}
}
