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

			errs := make(chan error, 2)

			// Goroutine 1: Handle recv direction (client -> downstream)
			go func() {
				defer close(downstreamRecv)
				errs <- simpleMiddlewareRecvLoop(ctx, base, recv, send, downstreamRecv)
			}()

			// Goroutine 2: Handle send direction (downstream -> client)
			go func() {
				errs <- simpleMiddlewareSendLoop(ctx, base, downstreamSend, send)
			}()

			// Run downstream handler
			handlerErr := next.ServeNostr(ctx, downstreamSend, downstreamRecv)

			// Cancel and wait for goroutines
			cancel()
			err1, err2 := <-errs, <-errs

			err = errors.Join(handlerErr, err1, err2)
			return
		})
	}
}

// simpleMiddlewareRecvLoop handles the recv direction.
func simpleMiddlewareRecvLoop(
	ctx context.Context,
	base SimpleMiddlewareBase,
	recv <-chan *ClientMsg,
	send chan<- *ServerMsg,
	downstreamRecv chan<- *ClientMsg,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				return nil // recv closed, normal termination
			}

			out, resp, err := base.HandleClientMsg(ctx, msg)
			if err != nil {
				return err
			}

			// Send response to client if provided
			if resp != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case send <- resp:
				}
			}

			// Pass to downstream if out is provided
			if out != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case downstreamRecv <- out:
				}
			}
		}
	}
}

// simpleMiddlewareSendLoop handles the send direction.
func simpleMiddlewareSendLoop(
	ctx context.Context,
	base SimpleMiddlewareBase,
	downstreamSend <-chan *ServerMsg,
	send chan<- *ServerMsg,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-downstreamSend:
			if !ok {
				return nil // downstream closed
			}

			out, err := base.HandleServerMsg(ctx, msg)
			if err != nil {
				return err
			}

			// Send to client if out is provided
			if out != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case send <- out:
				}
			}
		}
	}
}
