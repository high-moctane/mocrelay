//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestMaxSubscriptionsMiddleware_AllowWithinLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxSubscriptionsMiddleware(3)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send 3 REQ messages (should all be allowed)
		for i := range 3 {
			recv <- &ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: string(rune('a' + i)), // "a", "b", "c"
				Filters:        []*ReqFilter{{}},
			}
		}

		synctest.Wait()

		// Should receive 3 EOSE messages from NopHandler
		for range 3 {
			select {
			case msg := <-send:
				if msg.Type != MsgTypeEOSE {
					t.Errorf("expected EOSE, got %v", msg.Type)
				}
			default:
				t.Error("expected message but got none")
			}
		}

		cancel()
		<-done
	})
}

func TestMaxSubscriptionsMiddleware_RejectOverLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxSubscriptionsMiddleware(2)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send 2 REQ messages (should be allowed)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub2",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Drain the 2 EOSE messages
		<-send
		<-send

		// Send 3rd REQ message (should be rejected)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub3",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive CLOSED message (not EOSE)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeClosed {
				t.Errorf("expected CLOSED, got %v", msg.Type)
			}
			if msg.SubscriptionID != "sub3" {
				t.Errorf("expected sub3, got %v", msg.SubscriptionID)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxSubscriptionsMiddleware_ReplaceAllowed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxSubscriptionsMiddleware(1)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with sub1
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()
		<-send // EOSE

		// Replace sub1 (same subscription ID, should be allowed)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive EOSE (replacement allowed)
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

func TestMaxSubscriptionsMiddleware_CloseFreesSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxSubscriptionsMiddleware(1)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with sub1
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()
		<-send // EOSE

		// Close sub1
		recv <- &ClientMsg{
			Type:           MsgTypeClose,
			SubscriptionID: "sub1",
		}

		synctest.Wait()

		// Now sub2 should be allowed
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub2",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive EOSE (slot freed by CLOSE)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeEOSE {
				t.Errorf("expected EOSE, got %v", msg.Type)
			}
			if msg.SubscriptionID != "sub2" {
				t.Errorf("expected sub2, got %v", msg.SubscriptionID)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}
