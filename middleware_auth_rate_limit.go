package mocrelay

import (
	"context"
	"time"
)

// NewAuthRateLimitMiddlewareBase returns a middleware base that rate-limits
// incoming AUTH (NIP-42) messages on a per-connection basis using a token
// bucket.
//
// rate is the long-term sustained rate of AUTH messages allowed per
// connection, in AUTHs per second; must be positive.
// burst is the maximum number of AUTH messages a connection may submit
// in a tight window before rate-limiting kicks in. The bucket starts
// full at connection start; must be positive.
//
// When the bucket is empty, the offending AUTH is dropped and a
// `["OK", <event_id>, false, "rate-limited: too many auth attempts"]` is
// sent to the client. NIP-42 specifies AUTH responses are OK messages, so
// the response shape matches a successful AUTH except for accepted=false.
//
// EVENT / REQ / CLOSE / COUNT messages pass through untouched -- compose
// with the matching per-type middleware (Event/Req/Count) if you need to
// limit those independently.
//
// Use this middleware to defend against AUTH challenge-response brute
// force; a typical setting is a low rate (e.g. 0.1 = one attempt every
// 10 seconds long-term) with a small burst (e.g. 3) so legitimate
// reconnects still work but credential stuffing is throttled.
//
// Each connection gets its own token bucket via OnStart + context.
//
// Panics if rate <= 0 or burst < 1.
func NewAuthRateLimitMiddlewareBase(rate float64, burst int) SimpleMiddlewareBase {
	if rate <= 0 {
		panic("rate must be positive")
	}
	if burst < 1 {
		panic("burst must be positive")
	}
	return &authRateLimitMiddleware{
		rate:  rate,
		burst: burst,
	}
}

type authRateLimitMiddleware struct {
	rate  float64
	burst int
}

type authRateLimitCtxKey struct{}

func (m *authRateLimitMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	tb := newTokenBucket(m.rate, m.burst, time.Now())
	ctx = context.WithValue(ctx, authRateLimitCtxKey{}, tb)
	return ctx, nil, nil
}

func (m *authRateLimitMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *authRateLimitMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeAuth {
		return msg, nil, nil
	}

	tb := ctx.Value(authRateLimitCtxKey{}).(*tokenBucket)
	if tb.allow(time.Now()) {
		return msg, nil, nil
	}

	eventID := ""
	if msg.Event != nil {
		eventID = msg.Event.ID
	}
	logRejection(ctx, "auth_rate_limit", "rate_limited",
		"event_id", eventID,
	)
	return nil, NewServerOKMsg(eventID, false, "rate-limited: too many auth attempts"), nil
}

func (m *authRateLimitMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
