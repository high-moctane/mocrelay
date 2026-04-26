package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// helper: send one EVENT, wait, drain a single OK message and return it.
func sendEventDrainOK(t *testing.T, recv chan<- *ClientMsg, send <-chan *ServerMsg, id string) *ServerMsg {
	t.Helper()
	recv <- &ClientMsg{
		Type:  MsgTypeEvent,
		Event: &Event{ID: id, Content: "x"},
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

func TestEventRateLimitMiddleware_AllowWithinBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewEventRateLimitMiddlewareBase(1, 3))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Burst=3 -> three EVENTs in immediate succession all pass through.
		for i := range 3 {
			msg := sendEventDrainOK(t, recv, send, "ok-event")
			if msg.Type != MsgTypeOK {
				t.Errorf("at %d: expected OK, got %v", i, msg.Type)
			}
			if !msg.Accepted {
				t.Errorf("at %d: expected Accepted=true", i)
			}
		}

		cancel()
		<-done
	})
}

func TestEventRateLimitMiddleware_RejectAfterBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewEventRateLimitMiddlewareBase(1, 2))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// First two: accepted
		for i := range 2 {
			msg := sendEventDrainOK(t, recv, send, "rl-event")
			if msg.Type != MsgTypeOK {
				t.Errorf("at %d: expected OK, got %v", i, msg.Type)
			}
			if !msg.Accepted {
				t.Errorf("at %d: expected Accepted=true", i)
			}
		}

		// Third: rate-limited
		msg := sendEventDrainOK(t, recv, send, "rl-event-3")
		if msg.Type != MsgTypeOK {
			t.Errorf("expected OK, got %v", msg.Type)
		}
		if msg.Accepted {
			t.Error("expected Accepted=false")
		}
		if msg.Message != "rate-limited: too many events" {
			t.Errorf("unexpected reason: %q", msg.Message)
		}
		if msg.EventID != "rl-event-3" {
			t.Errorf("expected EventID=%q in OK, got %q", "rl-event-3", msg.EventID)
		}

		cancel()
		<-done
	})
}

func TestEventRateLimitMiddleware_RecoversAfterWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewEventRateLimitMiddlewareBase(2, 1))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// First: accepted (consumes the only burst token)
		first := sendEventDrainOK(t, recv, send, "ev")
		if !first.Accepted {
			t.Error("expected first event accepted")
		}

		// Second (immediately after): rate-limited
		second := sendEventDrainOK(t, recv, send, "ev")
		if second.Accepted {
			t.Error("expected second event rate-limited")
		}

		// Wait 1 second: rate=2 -> 2 tokens accumulated, but capped at burst=1.
		// So one more allowed, then again rate-limited.
		// (synctest.Test bubble: time.Sleep advances the fake clock, no real wait.)
		time.Sleep(1 * time.Second)

		third := sendEventDrainOK(t, recv, send, "ev")
		if !third.Accepted {
			t.Error("expected third event accepted after refill")
		}

		fourth := sendEventDrainOK(t, recv, send, "ev")
		if fourth.Accepted {
			t.Error("expected fourth event rate-limited (burst capped)")
		}

		cancel()
		<-done
	})
}

func TestEventRateLimitMiddleware_NonEventPassThrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewEventRateLimitMiddlewareBase(1, 1))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Drain the EVENT bucket so any EVENT would now be rate-limited.
		first := sendEventDrainOK(t, recv, send, "e1")
		if !first.Accepted {
			t.Fatal("expected first event accepted")
		}

		// REQ should pass through to NopHandler -> EOSE (not rate-limited
		// even though the EVENT bucket is empty).
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
				t.Errorf("expected EOSE for REQ to pass through, got %v", msg.Type)
			}
		default:
			t.Fatal("expected EOSE message but got none")
		}

		cancel()
		<-done
	})
}

func TestEventRateLimitMiddleware_PerConnectionIsolation(t *testing.T) {
	// Two independent connections share the same middleware instance.
	// Draining one must NOT affect the other.
	base := NewEventRateLimitMiddlewareBase(1, 1)

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

			got = sendEventDrainOK(t, recv, send, "e")

			cancel()
			<-done
		})
		return got
	}

	first := runConn(t)
	if !first.Accepted {
		t.Error("conn1: expected first event accepted")
	}

	second := runConn(t)
	if !second.Accepted {
		t.Error("conn2: expected first event accepted (per-connection bucket)")
	}
}

func TestNewEventRateLimitMiddlewareBase_PanicOnZeroRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for rate=0")
		}
	}()
	NewEventRateLimitMiddlewareBase(0, 1)
}

func TestNewEventRateLimitMiddlewareBase_PanicOnNegativeRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for negative rate")
		}
	}()
	NewEventRateLimitMiddlewareBase(-1, 1)
}

func TestNewEventRateLimitMiddlewareBase_PanicOnZeroBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for burst=0")
		}
	}()
	NewEventRateLimitMiddlewareBase(1, 0)
}

func TestNewEventRateLimitMiddlewareBase_PanicOnNegativeBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for negative burst")
		}
	}()
	NewEventRateLimitMiddlewareBase(1, -1)
}
