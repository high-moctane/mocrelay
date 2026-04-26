package mocrelay

import (
	"testing"
	"time"
)

func TestTokenBucket_InitialBurstAllowed(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 5, now)

	// Should allow exactly burst (5) consecutive requests at t=0
	for i := range 5 {
		if !tb.allow(now) {
			t.Fatalf("expected allow at iteration %d", i)
		}
	}

	// 6th must be rejected: bucket exhausted, no time has passed
	if tb.allow(now) {
		t.Fatal("expected reject after exhausting burst")
	}
}

func TestTokenBucket_RefillByElapsed(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(2, 1, now) // 2 tokens/sec, burst 1

	// Consume the only burst token
	if !tb.allow(now) {
		t.Fatal("expected initial allow")
	}
	if tb.allow(now) {
		t.Fatal("expected immediate reject")
	}

	// After 0.5s exactly 1 token should be available (rate=2 -> 1 per 0.5s)
	now = now.Add(500 * time.Millisecond)
	if !tb.allow(now) {
		t.Fatal("expected allow after 0.5s refill")
	}

	// And then immediately reject again (we drained it)
	if tb.allow(now) {
		t.Fatal("expected reject after consuming refilled token")
	}
}

func TestTokenBucket_RefillCapsAtBurst(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 3, now) // 1 token/sec, burst 3

	// Exhaust
	for i := range 3 {
		if !tb.allow(now) {
			t.Fatalf("expected initial allow at %d", i)
		}
	}

	// Wait 100 seconds - well more than enough to fill way past burst
	now = now.Add(100 * time.Second)

	// Should allow exactly 3 (capped at burst), not 100
	for i := range 3 {
		if !tb.allow(now) {
			t.Fatalf("expected allow at %d after long wait", i)
		}
	}

	if tb.allow(now) {
		t.Fatal("expected reject: refill must cap at burst, not accumulate")
	}
}

func TestTokenBucket_FractionalAccumulation(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 1, now) // 1 token/sec, burst 1

	// Consume initial token
	if !tb.allow(now) {
		t.Fatal("expected initial allow")
	}

	// Two consecutive 0.6s waits: each accumulates 0.6 token, but the
	// first 0.6 is below the 1.0 threshold so allow must reject;
	// after the second wait the bucket has min(1, 1.2) = 1.0 -> allow.
	now = now.Add(600 * time.Millisecond)
	if tb.allow(now) {
		t.Fatal("expected reject at 0.6s (only 0.6 token accumulated)")
	}

	now = now.Add(600 * time.Millisecond)
	if !tb.allow(now) {
		t.Fatal("expected allow at 1.2s (token threshold reached)")
	}
}

func TestTokenBucket_BackwardsClockIgnored(t *testing.T) {
	// If somehow now goes backwards (clock skew, test mistake, ...) the
	// bucket must not credit negative time and start handing out tokens
	// on every call. Pin lastUpdate forward by ignoring elapsed <= 0.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 1, start)

	if !tb.allow(start) {
		t.Fatal("expected initial allow")
	}

	// Move backward 10s -- should still be rejected, not refilled
	earlier := start.Add(-10 * time.Second)
	if tb.allow(earlier) {
		t.Fatal("expected reject when clock moves backwards")
	}
}
