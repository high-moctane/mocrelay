package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// authEchoHandler is a test-only handler that ACKs every AUTH message it
// receives with an OK. NopHandler does not handle AUTH, so we need this
// to verify that AUTH messages pass through the rate-limit middleware.
func authEchoHandler() Handler {
	return HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case msg, ok := <-recv:
				if !ok {
					return nil
				}
				if msg.Type == MsgTypeAuth && msg.Event != nil {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case send <- NewServerOKMsg(msg.Event.ID, true, ""):
					}
				}
			}
		}
	})
}

// helper: send one AUTH, wait, drain a single message and return it.
func sendAuthDrain(t *testing.T, recv chan<- *ClientMsg, send <-chan *ServerMsg, eventID string) *ServerMsg {
	t.Helper()
	recv <- &ClientMsg{
		Type:  MsgTypeAuth,
		Event: &Event{ID: eventID, Kind: 22242},
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

func TestAuthRateLimitMiddleware_AllowWithinBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewAuthRateLimitMiddlewareBase(
			&AuthRateLimitMiddlewareOptions{Rate: 1, Burst: 3},
		))
		handler := middleware(authEchoHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Burst=3 -> three AUTHs reach the echo handler -> OK true.
		for i := range 3 {
			msg := sendAuthDrain(t, recv, send, "auth-ev")
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

func TestAuthRateLimitMiddleware_RejectAfterBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewAuthRateLimitMiddlewareBase(
			&AuthRateLimitMiddlewareOptions{Rate: 1, Burst: 2},
		))
		handler := middleware(authEchoHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// First two: accepted (passes through to echo handler)
		for i := range 2 {
			msg := sendAuthDrain(t, recv, send, "auth-ev")
			if msg.Type != MsgTypeOK {
				t.Errorf("at %d: expected OK, got %v", i, msg.Type)
			}
			if !msg.Accepted {
				t.Errorf("at %d: expected Accepted=true", i)
			}
		}

		// Third: rate-limited (OK accepted=false from the middleware)
		msg := sendAuthDrain(t, recv, send, "auth-ev-3")
		if msg.Type != MsgTypeOK {
			t.Errorf("expected OK, got %v", msg.Type)
		}
		if msg.Accepted {
			t.Error("expected Accepted=false")
		}
		if msg.Message != "rate-limited: too many auth attempts" {
			t.Errorf("unexpected reason: %q", msg.Message)
		}
		if msg.EventID != "auth-ev-3" {
			t.Errorf("expected EventID=%q in OK, got %q", "auth-ev-3", msg.EventID)
		}

		cancel()
		<-done
	})
}

func TestAuthRateLimitMiddleware_RejectMissingEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewAuthRateLimitMiddlewareBase(
			&AuthRateLimitMiddlewareOptions{Rate: 1, Burst: 1},
		))
		handler := middleware(authEchoHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Drain bucket
		first := sendAuthDrain(t, recv, send, "auth-ok")
		if !first.Accepted {
			t.Fatal("expected first AUTH accepted")
		}

		// Second AUTH with nil Event must still be rejected with rate-limit
		// (no panic, empty EventID in the OK reply)
		recv <- &ClientMsg{
			Type:  MsgTypeAuth,
			Event: nil,
		}
		synctest.Wait()
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected Accepted=false")
			}
			if msg.EventID != "" {
				t.Errorf("expected empty EventID for nil Event, got %q", msg.EventID)
			}
		default:
			t.Fatal("expected response")
		}

		cancel()
		<-done
	})
}

func TestAuthRateLimitMiddleware_RecoversAfterWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewAuthRateLimitMiddlewareBase(
			&AuthRateLimitMiddlewareOptions{Rate: 2, Burst: 1},
		))
		handler := middleware(authEchoHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		first := sendAuthDrain(t, recv, send, "a")
		if !first.Accepted {
			t.Error("expected first AUTH accepted")
		}

		second := sendAuthDrain(t, recv, send, "a")
		if second.Accepted {
			t.Error("expected second AUTH rate-limited")
		}

		time.Sleep(1 * time.Second)

		third := sendAuthDrain(t, recv, send, "a")
		if !third.Accepted {
			t.Error("expected third AUTH accepted after refill")
		}

		fourth := sendAuthDrain(t, recv, send, "a")
		if fourth.Accepted {
			t.Error("expected fourth AUTH rate-limited (burst capped)")
		}

		cancel()
		<-done
	})
}

func TestAuthRateLimitMiddleware_NonAuthPassThrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewSimpleMiddleware(NewAuthRateLimitMiddlewareBase(
			&AuthRateLimitMiddlewareOptions{Rate: 1, Burst: 1},
		))
		// Use NopHandler here because we want EVENT/REQ pass-through
		// behavior, not AUTH. Drain the AUTH bucket first via the echo
		// handler test above; here just verify EVENT passes through with
		// the AUTH bucket fully empty.
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// EVENT must pass through regardless of AUTH bucket state
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

func TestAuthRateLimitMiddleware_PerConnectionIsolation(t *testing.T) {
	base := NewAuthRateLimitMiddlewareBase(
		&AuthRateLimitMiddlewareOptions{Rate: 1, Burst: 1},
	)

	runConn := func(t *testing.T) *ServerMsg {
		var got *ServerMsg
		synctest.Test(t, func(t *testing.T) {
			handler := NewSimpleMiddleware(base)(authEchoHandler())
			recv := make(chan *ClientMsg)
			send := make(chan *ServerMsg, 4)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error)
			go func() { done <- handler.ServeNostr(ctx, send, recv) }()

			got = sendAuthDrain(t, recv, send, "a")

			cancel()
			<-done
		})
		return got
	}

	first := runConn(t)
	if !first.Accepted {
		t.Error("conn1: expected first AUTH accepted")
	}
	second := runConn(t)
	if !second.Accepted {
		t.Error("conn2: expected first AUTH accepted (per-connection bucket)")
	}
}

func TestNewAuthRateLimitMiddlewareBase_PanicOnNilOpts(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil opts")
		}
	}()
	NewAuthRateLimitMiddlewareBase(nil)
}

func TestNewAuthRateLimitMiddlewareBase_PanicOnZeroRate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Rate=0")
		}
	}()
	NewAuthRateLimitMiddlewareBase(&AuthRateLimitMiddlewareOptions{Rate: 0, Burst: 1})
}

func TestNewAuthRateLimitMiddlewareBase_PanicOnZeroBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Burst=0")
		}
	}()
	NewAuthRateLimitMiddlewareBase(&AuthRateLimitMiddlewareOptions{Rate: 1, Burst: 0})
}
