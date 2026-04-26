package mocrelay

import (
	"context"
	"time"
)

// ReqRateLimitMiddlewareOptions configures the middleware returned by
// [NewReqRateLimitMiddlewareBase]. Both fields are required; zero or
// negative values cause the constructor to panic.
type ReqRateLimitMiddlewareOptions struct {
	// Rate is the long-term sustained rate of REQ messages allowed per
	// connection, in REQs per second.
	Rate float64

	// Burst is the maximum number of REQ messages a connection may submit
	// in a tight window before rate-limiting kicks in. The bucket starts
	// full at connection start.
	Burst float64
}

// NewReqRateLimitMiddlewareBase returns a middleware base that rate-limits
// incoming REQ messages on a per-connection basis using a token bucket.
//
// When the bucket is empty, the offending REQ is dropped and a
// `["CLOSED", <sub_id>, "rate-limited: too many subscriptions"]` is sent
// to the client. EVENT / CLOSE / AUTH / COUNT messages pass through
// untouched -- compose with the matching per-type middleware
// (Event/Count/Auth) if you need to limit those independently.
//
// Each connection gets its own token bucket via OnStart + context.
//
// Panics if opts is nil or if Rate / Burst is not positive.
func NewReqRateLimitMiddlewareBase(opts *ReqRateLimitMiddlewareOptions) SimpleMiddlewareBase {
	if opts == nil {
		panic("opts must not be nil")
	}
	if opts.Rate <= 0 {
		panic("Rate must be positive")
	}
	if opts.Burst <= 0 {
		panic("Burst must be positive")
	}
	return &reqRateLimitMiddleware{
		rate:  opts.Rate,
		burst: opts.Burst,
	}
}

type reqRateLimitMiddleware struct {
	rate  float64
	burst float64
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
