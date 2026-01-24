//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestKindBlacklistMiddleware_AllowNonBlacklisted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Block DM-related kinds
		middleware := NewKindBlacklistMiddleware([]int64{4, 13, 14, 1059, 10050})
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 1 (text note, should be allowed)
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

func TestKindBlacklistMiddleware_RejectBlacklisted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Block DM-related kinds
		middleware := NewKindBlacklistMiddleware([]int64{4, 13, 14, 1059, 10050})
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 4 (legacy DM, should be blocked)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:   "test-event-id",
				Kind: 4,
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
				t.Error("expected Accepted=false for kind 4")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestKindBlacklistMiddleware_RejectGiftWrap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Block DM-related kinds
		middleware := NewKindBlacklistMiddleware([]int64{4, 13, 14, 1059, 10050})
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

func TestKindBlacklistMiddleware_EmptyBlacklist(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Empty blacklist - all kinds allowed
		middleware := NewKindBlacklistMiddleware([]int64{})
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with kind 4 (should be allowed with empty blacklist)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:   "test-event-id",
				Kind: 4,
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
				t.Error("expected Accepted=true with empty blacklist")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestKindBlacklistMiddleware_PassthroughNonEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Block all kinds (extreme case)
		middleware := NewKindBlacklistMiddleware([]int64{0, 1, 2, 3, 4, 5})
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
