//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestMaxLimitMiddleware_ClampToMax(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxLimitMiddleware(100, 50)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with limit exceeding max
		limit := int64(500)
		msg := &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &limit}},
		}
		recv <- msg

		synctest.Wait()

		// Limit should be clamped to 100
		if *msg.Filters[0].Limit != 100 {
			t.Errorf("expected limit to be clamped to 100, got %d", *msg.Filters[0].Limit)
		}

		// Should receive EOSE
		select {
		case m := <-send:
			if m.Type != MsgTypeEOSE {
				t.Errorf("expected EOSE, got %v", m.Type)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxLimitMiddleware_ApplyDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxLimitMiddleware(100, 50)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ without limit (should apply default)
		msg := &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}}, // no limit
		}
		recv <- msg

		synctest.Wait()

		// Limit should be set to default 50
		if msg.Filters[0].Limit == nil || *msg.Filters[0].Limit != 50 {
			t.Errorf("expected default limit 50, got %v", msg.Filters[0].Limit)
		}

		// Should receive EOSE
		select {
		case m := <-send:
			if m.Type != MsgTypeEOSE {
				t.Errorf("expected EOSE, got %v", m.Type)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxLimitMiddleware_ApplyDefaultForZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxLimitMiddleware(100, 50)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with limit=0 (should apply default)
		zero := int64(0)
		msg := &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &zero}},
		}
		recv <- msg

		synctest.Wait()

		// Limit should be set to default 50
		if *msg.Filters[0].Limit != 50 {
			t.Errorf("expected default limit 50, got %d", *msg.Filters[0].Limit)
		}

		cancel()
		<-done
	})
}

func TestMaxLimitMiddleware_AllowWithinLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxLimitMiddleware(100, 50)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send REQ with limit within max (should not be modified)
		limit := int64(75)
		msg := &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &limit}},
		}
		recv <- msg

		synctest.Wait()

		// Limit should remain 75
		if *msg.Filters[0].Limit != 75 {
			t.Errorf("expected limit to remain 75, got %d", *msg.Filters[0].Limit)
		}

		cancel()
		<-done
	})
}
