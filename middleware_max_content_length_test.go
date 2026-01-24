//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestMaxContentLengthMiddleware_AllowWithinLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxContentLengthMiddleware(10)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with 5 characters (within limit of 10)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:      "test-event-id",
				Content: "hello",
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

func TestMaxContentLengthMiddleware_RejectTooLong(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxContentLengthMiddleware(5)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with 11 characters (exceeds limit of 5)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:      "test-event-id",
				Content: "hello world",
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
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxContentLengthMiddleware_UnicodeCharacters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxContentLengthMiddleware(5)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with 5 Japanese characters (should be allowed)
		// Note: "こんにちは" is 5 characters but 15 bytes in UTF-8
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:      "test-event-id",
				Content: "こんにちは", // 5 Unicode characters
			},
		}

		synctest.Wait()

		// Should receive OK true (5 characters <= limit of 5)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Error("expected Accepted=true for 5 Unicode characters")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxContentLengthMiddleware_UnicodeExceedsLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxContentLengthMiddleware(4)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with 5 Japanese characters (exceeds limit of 4)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:      "test-event-id",
				Content: "こんにちは", // 5 Unicode characters
			},
		}

		synctest.Wait()

		// Should receive OK false (5 characters > limit of 4)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if msg.Accepted {
				t.Error("expected Accepted=false for 5 Unicode characters exceeding limit of 4")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}

func TestMaxContentLengthMiddleware_Emoji(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		middleware := NewMaxContentLengthMiddleware(3)
		handler := middleware(NewNopHandler())

		recv := make(chan *ClientMsg)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// Send EVENT with 3 emoji (each is 1 Unicode code point, but may be multiple bytes)
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:      "test-event-id",
				Content: "🌸💕✨", // 3 Unicode characters
			},
		}

		synctest.Wait()

		// Should receive OK true (3 characters <= limit of 3)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Errorf("expected OK, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Error("expected Accepted=true for 3 emoji")
			}
		default:
			t.Error("expected message but got none")
		}

		cancel()
		<-done
	})
}
