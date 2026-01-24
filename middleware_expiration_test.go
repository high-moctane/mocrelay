//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestExpirationMiddleware_RejectExpiredEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create middleware with fixed time
		fixedTime := time.Unix(1000000, 0)
		mw := &ExpirationMiddleware{now: func() time.Time { return fixedTime }}
		middleware := NewSimpleMiddleware(mw)

		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			// Should not receive any messages
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

		// Send expired event (expiration < now)
		expiredEvent := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123",
			CreatedAt: fixedTime.Add(-time.Hour),
			Kind:      1,
			Tags:      []Tag{{"expiration", "999999"}}, // expired (999999 < 1000000)
			Content:   "test",
			Sig:       "sig123",
		}
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: expiredEvent}
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
		if response.Message != "invalid: event has expired" {
			t.Errorf("unexpected message: %s", response.Message)
		}
	})
}

func TestExpirationMiddleware_AcceptNonExpiredEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixedTime := time.Unix(1000000, 0)
		mw := &ExpirationMiddleware{now: func() time.Time { return fixedTime }}
		middleware := NewSimpleMiddleware(mw)

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

		// Send non-expired event (expiration > now)
		validEvent := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123",
			CreatedAt: fixedTime,
			Kind:      1,
			Tags:      []Tag{{"expiration", "1000001"}}, // not expired (1000001 > 1000000)
			Content:   "test",
			Sig:       "sig123",
		}
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: validEvent}
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

func TestExpirationMiddleware_AcceptEventWithoutExpiration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixedTime := time.Unix(1000000, 0)
		mw := &ExpirationMiddleware{now: func() time.Time { return fixedTime }}
		middleware := NewSimpleMiddleware(mw)

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

		// Send event without expiration tag
		event := &Event{
			ID:        "abc123",
			Pubkey:    "pubkey123",
			CreatedAt: fixedTime,
			Kind:      1,
			Tags:      []Tag{},
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

func TestExpirationMiddleware_DropExpiredEventOnSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixedTime := time.Unix(1000000, 0)
		mw := &ExpirationMiddleware{now: func() time.Time { return fixedTime }}
		middleware := NewSimpleMiddleware(mw)

		messagesSent := make(chan struct{})
		downstream := HandlerFunc(func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
			// Send expired event to client
			expiredEvent := &Event{
				ID:        "expired123",
				Pubkey:    "pubkey123",
				CreatedAt: fixedTime.Add(-time.Hour),
				Kind:      1,
				Tags:      []Tag{{"expiration", "999999"}},
				Content:   "test",
				Sig:       "sig123",
			}
			send <- NewServerEventMsg("sub1", expiredEvent)

			// Send non-expired event
			validEvent := &Event{
				ID:        "valid123",
				Pubkey:    "pubkey123",
				CreatedAt: fixedTime,
				Kind:      1,
				Tags:      []Tag{{"expiration", "1000001"}},
				Content:   "test",
				Sig:       "sig123",
			}
			send <- NewServerEventMsg("sub1", validEvent)

			close(messagesSent)

			// Wait for context to be cancelled (keep handler alive)
			<-ctx.Done()
			return nil
		})

		handler := middleware(downstream)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go func() {
			_ = handler.ServeNostr(ctx, send, recv)
		}()

		// Wait for messages to be sent by downstream
		<-messagesSent
		synctest.Wait()

		// Now cancel to stop the handler
		cancel()
		synctest.Wait()

		// Should only receive the non-expired event
		var messages []*ServerMsg
		for {
			select {
			case msg, ok := <-send:
				if !ok {
					goto collectDone
				}
				messages = append(messages, msg)
			default:
				goto collectDone
			}
		}
	collectDone:

		// Filter out non-EVENT messages
		var eventMsgs []*ServerMsg
		for _, msg := range messages {
			if msg.Type == MsgTypeEvent {
				eventMsgs = append(eventMsgs, msg)
			}
		}

		if len(eventMsgs) != 1 {
			t.Errorf("expected 1 event message, got %d", len(eventMsgs))
		}

		if len(eventMsgs) > 0 && eventMsgs[0].Event.ID != "valid123" {
			t.Errorf("expected valid123, got %s", eventMsgs[0].Event.ID)
		}
	})
}

func TestExpirationMiddleware_PassThroughNonEventMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixedTime := time.Unix(1000000, 0)
		mw := &ExpirationMiddleware{now: func() time.Time { return fixedTime }}
		middleware := NewSimpleMiddleware(mw)

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

		// Send REQ message (should pass through)
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
