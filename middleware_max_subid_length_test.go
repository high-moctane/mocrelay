//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestMaxSubidLengthMiddleware_AllowWithinLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxSubidLengthMiddleware(10)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with short subscription ID (should be allowed)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub123", // 6 chars, within limit
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

func TestMaxSubidLengthMiddleware_RejectTooLong(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxSubidLengthMiddleware(5)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with too long subscription ID
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "toolong", // 7 chars, exceeds limit of 5
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive CLOSED message (not EOSE)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeClosed {
				t.Errorf("expected CLOSED, got %v", msg.Type)
			}
			if msg.SubscriptionID != "toolong" {
				t.Errorf("expected subscription ID 'toolong', got %v", msg.SubscriptionID)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxSubidLengthMiddleware_ExactLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxSubidLengthMiddleware(5)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with exactly max length subscription ID (should be allowed)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "12345", // exactly 5 chars
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
