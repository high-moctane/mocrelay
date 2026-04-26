package mocrelay

import (
	"context"
	"time"
)

// NewCountRateLimitMiddlewareBase returns a middleware base that rate-limits
// incoming COUNT (NIP-45) messages on a per-connection basis using a token
// bucket.
//
// rate is the long-term sustained rate of COUNT messages allowed per
// connection, in COUNTs per second; must be positive.
// burst is the maximum number of COUNT messages a connection may submit
// in a tight window before rate-limiting kicks in. The bucket starts
// full at connection start; must be positive.
//
// When the bucket is empty, the offending COUNT is dropped and a
// `["CLOSED", <sub_id>, "rate-limited: too many counts"]` is sent to the
// client. EVENT / REQ / CLOSE / AUTH messages pass through untouched --
// compose with the matching per-type middleware (Event/Req/Auth) if you
// need to limit those independently.
//
// COUNT can be expensive to compute (it scans the same indexes a REQ
// would but cannot stream early), so a tighter rate than REQ is often
// appropriate. Each connection gets its own token bucket via OnStart +
// context.
//
// Panics if rate <= 0 or burst < 1.
func NewCountRateLimitMiddlewareBase(rate float64, burst int) SimpleMiddlewareBase {
	if rate <= 0 {
		panic("rate must be positive")
	}
	if burst < 1 {
		panic("burst must be positive")
	}
	return &countRateLimitMiddleware{
		rate:  rate,
		burst: burst,
	}
}

type countRateLimitMiddleware struct {
	rate  float64
	burst int
}

type countRateLimitCtxKey struct{}

func (m *countRateLimitMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	tb := newTokenBucket(m.rate, m.burst, time.Now())
	ctx = context.WithValue(ctx, countRateLimitCtxKey{}, tb)
	return ctx, nil, nil
}

func (m *countRateLimitMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *countRateLimitMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeCount {
		return msg, nil, nil
	}

	tb := ctx.Value(countRateLimitCtxKey{}).(*tokenBucket)
	if tb.allow(time.Now()) {
		return msg, nil, nil
	}

	logRejection(ctx, "count_rate_limit", "rate_limited",
		"sub_id", msg.SubscriptionID,
	)
	return nil, NewServerClosedMsg(msg.SubscriptionID, "rate-limited: too many counts"), nil
}

func (m *countRateLimitMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
