package mocrelay

import (
	"context"
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
//
// Practical note on rounding: in normal use the returned duration is
// strictly positive whenever allow() would return false. But the
// float-to-int64 conversion can round (1 - tokens) / rate down to 0 ns
// in extreme cases -- e.g. tokens = 0.9999999999 with a high rate, where
// the true deficit is < 1 ns. readStall guards against the resulting
// tight-spin with a millisecond floor in its sleep loop; treat that
// floor as part of waitDuration's contract in any caller that wants to
// drive a stall loop with the same invariant.
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

// readStall blocks until bucket allows one token, or ctx is done.
//
// This drives the readLoop-level rate limit (RelayOptions.ReadRate /
// ReadBurst), where the policy is to STALL the read rather than reject
// the message: the WebSocket conn.Read is simply not invoked until a
// token is available. Back-pressure propagates naturally to the client
// via the TCP receive window once the OS buffer fills.
//
// A nil bucket means rate limiting is disabled and readStall returns
// immediately without consulting ctx -- the caller pays no overhead.
//
// onStall is invoked exactly once per stall window: when the fast path
// fails and the slow loop is entered. Subsequent allow() failures inside
// the loop (which happen if the clock jitters or refill rate is very
// low) do not re-trigger it. Pass nil to skip. This shape lets callers
// increment a "stalls" Counter (one stall = one client throttled at
// least once) without double-counting noise from the inner loop.
//
// Returns nil on success, ctx.Err() on cancel.
//
// A pre-canceled ctx short-circuits before allow() is consulted, so a
// connection that's already going away cannot debit a token from the
// bucket. This keeps readStall consistent with the readLoop's other ctx
// observation points -- conn.Read(ctx) would also fail immediately on a
// canceled ctx -- and avoids burning bucket capacity on dead conns.
func readStall(ctx context.Context, bucket *tokenBucket, onStall func()) error {
	if bucket == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if bucket.allow(time.Now()) {
		return nil
	}
	if onStall != nil {
		onStall()
	}
	for {
		d := bucket.waitDuration(time.Now())
		// Floor against rounding-to-zero. waitDuration computes
		// (1 - tokens) / rate in float and converts to int64 ns, which can
		// drop to 0 for tiny fractional deficits at high rates. time.After(0)
		// would fire immediately, allow() would still fail, and the loop
		// would tight-spin until refill caught up. A 1ms floor keeps the
		// backoff bounded; real refill windows in production are orders of
		// magnitude larger so the floor is invisible to legitimate callers.
		if d <= 0 {
			d = time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
		if bucket.allow(time.Now()) {
			return nil
		}
	}
}
