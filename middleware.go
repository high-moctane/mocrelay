package mocrelay

import (
	"context"
	"errors"
)

// Middleware wraps a Handler to add functionality.
type Middleware func(Handler) Handler

// SimpleMiddlewareBase is the interface for middlewares that process messages one at a time.
// This is easier to implement than writing a full Middleware for most use cases.
//
// The lifecycle is:
//  1. OnStart is called when the connection is established
//  2. HandleClientMsg is called for each client message (recv direction)
//  3. HandleServerMsg is called for each server message (send direction)
//  4. OnEnd is called when the connection is closing (always called, even on error)
//
// Message handling rules:
//   - HandleClientMsg returns (out, resp, err):
//   - out != nil: pass out to downstream handler
//   - resp != nil: send resp to client (without passing to downstream)
//   - out == nil && resp == nil: drop the message
//   - HandleServerMsg returns (out, err):
//   - out != nil: send out to client
//   - out == nil: drop the message
type SimpleMiddlewareBase interface {
	// OnStart is called when a new connection is established.
	// The returned context is passed to subsequent calls.
	// The returned ServerMsg (if non-nil) is sent to the client (e.g., AUTH challenge).
	// Return an error to reject the connection.
	OnStart(ctx context.Context) (context.Context, *ServerMsg, error)

	// OnEnd is called when the connection is closing.
	// This is always called, even if Handle* returned an error.
	// The returned ServerMsg (if non-nil) is sent to the client.
	// Use this for cleanup.
	OnEnd(ctx context.Context) (*ServerMsg, error)

	// HandleClientMsg processes a client message.
	// Returns:
	//   - (out, nil, nil): pass out to downstream handler
	//   - (nil, resp, nil): send resp to client, don't pass downstream
	//   - (nil, nil, nil): drop the message
	//   - (_, _, err): error, connection will be terminated
	HandleClientMsg(ctx context.Context, msg *ClientMsg) (out *ClientMsg, resp *ServerMsg, err error)

	// HandleServerMsg processes a server message from downstream.
	// Returns:
	//   - (out, nil): send out to client
	//   - (nil, nil): drop the message
	//   - (_, err): error, connection will be terminated
	HandleServerMsg(ctx context.Context, msg *ServerMsg) (out *ServerMsg, err error)
}

// NewSimpleMiddleware creates a Middleware from one or more SimpleMiddlewareBase.
//
// When multiple bases are provided, they are composed into a single pipeline
// that uses only one goroutine, regardless of the number of bases.
// Bases are listed in outermost-first order (like alice.New):
//
//	NewSimpleMiddleware(logging, auth)(handler)
//	// message flow: client → logging → auth → handler
//
// Internally uses a unified select loop with nil channel pattern to handle
// both recv and send directions in a single goroutine, avoiding deadlocks
// without requiring buffered channels.
func NewSimpleMiddleware(bases ...SimpleMiddlewareBase) Middleware {
	if len(bases) == 0 {
		return func(next Handler) Handler { return next }
	}

	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) (err error) {
			// OnStart all bases (outermost first)
			for i, base := range bases {
				var startMsg *ServerMsg
				var startErr error
				ctx, startMsg, startErr = base.OnStart(ctx)
				if startErr != nil {
					// OnEnd already-started bases (reverse order)
					for j := i - 1; j >= 0; j-- {
						_, endErr := bases[j].OnEnd(ctx)
						err = errors.Join(err, endErr)
					}
					return errors.Join(err, startErr)
				}
				if startMsg != nil {
					select {
					case <-ctx.Done():
						// OnEnd already-started bases including current (reverse order)
						for j := i; j >= 0; j-- {
							_, endErr := bases[j].OnEnd(ctx)
							err = errors.Join(err, endErr)
						}
						return errors.Join(err, ctx.Err())
					case send <- startMsg:
					}
				}
			}

			// OnEnd all bases (reverse order, always called)
			defer func() {
				for i := len(bases) - 1; i >= 0; i-- {
					endMsg, endErr := bases[i].OnEnd(ctx)
					err = errors.Join(err, endErr)
					if endMsg != nil {
						select {
						case send <- endMsg:
						default:
							// send channel might be full or closed, best effort
						}
					}
				}
			}()

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Channels between this pipeline and downstream handler
			downstreamRecv := make(chan *ClientMsg)
			downstreamSend := make(chan *ServerMsg)

			errs := make(chan error, 1)

			// Single goroutine: pipeline loop for all bases
			go func() {
				defer close(downstreamRecv)
				errs <- simpleMiddlewarePipelineLoop(ctx, bases, recv, send, downstreamRecv, downstreamSend)
			}()

			// Run downstream handler
			handlerErr := next.ServeNostr(ctx, downstreamSend, downstreamRecv)

			// Cancel and wait for goroutine
			cancel()
			loopErr := <-errs

			err = errors.Join(handlerErr, loopErr)
			return
		})
	}
}

