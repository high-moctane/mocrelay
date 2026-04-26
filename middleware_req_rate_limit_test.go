package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// helper: send one REQ, wait, drain a single message and return it.
func sendReqDrain(t *testing.T, recv chan<- *ClientMsg, send <-chan *ServerMsg, subID string) *ServerMsg {
	t.Helper()
	limit := int64(0)
	recv <- &ClientMsg{
		Type:           MsgTypeReq,
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

func TestReqRateLimitMiddleware_AllowWithinBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewReqRateLimitMiddlewareBase(1, 3))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Burst=3 -> three REQs in immediate succession all reach NopHandler -> EOSE.
		for i := range 3 {
			msg := sendReqDrain(t, recv, send, "sub")
			if msg.Type != MsgTypeEOSE {
				t.Errorf("at %d: expected EOSE, got %v", i, msg.Type)
			}
		}

		cancel()
		<-done
	})
}

func TestReqRateLimitMiddleware_RejectAfterBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewReqRateLimitMiddlewareBase(1, 2))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// First two: EOSE
		for i := range 2 {
			msg := sendReqDrain(t, recv, send, "sub")
			if msg.Type != MsgTypeEOSE {
				t.Errorf("at %d: expected EOSE, got %v", i, msg.Type)
			}
		}

		// Third: CLOSED with rate-limited prefix
		msg := sendReqDrain(t, recv, send, "sub-3")
		if msg.Type != MsgTypeClosed {
			t.Errorf("expected CLOSED, got %v", msg.Type)
		}
		if msg.Message != "rate-limited: too many subscriptions" {
			t.Errorf("unexpected reason: %q", msg.Message)
		}
		if msg.SubscriptionID != "sub-3" {
			t.Errorf("expected sub_id=%q, got %q", "sub-3", msg.SubscriptionID)
		}

		cancel()
		<-done
	})
}

func TestReqRateLimitMiddleware_RecoversAfterWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewReqRateLimitMiddlewareBase(2, 1))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		first := sendReqDrain(t, recv, send, "s")
		if first.Type != MsgTypeEOSE {
			t.Errorf("expected EOSE, got %v", first.Type)
		}

		second := sendReqDrain(t, recv, send, "s")
		if second.Type != MsgTypeClosed {
			t.Errorf("expected CLOSED for rate-limited REQ, got %v", second.Type)
		}

		// Wait 1 second: bucket refilled (capped at burst=1)
		// (synctest.Test bubble: time.Sleep advances the fake clock, no real wait.)
		time.Sleep(1 * time.Second)

		third := sendReqDrain(t, recv, send, "s")
		if third.Type != MsgTypeEOSE {
			t.Errorf("expected EOSE after refill, got %v", third.Type)
		}

		fourth := sendReqDrain(t, recv, send, "s")
		if fourth.Type != MsgTypeClosed {
			t.Errorf("expected CLOSED (burst capped), got %v", fourth.Type)
		}

		cancel()
		<-done
	})
}

func TestReqRateLimitMiddleware_NonReqPassThrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewReqRateLimitMiddlewareBase(1, 1))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Drain the REQ bucket
		first := sendReqDrain(t, recv, send, "s1")
		if first.Type != MsgTypeEOSE {
			t.Fatalf("expected EOSE, got %v", first.Type)
		}

		// EVENT must pass through (REQ bucket empty does NOT affect EVENT)
		recv <- &ClientMsg{
			Type:  MsgTypeEvent,
			Event: &Event{ID: "ev", Content: "x"},
		}
		synctest.Wait()
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK for EVENT pass-through, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Error("expected EVENT accepted")
			}
		default:
			t.Fatal("expected EVENT response")
		}

		cancel()
		<-done
	})
}

func TestReqRateLimitMiddleware_PerConnectionIsolation(t *testing.T) {
	base := NewReqRateLimitMiddlewareBase(1, 1)

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

			got = sendReqDrain(t, recv, send, "s")

			cancel()
			<-done
		})
		return got
	}

	first := runConn(t)
	if first.Type != MsgTypeEOSE {
		t.Errorf("conn1: expected EOSE, got %v", first.Type)
	}
	second := runConn(t)
	if second.Type != MsgTypeEOSE {
		t.Errorf("conn2: expected EOSE (per-connection bucket), got %v", second.Type)
	}
}

func TestNewReqRateLimitMiddlewareBase_PanicOnZeroRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for rate=0")
		}
	}()
	NewReqRateLimitMiddlewareBase(0, 1)
}

func TestNewReqRateLimitMiddlewareBase_PanicOnNegativeRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for negative rate")
		}
	}()
	NewReqRateLimitMiddlewareBase(-1, 1)
}

func TestNewReqRateLimitMiddlewareBase_PanicOnZeroBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for burst=0")
		}
	}()
	NewReqRateLimitMiddlewareBase(1, 0)
}

func TestNewReqRateLimitMiddlewareBase_PanicOnNegativeBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for negative burst")
		}
	}()
	NewReqRateLimitMiddlewareBase(1, -1)
}
