//go:build goexperiment.jsonv2

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

// NewSimpleMiddleware creates a Middleware from a SimpleMiddlewareBase.
//
// Internally uses a unified select loop with nil channel pattern to handle
// both recv and send directions in a single goroutine, avoiding deadlocks
// without requiring buffered channels.
func NewSimpleMiddleware(base SimpleMiddlewareBase) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) (err error) {
			ctx, startMsg, startErr := base.OnStart(ctx)
			if startErr != nil {
				return startErr
			}
			defer func() {
				endMsg, endErr := base.OnEnd(ctx)
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

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Channels between this middleware and downstream handler
			downstreamRecv := make(chan *ClientMsg)
			downstreamSend := make(chan *ServerMsg)

			errs := make(chan error, 1)

			// Single goroutine: unified select loop for both directions
			go func() {
				defer close(downstreamRecv)
				errs <- simpleMiddlewareLoop(ctx, base, recv, send, downstreamRecv, downstreamSend)
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

// simpleMiddlewareLoop handles both recv and send directions in a single goroutine.
//
// Uses the nil channel pattern to avoid deadlock: when there are pending messages
// to forward to downstream, the downstreamRecv channel is active in the select;
// when there are none, it is set to nil so the case is never selected.
//
// Similarly, pending responses to the client use a queue with nil channel pattern
// on the send channel.
func simpleMiddlewareLoop(
	ctx context.Context,
	base SimpleMiddlewareBase,
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

			out, resp, err := base.HandleClientMsg(ctx, msg)
			if err != nil {
				return err
			}

			if resp != nil {
				pendingToClient = append(pendingToClient, resp)
			}
			if out != nil {
				pendingToDownstream = append(pendingToDownstream, out)
			}

		case msg, ok := <-downstreamSend:
			if !ok {
				return nil // downstream handler finished
			}

			out, err := base.HandleServerMsg(ctx, msg)
			if err != nil {
				return err
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