// simpleMiddlewarePipelineLoop handles both recv and send directions for
// multiple middleware bases in a single goroutine.
//
// For client messages (recv direction), HandleClientMsg is applied through
// all bases in forward order (outermost to innermost). If a base generates
// a response, that response is passed through HandleServerMsg of all outer
// bases before being sent to the client.
//
// For server messages (send direction), HandleServerMsg is applied through
// all bases in reverse order (innermost to outermost).
//
// Uses the nil channel pattern to avoid deadlock: when there are pending messages
// to forward to downstream, the downstreamRecv channel is active in the select;
// when there are none, it is set to nil so the case is never selected.
func simpleMiddlewarePipelineLoop(
	ctx context.Context,
	bases []SimpleMiddlewareBase,
	recv <-chan *ClientMsg,
	send chan<- *ServerMsg,
	downstreamRecv chan<- *ClientMsg,
	downstreamSend <-chan *ServerMsg,
) error {
	// Pending queues for nil channel pattern
	var pendingToDownstream []*ClientMsg
	var pendingToClient []*ServerMsg

	for {
		// When recv is closed (nil) and all pending messages are drained, we're done.
		// This ensures all queued responses reach their destination before returning.
		if recv == nil && len(pendingToDownstream) == 0 && len(pendingToClient) == 0 {
			return nil
		}

		// nil channel pattern: only activate send cases when there are pending messages.
		// A nil channel in select is never selected, effectively disabling that case.
		var downstreamRecvCh chan<- *ClientMsg
		var downstreamRecvMsg *ClientMsg
		if len(pendingToDownstream) > 0 {
			downstreamRecvCh = downstreamRecv
			downstreamRecvMsg = pendingToDownstream[0]
		}

		var sendCh chan<- *ServerMsg
		var sendMsg *ServerMsg
		if len(pendingToClient) > 0 {
			sendCh = send
			sendMsg = pendingToClient[0]
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				// recv closed: nil it out to stop reading, then drain pending queues.
				// This is the nil channel pattern applied to recv itself.
				recv = nil
				continue
			}

			// Apply HandleClientMsg through all bases (outermost to innermost)
			out := msg
			for i, base := range bases {
				var resp *ServerMsg
				var handleErr error
				out, resp, handleErr = base.HandleClientMsg(ctx, out)
				if handleErr != nil {
					return handleErr
				}

				if resp != nil {
					// Response goes through HandleServerMsg of outer bases
					for j := i - 1; j >= 0; j-- {
						var serverErr error
						resp, serverErr = bases[j].HandleServerMsg(ctx, resp)
						if serverErr != nil {
							return serverErr
						}
						if resp == nil {
							break
						}
					}
					if resp != nil {
						pendingToClient = append(pendingToClient, resp)
					}
				}

				if out == nil {
					break
				}
			}
			if out != nil {
				pendingToDownstream = append(pendingToDownstream, out)
			}

		case msg, ok := <-downstreamSend:
			if !ok {
				return nil // downstream handler finished
			}

			// Apply HandleServerMsg through all bases (innermost to outermost)
			out := msg
			for i := len(bases) - 1; i >= 0; i-- {
				var handleErr error
				out, handleErr = bases[i].HandleServerMsg(ctx, out)
				if handleErr != nil {
					return handleErr
				}
				if out == nil {
					break
				}
			}
			if out != nil {
				pendingToClient = append(pendingToClient, out)
			}

		case downstreamRecvCh <- downstreamRecvMsg:
			pendingToDownstream = pendingToDownstream[1:]

		case sendCh <- sendMsg:
			pendingToClient = pendingToClient[1:]
		}
	}
}
