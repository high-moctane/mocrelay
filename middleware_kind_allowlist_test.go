package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestKindAllowlistMiddleware_AllowAllowlisted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Allow only kind 1 (text note)
		middleware := NewSimpleMiddleware(NewKindAllowlistMiddlewareBase([]int64{1}))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 1 (allowed)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:   "test-event-id",
				Kind: 1,
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
				t.Error("expected Accepted=true for kind 1")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestKindAllowlistMiddleware_RejectNonAllowlisted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Allow only kinds 0, 1, 3 (well-known basics)
		middleware := NewSimpleMiddleware(NewKindAllowlistMiddlewareBase([]int64{0, 1, 3}))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 999 (not in allowlist)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:   "test-event-id",
				Kind: 999,
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
				t.Error("expected Accepted=false for kind 999")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestKindAllowlistMiddleware_RejectDM(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Allow only kinds 0, 1, 3 (DM kinds excluded)
		middleware := NewSimpleMiddleware(NewKindAllowlistMiddlewareBase([]int64{0, 1, 3}))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 1059 (Gift Wrap, should be blocked)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:   "test-event-id",
				Kind: 1059,
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
				t.Error("expected Accepted=false for kind 1059 (Gift Wrap)")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestKindAllowlistMiddleware_EmptyAllowlist(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Empty allowlist - all kinds rejected
		middleware := NewSimpleMiddleware(NewKindAllowlistMiddlewareBase([]int64{}))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 1 (should be rejected with empty allowlist)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:   "test-event-id",
				Kind: 1,
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
				t.Error("expected Accepted=false with empty allowlist")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestKindAllowlistMiddleware_PassthroughNonEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Empty allowlist (would block all events)
		middleware := NewSimpleMiddleware(NewKindAllowlistMiddlewareBase([]int64{}))
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

func TestKindAllowlistMiddleware_RangeAllowed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Allow Job Request range (5000-5999, expanded)
		var kinds []int64
		for k := int64(5000); k <= 5999; k++ {
			kinds = append(kinds, k)
		}
		middleware := NewSimpleMiddleware(NewKindAllowlistMiddlewareBase(kinds))
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 5500 (in range)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:   "test-event-id",
				Kind: 5500,
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
				t.Error("expected Accepted=true for kind 5500 in range")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}
