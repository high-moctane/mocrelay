//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestMaxEventTagsMiddleware_AllowWithinLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxEventTagsMiddleware(5)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with 3 tags (within limit of 5)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID: "test-event-id",
				Tags: []Tag{
					{"e", "eventid1"},
					{"p", "pubkey1"},
					{"t", "tag1"},
				},
			},
		}

		synctest.Wait()

		// Should receive OK true from NopHandler
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

func TestMaxEventTagsMiddleware_RejectTooMany(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxEventTagsMiddleware(2)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with 3 tags (exceeds limit of 2)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID: "test-event-id",
				Tags: []Tag{
					{"e", "eventid1"},
					{"p", "pubkey1"},
					{"t", "tag1"},
				},
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
			if msg.EventID != "test-event-id" {
				t.Errorf("expected event ID 'test-event-id', got %v", msg.EventID)
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxEventTagsMiddleware_ExactLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxEventTagsMiddleware(2)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with exactly 2 tags (at limit)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID: "test-event-id",
				Tags: []Tag{
					{"e", "eventid1"},
					{"p", "pubkey1"},
				},
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

func TestMaxEventTagsMiddleware_PassthroughNonEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxEventTagsMiddleware(1)
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

		// Should receive EOSE from NopHandler (REQ passed through)
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
