package mocrelay

import (
	"math"
	"time"
)

// tokenBucket is an internal token-bucket rate limiter used by the
// per-message-type rate limit middlewares (Event/Req/Count/Auth).
//
// It is intentionally not safe for concurrent use: each instance is owned
// by a single connection and accessed only from the simpleMiddlewarePipelineLoop
// goroutine that drives that connection.
//
// The bucket starts full (tokens = burst) so a freshly-connected client can
// burst up to `burst` requests immediately, then is rate-limited to `rate`
// requests per second long-term.
type tokenBucket struct {
	rate       float64 // tokens added per second
	burst      float64 // maximum tokens the bucket can hold
	tokens     float64 // current tokens
	lastUpdate time.Time
}

// newTokenBucket constructs a token bucket with the given rate and burst,
// starting full at the given time.
func newTokenBucket(rate, burst float64, now time.Time) *tokenBucket {
	return &tokenBucket{
		rate:       rate,
		burst:      burst,
		tokens:     burst,
		lastUpdate: now,
	}
}

// allow attempts to consume one token. Returns true if a token was available
// (the request is permitted), false if the bucket is empty (the request must
// be rate-limited).
//
// The bucket is refilled lazily at call time based on elapsed wall-clock time
// since the last call. Backwards or zero elapsed time is treated as zero
// elapsed -- the bucket never gains tokens from clock skew.
func (tb *tokenBucket) allow(now time.Time) bool {
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	if elapsed > 0 {
		tb.tokens = math.Min(tb.burst, tb.tokens+elapsed*tb.rate)
		tb.lastUpdate = now
	}

	if tb.tokens >= 1 {
		tb.tokens -= 1
		return true
	}
	return false
}
