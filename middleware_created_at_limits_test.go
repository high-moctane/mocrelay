//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestCreatedAtLimitsMiddleware_AllowWithinRange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewCreatedAtLimitsMiddleware(3600, 60) // 1 hour past, 1 minute future
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with current timestamp (should be allowed)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:        "test-event-id",
				CreatedAt: time.Now(),
			},
		}

		synctest.Wait()

		// Should receive OK true
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Error("expected Accepted=true")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestCreatedAtLimitsMiddleware_RejectTooOld(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := &CreatedAtLimitsMiddleware{
			lowerLimit: 3600, // 1 hour
			upperLimit: 60,   // 1 minute
			now: func() time.Time {
				return time.Unix(1000000, 0)
			},
		}
		middleware := NewSimpleMiddleware(base)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with timestamp too old (2 hours ago)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:        "test-event-id",
				CreatedAt: time.Unix(1000000-7200, 0), // 2 hours before "now"
			},
		}

		synctest.Wait()

		// Should receive OK false
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected Accepted=false")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestCreatedAtLimitsMiddleware_RejectTooFuture(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := &CreatedAtLimitsMiddleware{
			lowerLimit: 3600, // 1 hour
			upperLimit: 60,   // 1 minute
			now: func() time.Time {
				return time.Unix(1000000, 0)
			},
		}
		middleware := NewSimpleMiddleware(base)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with timestamp too far in the future (2 minutes ahead)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:        "test-event-id",
				CreatedAt: time.Unix(1000000+120, 0), // 2 minutes after "now"
			},
		}

		synctest.Wait()

		// Should receive OK false
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected Accepted=false")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestCreatedAtLimitsMiddleware_AllowAtBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := &CreatedAtLimitsMiddleware{
			lowerLimit: 3600, // 1 hour
			upperLimit: 60,   // 1 minute
			now: func() time.Time {
				return time.Unix(1000000, 0)
			},
		}
		middleware := NewSimpleMiddleware(base)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with timestamp exactly at lower boundary
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:        "test-event-id",
				CreatedAt: time.Unix(1000000-3600, 0), // exactly 1 hour ago
			},
		}

		synctest.Wait()

		// Should receive OK true (boundary is inclusive)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Error("expected Accepted=true at lower boundary")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestCreatedAtLimitsMiddleware_PassthroughNonEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewCreatedAtLimitsMiddleware(1, 1) // very strict limits
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ (not EVENT, should pass through)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive EOSE from NopHandler
		select {
		case msg := <-send:
			if msg.Type != MsgTypeEOSE {
				t.Errorf("expected EOSE, got %v", msg.Type)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}
