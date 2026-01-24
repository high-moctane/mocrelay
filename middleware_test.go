//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

// testPassthroughMiddleware passes all messages through unchanged.
type testPassthroughMiddleware struct{}

func (m *testPassthroughMiddleware) OnStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *testPassthroughMiddleware) OnEnd(ctx context.Context) error {
	return nil
}

func (m *testPassthroughMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	return msg, nil, nil
}

func (m *testPassthroughMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}

// testBlockingMiddleware blocks all EVENT messages.
type testBlockingMiddleware struct{}

func (m *testBlockingMiddleware) OnStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *testBlockingMiddleware) OnEnd(ctx context.Context) error {
	return nil
}

func (m *testBlockingMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type == MsgTypeEvent {
		// Block EVENT messages and return OK false
		resp := NewServerOKMsg(msg.Event.ID, false, "blocked: test")
		return nil, resp, nil
	}
	return msg, nil, nil
}

func (m *testBlockingMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}

func TestSimpleMiddleware_Passthrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Setup: middleware -> NopHandler
		middleware := NewSimpleMiddleware(&testPassthroughMiddleware{})
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send a REQ message
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
			if msg.SubscriptionID != "sub1" {
				t.Errorf("expected sub1, got %v", msg.SubscriptionID)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestSimpleMiddleware_BlockEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Setup: blocking middleware -> NopHandler
		middleware := NewSimpleMiddleware(&testBlockingMiddleware{})
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send an EVENT message (should be blocked)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID: "test-event-id",
			},
		}

		synctest.Wait()

		// Should receive OK false from middleware (not from NopHandler)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected Accepted=false")
			}
			if msg.Message != "blocked: test" {
				t.Errorf("expected message 'blocked: test', got %v", msg.Message)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestSimpleMiddleware_DropServerMsg(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Setup: middleware that drops all EOSE messages
		dropEOSE := &testDropEOSEMiddleware{}
		middleware := NewSimpleMiddleware(dropEOSE)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send a REQ message
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// EOSE should be dropped, so send channel should be empty
		select {
		case msg := <-send:
			t.Errorf("expected no message, but got %v", msg.Type)
		default:
			// Expected: no message
		}

		cancel()
		<-done
	})
}

// testDropEOSEMiddleware drops all EOSE messages.
type testDropEOSEMiddleware struct{}

func (m *testDropEOSEMiddleware) OnStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *testDropEOSEMiddleware) OnEnd(ctx context.Context) error {
	return nil
}

func (m *testDropEOSEMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	return msg, nil, nil
}

func (m *testDropEOSEMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	if msg.Type == MsgTypeEOSE {
		return nil, nil // Drop EOSE
	}
	return msg, nil
}
