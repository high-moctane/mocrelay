package mocrelay

import (
	"context"
	"time"
)

// EventRateLimitMiddlewareOptions configures the middleware returned by
// [NewEventRateLimitMiddlewareBase].
//
// Both fields are required. Zero or negative values cause the constructor
// to panic, mirroring [NewMaxContentLengthMiddlewareBase] and friends.
type EventRateLimitMiddlewareOptions struct {
	// Rate is the long-term sustained rate of EVENT messages allowed per
	// connection, in events per second.
	Rate float64

	// Burst is the maximum number of EVENT messages a connection may submit
	// in a tight window before rate-limiting kicks in. The bucket starts
	// full at connection start.
	Burst float64
}

// NewEventRateLimitMiddlewareBase returns a middleware base that rate-limits
// incoming EVENT messages on a per-connection basis using a token bucket.
//
// When the bucket is empty, the offending EVENT is dropped and a
// `["OK", <id>, false, "rate-limited: too many events"]` is sent to the
// client (NIP-01 standard prefix). REQ / CLOSE / AUTH / COUNT messages
// pass through untouched -- compose with the matching per-type middleware
// (Req/Count/Auth) if you need to limit those independently.
//
// The token bucket is created in OnStart and stored in the request
// context, so each connection has its own independent bucket. Counters
// are not shared across connections.
//
// Panics if opts is nil or if Rate / Burst is not positive.
func NewEventRateLimitMiddlewareBase(opts *EventRateLimitMiddlewareOptions) SimpleMiddlewareBase {
	if opts == nil {
		panic("opts must not be nil")
	}
	if opts.Rate <= 0 {
		panic("Rate must be positive")
	}
	if opts.Burst <= 0 {
		panic("Burst must be positive")
	}
	return &eventRateLimitMiddleware{
		rate:  opts.Rate,
		burst: opts.Burst,
	}
}

type eventRateLimitMiddleware struct {
	rate  float64
	burst float64
}

type eventRateLimitCtxKey struct{}

func (m *eventRateLimitMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	tb := newTokenBucket(m.rate, m.burst, time.Now())
	ctx = context.WithValue(ctx, eventRateLimitCtxKey{}, tb)
	return ctx, nil, nil
}

func (m *eventRateLimitMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *eventRateLimitMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	tb := ctx.Value(eventRateLimitCtxKey{}).(*tokenBucket)
	if tb.allow(time.Now()) {
		return msg, nil, nil
	}

	eventID := ""
	if msg.Event != nil {
		eventID = msg.Event.ID
	}
	logRejection(ctx, "event_rate_limit", "rate_limited",
		"event_id", eventID,
	)
	return nil, NewServerOKMsg(eventID, false, "rate-limited: too many events"), nil
}

func (m *eventRateLimitMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
