package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
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

func TestTokenBucket_WaitDurationZeroWhenAvailable(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 3, now)

	// Initially full, waitDuration must be 0.
	if d := tb.waitDuration(now); d != 0 {
		t.Fatalf("expected 0 at full bucket, got %v", d)
	}

	// After consuming 1 of 3, still 2 left -> waitDuration still 0.
	if !tb.allow(now) {
		t.Fatal("expected initial allow")
	}
	if d := tb.waitDuration(now); d != 0 {
		t.Fatalf("expected 0 with tokens remaining, got %v", d)
	}
}

func TestTokenBucket_WaitDurationAfterExhaustion(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(2, 1, now) // 2 tokens/sec, burst 1

	// Exhaust the burst.
	if !tb.allow(now) {
		t.Fatal("expected initial allow")
	}

	// At rate=2, one token refills in 0.5s exactly.
	if d := tb.waitDuration(now); d != 500*time.Millisecond {
		t.Fatalf("expected 500ms after exhaustion, got %v", d)
	}
}

func TestTokenBucket_WaitDurationAccountsForElapsed(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 1, now) // 1 token/sec, burst 1

	// Exhaust the burst.
	if !tb.allow(now) {
		t.Fatal("expected initial allow")
	}

	// After 0.3s elapsed, 0.3 token has accumulated lazily; we still need
	// 0.7 more, which at rate=1 means 0.7s of additional wait.
	now = now.Add(300 * time.Millisecond)
	if d := tb.waitDuration(now); d != 700*time.Millisecond {
		t.Fatalf("expected 700ms with 0.3s elapsed, got %v", d)
	}
}

func TestTokenBucket_WaitDurationDoesNotMutateState(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 1, now) // 1 token/sec, burst 1

	// Exhaust the burst.
	if !tb.allow(now) {
		t.Fatal("expected initial allow")
	}

	// Calling waitDuration repeatedly with the same `now` must return the
	// same answer -- it must not credit elapsed time on its own.
	later := now.Add(400 * time.Millisecond)
	d1 := tb.waitDuration(later)
	d2 := tb.waitDuration(later)
	if d1 != d2 {
		t.Fatalf("waitDuration not idempotent: %v vs %v", d1, d2)
	}

	// And after we wait the reported duration, allow at that exact instant
	// must succeed -- proving waitDuration isn't lying about when refill
	// completes (and hasn't silently consumed a token already).
	target := later.Add(d1)
	if !tb.allow(target) {
		t.Fatalf("expected allow at later+waitDuration (=%v), got reject", target)
	}
}

func TestTokenBucket_WaitDurationBackwardsClockIgnored(t *testing.T) {
	// Mirroring TestTokenBucket_BackwardsClockIgnored: a backwards clock
	// must not credit negative elapsed time. waitDuration with `earlier`
	// should report the full refill window (1s at rate=1), same as if no
	// time had passed at all -- never a negative or wrapped duration.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tb := newTokenBucket(1, 1, start)

	if !tb.allow(start) {
		t.Fatal("expected initial allow")
	}

	earlier := start.Add(-10 * time.Second)
	if d := tb.waitDuration(earlier); d != time.Second {
		t.Fatalf("expected 1s with backwards clock, got %v", d)
	}
}

func TestReadStall_NilBucket(t *testing.T) {
	// A nil bucket means rate limiting is disabled. readStall must return
	// immediately without ever consulting ctx.
	if err := readStall(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected nil error with nil bucket, got %v", err)
	}
}

func TestReadStall_FastPathDoesNotCallOnStall(t *testing.T) {
	// When the bucket has tokens available, readStall returns immediately
	// and onStall must NOT fire -- the callback exists to count actual
	// stalls, not every successful read.
	now := time.Now()
	bucket := newTokenBucket(1, 3, now)

	called := 0
	if err := readStall(context.Background(), bucket, func() { called++ }); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if called != 0 {
		t.Fatalf("onStall called %d times on fast path; expected 0", called)
	}
}

func TestReadStall_StallsUntilTokenAvailable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bucket := newTokenBucket(1, 1, time.Now())
		// Consume the burst -- next allow needs ~1s of refill.
		if !bucket.allow(time.Now()) {
			t.Fatal("expected initial allow")
		}

		called := 0
		start := time.Now()
		err := readStall(context.Background(), bucket, func() { called++ })
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		// At rate=1, full refill takes ~1s. synctest fake clock should
		// advance time deterministically to that moment.
		if elapsed < time.Second {
			t.Fatalf("expected >=1s stall, got %v", elapsed)
		}
		// onStall fires exactly once per stall window: when the slow path
		// kicks in. Subsequent re-checks inside the loop must not double-count.
		if called != 1 {
			t.Fatalf("expected onStall called once, got %d", called)
		}
	})
}

func TestReadStall_CtxCancelExits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bucket := newTokenBucket(1, 1, time.Now())
		if !bucket.allow(time.Now()) {
			t.Fatal("expected initial allow")
		}

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel partway through the stall -- well before the 1s refill.
		time.AfterFunc(100*time.Millisecond, cancel)

		err := readStall(ctx, bucket, nil)
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}
