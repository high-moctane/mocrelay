package mocrelay

import (
	"math"
	"time"
)

// tokenBucket is an internal token-bucket rate limiter used by the
// per-message-type rate limit middlewares (Event/Req/Count/Auth) and by
// the Relay's readLoop-level stall (RelayOptions.ReadRate).
//
// It is intentionally not safe for concurrent use: each instance is owned
// by a single connection and accessed only from the goroutine that drives
// that connection (a simpleMiddlewarePipelineLoop, or the readLoop itself).
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
// starting full at the given time. Burst is taken as int because messages
// are integral; the bucket itself stores tokens as float64 internally so
// fractional refills (e.g. half a token after 0.5s at rate=1) accumulate
// correctly across calls.
func newTokenBucket(rate float64, burst int, now time.Time) *tokenBucket {
	return &tokenBucket{
		rate:       rate,
		burst:      float64(burst),
		tokens:     float64(burst),
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

// waitDuration returns how long the caller must wait before allow() would
// next succeed. Returns 0 if a token is already available.
//
// This is a pure read of the bucket state: it does not consume tokens and
// does not advance lastUpdate, so calling it twice in a row with the same
// `now` returns the same answer. Callers driving a stall loop call
// waitDuration to compute a sleep target, sleep, then call allow() to
// actually claim the token.
//
// Backwards or zero elapsed time is treated as zero elapsed, mirroring
// allow(): the bucket never credits clock skew, so the reported wait is
// always the conservative "full refill from current state" duration.
func (tb *tokenBucket) waitDuration(now time.Time) time.Duration {
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	tokens := tb.tokens
	if elapsed > 0 {
		tokens = math.Min(tb.burst, tokens+elapsed*tb.rate)
	}
	if tokens >= 1 {
		return 0
	}
	needed := 1 - tokens
	return time.Duration(needed / tb.rate * float64(time.Second))
}
