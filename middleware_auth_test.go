//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestAuthMiddleware_ChallengeOnStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		// Should receive AUTH challenge on start
		select {
		case msg := <-send:
			if msg.Type != MsgTypeAuth {
				t.Errorf("expected AUTH message, got %v", msg.Type)
			}
			if msg.Message == "" {
				t.Error("expected non-empty challenge")
			}
			// Challenge should be 32 hex chars (16 bytes)
			if len(msg.Message) != 32 {
				t.Errorf("expected 32-char challenge, got %d chars", len(msg.Message))
			}
		default:
			t.Error("expected AUTH challenge, got nothing")
		}

		cancel()
	})
}

func TestAuthMiddleware_UnauthenticatedEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		// Drain AUTH challenge
		<-send

		// Send EVENT without authentication
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:        "test-event-id",
				Pubkey:    "test-pubkey",
				CreatedAt: time.Now(),
				Kind:      1,
				Tags:      []Tag{},
				Content:   "test",
				Sig:       "test-sig",
			},
		}

		synctest.Wait()

		// Should get OK false with auth-required
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK message, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected accepted=false")
			}
			if !strings.HasPrefix(msg.Message, "auth-required:") {
				t.Errorf("expected auth-required prefix, got %q", msg.Message)
			}
		default:
			t.Error("expected OK message, got nothing")
		}

		cancel()
	})
}

func TestAuthMiddleware_UnauthenticatedReq(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		// Drain AUTH challenge
		<-send

		// Send REQ without authentication
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should get CLOSED with auth-required
		select {
		case msg := <-send:
			if msg.Type != MsgTypeClosed {
				t.Errorf("expected CLOSED message, got %v", msg.Type)
			}
			if msg.SubscriptionID != "sub1" {
				t.Errorf("expected sub1, got %s", msg.SubscriptionID)
			}
			if !strings.HasPrefix(msg.Message, "auth-required:") {
				t.Errorf("expected auth-required prefix, got %q", msg.Message)
			}
		default:
			t.Error("expected CLOSED message, got nothing")
		}

		cancel()
	})
}

func TestAuthMiddleware_SuccessfulAuth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		// Get AUTH challenge
		authMsg := <-send
		challenge := authMsg.Message

		// Send valid AUTH event
		authEvent := &Event{
			ID:        "auth-event-id",
			Pubkey:    "authenticated-pubkey",
			CreatedAt: time.Now(),
			Kind:      22242,
			Tags: []Tag{
				{"relay", "wss://relay.example.com/"},
				{"challenge", challenge},
			},
			Content: "",
			Sig:     "valid-sig",
		}

		recv <- &ClientMsg{
			Type:  MsgTypeAuth,
			Event: authEvent,
		}

		synctest.Wait()

		// Should get OK true
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK message, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Errorf("expected accepted=true, got false with message: %s", msg.Message)
			}
		default:
			t.Error("expected OK message, got nothing")
		}

		// Now send EVENT - should succeed
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:        "real-event-id",
				Pubkey:    "authenticated-pubkey",
				CreatedAt: time.Now(),
				Kind:      1,
				Tags:      []Tag{},
				Content:   "test",
				Sig:       "test-sig",
			},
		}

		synctest.Wait()

		// Should get OK true from NopHandler
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK message, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Errorf("expected accepted=true, got false with message: %s", msg.Message)
			}
		default:
			t.Error("expected OK message, got nothing")
		}

		cancel()
	})
}

func TestAuthMiddleware_InvalidKind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		// Get and use challenge
		authMsg := <-send
		challenge := authMsg.Message

		// Send AUTH with wrong kind
		recv <- &ClientMsg{
			Type: MsgTypeAuth,
			Event: &Event{
				ID:        "auth-event-id",
				Pubkey:    "test-pubkey",
				CreatedAt: time.Now(),
				Kind:      1, // Wrong kind!
				Tags: []Tag{
					{"relay", "wss://relay.example.com/"},
					{"challenge", challenge},
				},
				Content: "",
				Sig:     "valid-sig",
			},
		}

		synctest.Wait()

		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK message, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected accepted=false")
			}
			if !strings.Contains(msg.Message, "kind 22242") {
				t.Errorf("expected kind error, got %q", msg.Message)
			}
		default:
			t.Error("expected OK message, got nothing")
		}

		cancel()
	})
}

func TestAuthMiddleware_InvalidChallenge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		// Drain AUTH challenge (don't use it)
		<-send

		// Send AUTH with wrong challenge
		recv <- &ClientMsg{
			Type: MsgTypeAuth,
			Event: &Event{
				ID:        "auth-event-id",
				Pubkey:    "test-pubkey",
				CreatedAt: time.Now(),
				Kind:      22242,
				Tags: []Tag{
					{"relay", "wss://relay.example.com/"},
					{"challenge", "wrong-challenge"},
				},
				Content: "",
				Sig:     "valid-sig",
			},
		}

		synctest.Wait()

		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK message, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected accepted=false")
			}
			if !strings.Contains(msg.Message, "challenge") {
				t.Errorf("expected challenge error, got %q", msg.Message)
			}
		default:
			t.Error("expected OK message, got nothing")
		}

		cancel()
	})
}

func TestAuthMiddleware_ExpiredCreatedAt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		authMsg := <-send
		challenge := authMsg.Message

		// Send AUTH with old created_at
		recv <- &ClientMsg{
			Type: MsgTypeAuth,
			Event: &Event{
				ID:        "auth-event-id",
				Pubkey:    "test-pubkey",
				CreatedAt: time.Now().Add(-20 * time.Minute), // Too old
				Kind:      22242,
				Tags: []Tag{
					{"relay", "wss://relay.example.com/"},
					{"challenge", challenge},
				},
				Content: "",
				Sig:     "valid-sig",
			},
		}

		synctest.Wait()

		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK message, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected accepted=false")
			}
			if !strings.Contains(msg.Message, "created_at") {
				t.Errorf("expected created_at error, got %q", msg.Message)
			}
		default:
			t.Error("expected OK message, got nothing")
		}

		cancel()
	})
}

func TestAuthMiddleware_ClosePassesThrough(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewAuthMiddleware("wss://relay.example.com/")
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		synctest.Wait()

		// Drain AUTH challenge
		<-send

		// Send CLOSE without authentication (should pass through)
		recv <- &ClientMsg{
			Type:           MsgTypeClose,
			SubscriptionID: "sub1",
		}

		synctest.Wait()

		// CLOSE should pass through (NopHandler ignores it, no response expected)
		// This test just verifies no panic or error occurs

		cancel()
	})
}

func TestFindTagValue(t *testing.T) {
	tests := []struct {
		name string
		tags []Tag
		key  string
		want string
	}{
		{
			name: "found",
			tags: []Tag{{"challenge", "abc123"}, {"relay", "wss://example.com"}},
			key:  "challenge",
			want: "abc123",
		},
		{
			name: "not found",
			tags: []Tag{{"challenge", "abc123"}},
			key:  "relay",
			want: "",
		},
		{
			name: "empty tags",
			tags: []Tag{},
			key:  "challenge",
			want: "",
		},
		{
			name: "tag with only key",
			tags: []Tag{{"challenge"}},
			key:  "challenge",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findTagValue(tt.tags, tt.key)
			if got != tt.want {
				t.Errorf("findTagValue() = %q, want %q", got, tt.want)
			}
		})
	}
}
