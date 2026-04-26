package mocrelay

import (
	"context"
	"time"
)

// NewEventRateLimitMiddlewareBase returns a middleware base that rate-limits
// incoming EVENT messages on a per-connection basis using a token bucket.
//
// rate is the long-term sustained rate of EVENT messages allowed per
// connection, in events per second; must be positive.
// burst is the maximum number of EVENT messages a connection may submit
// in a tight window before rate-limiting kicks in. The bucket starts
// full at connection start; must be positive.
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
// Panics if rate <= 0 or burst < 1.
func NewEventRateLimitMiddlewareBase(rate float64, burst int) SimpleMiddlewareBase {
	if rate <= 0 {
		panic("rate must be positive")
	}
	if burst < 1 {
		panic("burst must be positive")
	}
	return &eventRateLimitMiddleware{
		rate:  rate,
		burst: burst,
	}
}

type eventRateLimitMiddleware struct {
	rate  float64
	burst int
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
