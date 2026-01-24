//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestProtectedEventsMiddleware_RejectWithoutAuth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewProtectedEventsMiddleware()

		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			for msg := range recv {
				t.Errorf("downstream received unexpected message: %v", msg)
			}
			return nil
		})

		handler := middleware(downstream)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go func() {
			_ = handler.ServeNostr(t.Context(), send, recv)
		}()

		// Send protected event without auth
		event := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123",
			CreatedAt: time.Now(),
			Kind:      1,
			Tags:      []Tag{{"-"}}, // protected tag
			Content:   "test",
			Sig:       "sig123",
		}
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		close(recv)

		synctest.Wait()

		// Check response
		var response *ServerMsg
		select {
		case response = <-send:
		default:
			t.Fatal("expected response")
		}

		if response.Type != MsgTypeOK {
			t.Errorf("expected OK message, got %s", response.Type)
		}
		if response.Accepted {
			t.Error("expected rejected response")
		}
		if response.Message != "auth-required: this event may only be published by its author" {
			t.Errorf("unexpected message: %s", response.Message)
		}
	})
}

func TestProtectedEventsMiddleware_AcceptWithMatchingAuth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewProtectedEventsMiddleware()

		received := make(chan *ClientMsg, 1)
		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			for msg := range recv {
				received <- msg
			}
			return nil
		})

		handler := middleware(downstream)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		// Create context with auth state
		state := &authState{
			challenge:     "test-challenge",
			authedPubkeys: map[string]struct{}{"pubkey123": {}},
		}
		ctx := context.WithValue(t.Context(), authCtxKey{}, state)

		go func() {
			_ = handler.ServeNostr(ctx, send, recv)
		}()

		// Send protected event with matching pubkey
		event := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123", // matches authed pubkey
			CreatedAt: time.Now(),
			Kind:      1,
			Tags:      []Tag{{"-"}},
			Content:   "test",
			Sig:       "sig123",
		}
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		close(recv)

		synctest.Wait()

		// Check that downstream received the event
		select {
		case msg := <-received:
			if msg.Event.ID != "abc123" {
				t.Errorf("unexpected event ID: %s", msg.Event.ID)
			}
		default:
			t.Fatal("downstream should have received the event")
		}
	})
}

func TestProtectedEventsMiddleware_RejectWithNonMatchingAuth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewProtectedEventsMiddleware()

		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			for msg := range recv {
				t.Errorf("downstream received unexpected message: %v", msg)
			}
			return nil
		})

		handler := middleware(downstream)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		// Create context with auth state for a DIFFERENT pubkey
		state := &authState{
			challenge:     "test-challenge",
			authedPubkeys: map[string]struct{}{"other-pubkey": {}},
		}
		ctx := context.WithValue(t.Context(), authCtxKey{}, state)

		go func() {
			_ = handler.ServeNostr(ctx, send, recv)
		}()

		// Send protected event with non-matching pubkey
		event := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123", // does NOT match authed pubkey
			CreatedAt: time.Now(),
			Kind:      1,
			Tags:      []Tag{{"-"}},
			Content:   "test",
			Sig:       "sig123",
		}
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		close(recv)

		synctest.Wait()

		// Check response
		var response *ServerMsg
		select {
		case response = <-send:
		default:
			t.Fatal("expected response")
		}

		if response.Type != MsgTypeOK {
			t.Errorf("expected OK message, got %s", response.Type)
		}
		if response.Accepted {
			t.Error("expected rejected response")
		}
	})
}

func TestProtectedEventsMiddleware_AcceptNonProtectedEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewProtectedEventsMiddleware()

		received := make(chan *ClientMsg, 1)
		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			for msg := range recv {
				received <- msg
			}
			return nil
		})

		handler := middleware(downstream)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		// No auth state in context
		go func() {
			_ = handler.ServeNostr(t.Context(), send, recv)
		}()

		// Send non-protected event (no ["-"] tag)
		event := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123",
			CreatedAt: time.Now(),
			Kind:      1,
			Tags:      []Tag{{"t", "test"}}, // no protected tag
			Content:   "test",
			Sig:       "sig123",
		}
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		close(recv)

		synctest.Wait()

		// Check that downstream received the event
		select {
		case msg := <-received:
			if msg.Event.ID != "abc123" {
				t.Errorf("unexpected event ID: %s", msg.Event.ID)
			}
		default:
			t.Fatal("downstream should have received the event")
		}
	})
}

func TestProtectedEventsMiddleware_PassThroughNonEventMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewProtectedEventsMiddleware()

		received := make(chan *ClientMsg, 1)
		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			for msg := range recv {
				received <- msg
			}
			return nil
		})

		handler := middleware(downstream)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go func() {
			_ = handler.ServeNostr(t.Context(), send, recv)
		}()

		// Send REQ message (should pass through regardless of auth)
		recv <- &ClientMsg{Type: MsgTypeReq, SubscriptionID: "sub1", Filters: []*ReqFilter{{}}}
		close(recv)

		synctest.Wait()

		select {
		case msg := <-received:
			if msg.Type != MsgTypeReq {
				t.Errorf("expected REQ, got %s", msg.Type)
			}
		default:
			t.Fatal("downstream should have received the message")
		}
	})
}

func TestProtectedEventsMiddleware_ProtectedTagMustBeExactlyOneDash(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewProtectedEventsMiddleware()

		received := make(chan *ClientMsg, 1)
		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			for msg := range recv {
				received <- msg
			}
			return nil
		})

		handler := middleware(downstream)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		// No auth state in context
		go func() {
			_ = handler.ServeNostr(t.Context(), send, recv)
		}()

		// Send event with ["-", "extra"] tag (should NOT be treated as protected)
		event := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123",
			CreatedAt: time.Now(),
			Kind:      1,
			Tags:      []Tag{{"-", "extra"}}, // has extra element, not protected
			Content:   "test",
			Sig:       "sig123",
		}
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		close(recv)

		synctest.Wait()

		// Check that downstream received the event (not rejected)
		select {
		case msg := <-received:
			if msg.Event.ID != "abc123" {
				t.Errorf("unexpected event ID: %s", msg.Event.ID)
			}
		default:
			t.Fatal("downstream should have received the event")
		}
	})
}
