package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// helper: send one COUNT, wait, drain a single message and return it.
func sendCountDrain(t *testing.T, recv chan<- *ClientMsg, send <-chan *ServerMsg, subID string) *ServerMsg {
	t.Helper()
	limit := int64(0)
	recv <- &ClientMsg{
		Type:           MsgTypeCount,
		SubscriptionID: subID,
		Filters:        []*ReqFilter{{Limit: &limit}},
	}
	synctest.Wait()
	select {
	case msg := <-send:
		return msg
	default:
		t.Fatal("expected message but got none")
		return nil
	}
}

func TestCountRateLimitMiddleware_AllowWithinBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewCountRateLimitMiddlewareBase(
			&CountRateLimitMiddlewareOptions{Rate: 1, Burst: 3},
		))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Burst=3 -> three COUNTs reach NopHandler -> COUNT response with count=0.
		for i := range 3 {
			msg := sendCountDrain(t, recv, send, "sub")
			if msg.Type != MsgTypeCount {
				t.Errorf("at %d: expected COUNT, got %v", i, msg.Type)
			}
		}

		cancel()
		<-done
	})
}

func TestCountRateLimitMiddleware_RejectAfterBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewCountRateLimitMiddlewareBase(
			&CountRateLimitMiddlewareOptions{Rate: 1, Burst: 2},
		))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// First two: COUNT response from NopHandler
		for i := range 2 {
			msg := sendCountDrain(t, recv, send, "sub")
			if msg.Type != MsgTypeCount {
				t.Errorf("at %d: expected COUNT, got %v", i, msg.Type)
			}
		}

		// Third: CLOSED with rate-limited prefix
		msg := sendCountDrain(t, recv, send, "sub-3")
		if msg.Type != MsgTypeClosed {
			t.Errorf("expected CLOSED, got %v", msg.Type)
		}
		if msg.Message != "rate-limited: too many counts" {
			t.Errorf("unexpected reason: %q", msg.Message)
		}
		if msg.SubscriptionID != "sub-3" {
			t.Errorf("expected sub_id=%q, got %q", "sub-3", msg.SubscriptionID)
		}

		cancel()
		<-done
	})
}

func TestCountRateLimitMiddleware_RecoversAfterWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewCountRateLimitMiddlewareBase(
			&CountRateLimitMiddlewareOptions{Rate: 2, Burst: 1},
		))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		first := sendCountDrain(t, recv, send, "s")
		if first.Type != MsgTypeCount {
			t.Errorf("expected COUNT, got %v", first.Type)
		}

		second := sendCountDrain(t, recv, send, "s")
		if second.Type != MsgTypeClosed {
			t.Errorf("expected CLOSED for rate-limited COUNT, got %v", second.Type)
		}

		// Wait 1 second: bucket refilled (capped at burst=1)
		time.Sleep(1 * time.Second)

		third := sendCountDrain(t, recv, send, "s")
		if third.Type != MsgTypeCount {
			t.Errorf("expected COUNT after refill, got %v", third.Type)
		}

		fourth := sendCountDrain(t, recv, send, "s")
		if fourth.Type != MsgTypeClosed {
			t.Errorf("expected CLOSED (burst capped), got %v", fourth.Type)
		}

		cancel()
		<-done
	})
}

func TestCountRateLimitMiddleware_NonCountPassThrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewCountRateLimitMiddlewareBase(
			&CountRateLimitMiddlewareOptions{Rate: 1, Burst: 1},
		))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Drain the COUNT bucket
		first := sendCountDrain(t, recv, send, "s1")
		if first.Type != MsgTypeCount {
			t.Fatalf("expected COUNT, got %v", first.Type)
		}

		// REQ must pass through (COUNT bucket empty does NOT affect REQ)
		limit := int64(0)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &limit}},
		}
		synctest.Wait()
		select {
		case msg := <-send:
			if msg.Type != MsgTypeEOSE {
				t.Errorf("expected EOSE for REQ pass-through, got %v", msg.Type)
			}
		default:
			t.Fatal("expected REQ response")
		}

		cancel()
		<-done
	})
}

func TestCountRateLimitMiddleware_PerConnectionIsolation(t *testing.T) {
	base := NewCountRateLimitMiddlewareBase(
		&CountRateLimitMiddlewareOptions{Rate: 1, Burst: 1},
	)

	runConn := func(t *testing.T) *ServerMsg {
		var got *ServerMsg
		synctest.Test(t, func(t *testing.T) {
			handler := NewSimpleMiddleware(base)(NewNopHandler())
			recv := make(chan *ClientMsg)
			send := make(chan *ServerMsg, 4)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error)
			go func() { done <- handler.ServeNostr(ctx, send, recv) }()

			got = sendCountDrain(t, recv, send, "s")

			cancel()
			<-done
		})
		return got
	}

	first := runConn(t)
	if first.Type != MsgTypeCount {
		t.Errorf("conn1: expected COUNT, got %v", first.Type)
	}
	second := runConn(t)
	if second.Type != MsgTypeCount {
		t.Errorf("conn2: expected COUNT (per-connection bucket), got %v", second.Type)
	}
}

func TestNewCountRateLimitMiddlewareBase_PanicOnNilOpts(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil opts")
		}
	}()
	NewCountRateLimitMiddlewareBase(nil)
}

func TestNewCountRateLimitMiddlewareBase_PanicOnZeroRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Rate=0")
		}
	}()
	NewCountRateLimitMiddlewareBase(&CountRateLimitMiddlewareOptions{Rate: 0, Burst: 1})
}

func TestNewCountRateLimitMiddlewareBase_PanicOnZeroBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Burst=0")
		}
	}()
	NewCountRateLimitMiddlewareBase(&CountRateLimitMiddlewareOptions{Rate: 1, Burst: 0})
}
