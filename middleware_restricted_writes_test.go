//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestRestrictedWritesMiddleware_AllowlistAllow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewRestrictedWritesMiddleware(
			RestrictedWritesModeAllowlist,
			[]string{"allowed-pubkey-1", "allowed-pubkey-2"},
		)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT from allowed pubkey
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "test-event-id",
				Pubkey: "allowed-pubkey-1",
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
				t.Error("expected Accepted=true for allowed pubkey")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestRestrictedWritesMiddleware_AllowlistReject(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewRestrictedWritesMiddleware(
			RestrictedWritesModeAllowlist,
			[]string{"allowed-pubkey-1", "allowed-pubkey-2"},
		)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT from non-allowed pubkey
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "test-event-id",
				Pubkey: "unknown-pubkey",
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
				t.Error("expected Accepted=false for non-allowed pubkey")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestRestrictedWritesMiddleware_BlocklistAllow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewRestrictedWritesMiddleware(
			RestrictedWritesModeBlocklist,
			[]string{"blocked-pubkey-1", "blocked-pubkey-2"},
		)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT from non-blocked pubkey
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "test-event-id",
				Pubkey: "good-pubkey",
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
				t.Error("expected Accepted=true for non-blocked pubkey")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestRestrictedWritesMiddleware_BlocklistReject(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewRestrictedWritesMiddleware(
			RestrictedWritesModeBlocklist,
			[]string{"blocked-pubkey-1", "blocked-pubkey-2"},
		)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT from blocked pubkey
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "test-event-id",
				Pubkey: "blocked-pubkey-1",
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
				t.Error("expected Accepted=false for blocked pubkey")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestRestrictedWritesMiddleware_EmptyAllowlist(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Empty allowlist = nobody allowed
		middleware := NewRestrictedWritesMiddleware(
			RestrictedWritesModeAllowlist,
			[]string{},
		)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "test-event-id",
				Pubkey: "any-pubkey",
			},
		}

		synctest.Wait()

		// Should receive OK false (nobody in allowlist)
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

func TestRestrictedWritesMiddleware_EmptyBlocklist(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Empty blocklist = everyone allowed
		middleware := NewRestrictedWritesMiddleware(
			RestrictedWritesModeBlocklist,
			[]string{},
		)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "test-event-id",
				Pubkey: "any-pubkey",
			},
		}

		synctest.Wait()

		// Should receive OK true (nobody in blocklist)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Error("expected Accepted=true with empty blocklist")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestRestrictedWritesMiddleware_PassthroughNonEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Empty allowlist (extreme restriction)
		middleware := NewRestrictedWritesMiddleware(
			RestrictedWritesModeAllowlist,
			[]string{},
		)
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
