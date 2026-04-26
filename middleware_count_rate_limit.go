package mocrelay

import (
	"context"
	"time"
)

// CountRateLimitMiddlewareOptions configures the middleware returned by
// [NewCountRateLimitMiddlewareBase]. Both fields are required; zero or
// negative values cause the constructor to panic.
type CountRateLimitMiddlewareOptions struct {
	// Rate is the long-term sustained rate of COUNT (NIP-45) messages
	// allowed per connection, in COUNTs per second.
	Rate float64

	// Burst is the maximum number of COUNT messages a connection may
	// submit in a tight window before rate-limiting kicks in. The bucket
	// starts full at connection start.
	Burst float64
}

// NewCountRateLimitMiddlewareBase returns a middleware base that rate-limits
// incoming COUNT (NIP-45) messages on a per-connection basis using a token
// bucket.
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
// Panics if opts is nil or if Rate / Burst is not positive.
func NewCountRateLimitMiddlewareBase(opts *CountRateLimitMiddlewareOptions) SimpleMiddlewareBase {
	if opts == nil {
		panic("opts must not be nil")
	}
	if opts.Rate <= 0 {
		panic("Rate must be positive")
	}
	if opts.Burst <= 0 {
		panic("Burst must be positive")
	}
	return &countRateLimitMiddleware{
		rate:  opts.Rate,
		burst: opts.Burst,
	}
}

type countRateLimitMiddleware struct {
	rate  float64
	burst float64
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
