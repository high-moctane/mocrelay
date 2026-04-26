package mocrelay

import (
	"context"
	"time"
)

// NewReqRateLimitMiddlewareBase returns a middleware base that rate-limits
// incoming REQ messages on a per-connection basis using a token bucket.
//
// rate is the long-term sustained rate of REQ messages allowed per
// connection, in REQs per second; must be positive.
// burst is the maximum number of REQ messages a connection may submit
// in a tight window before rate-limiting kicks in. The bucket starts
// full at connection start; must be positive.
//
// When the bucket is empty, the offending REQ is dropped and a
// `["CLOSED", <sub_id>, "rate-limited: too many subscriptions"]` is sent
// to the client. EVENT / CLOSE / AUTH / COUNT messages pass through
// untouched -- compose with the matching per-type middleware
// (Event/Count/Auth) if you need to limit those independently.
//
// Each connection gets its own token bucket via OnStart + context.
//
// Panics if rate <= 0 or burst < 1.
func NewReqRateLimitMiddlewareBase(rate float64, burst int) SimpleMiddlewareBase {
	if rate <= 0 {
		panic("rate must be positive")
	}
	if burst < 1 {
		panic("burst must be positive")
	}
	return &reqRateLimitMiddleware{
		rate:  rate,
		burst: burst,
	}
}

type reqRateLimitMiddleware struct {
	rate  float64
	burst int
}

type reqRateLimitCtxKey struct{}

func (m *reqRateLimitMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	tb := newTokenBucket(m.rate, m.burst, time.Now())
	ctx = context.WithValue(ctx, reqRateLimitCtxKey{}, tb)
	return ctx, nil, nil
}

func (m *reqRateLimitMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *reqRateLimitMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeReq {
		return msg, nil, nil
	}

	tb := ctx.Value(reqRateLimitCtxKey{}).(*tokenBucket)
	if tb.allow(time.Now()) {
		return msg, nil, nil
	}

	logRejection(ctx, "req_rate_limit", "rate_limited",
		"sub_id", msg.SubscriptionID,
	)
	return nil, NewServerClosedMsg(msg.SubscriptionID, "rate-limited: too many subscriptions"), nil
}

func (m *reqRateLimitMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
