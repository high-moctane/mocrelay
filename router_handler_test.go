//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestRouterHandler_Event_ReturnsOK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()
		handler := NewRouterHandler(router)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		// Send EVENT
		recv <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "abc123",
				Pubkey: "pubkey123",
				Kind:   1,
			},
		}

		synctest.Wait()

		// Should receive OK
		select {
		case msg := <-send:
			if msg.Type != MsgTypeOK {
				t.Fatalf("expected OK, got %v", msg.Type)
			}
			if !msg.Accepted {
				t.Fatal("expected Accepted=true")
			}
			if msg.EventID != "abc123" {
				t.Fatalf("expected event ID 'abc123', got %q", msg.EventID)
			}
		default:
			t.Fatal("expected OK message")
		}
	})
}

func TestRouterHandler_Req_ReturnsEOSE(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()
		handler := NewRouterHandler(router)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Kinds: []int64{1}}},
		}

		synctest.Wait()

		// Should receive EOSE
		select {
		case msg := <-send:
			if msg.Type != MsgTypeEOSE {
				t.Fatalf("expected EOSE, got %v", msg.Type)
			}
			if msg.SubscriptionID != "sub1" {
				t.Fatalf("expected subscription ID 'sub1', got %q", msg.SubscriptionID)
			}
		default:
			t.Fatal("expected EOSE message")
		}
	})
}

func TestRouterHandler_Count_ReturnsZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()
		handler := NewRouterHandler(router)

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		// Send COUNT
		recv <- &ClientMsg{
			Type:           MsgTypeCount,
			SubscriptionID: "count1",
			Filters:        []*ReqFilter{{Kinds: []int64{1}}},
		}

		synctest.Wait()

		// Should receive COUNT with 0
		select {
		case msg := <-send:
			if msg.Type != MsgTypeCount {
				t.Fatalf("expected COUNT, got %v", msg.Type)
			}
			if msg.Count != 0 {
				t.Fatalf("expected count 0, got %d", msg.Count)
			}
		default:
			t.Fatal("expected COUNT message")
		}
	})
}

func TestRouterHandler_EventRouting_BetweenClients(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		// Client A (sender)
		handlerA := NewRouterHandler(router)
		sendA := make(chan *ServerMsg, 10)
		recvA := make(chan *ClientMsg, 10)

		// Client B (subscriber)
		handlerB := NewRouterHandler(router)
		sendB := make(chan *ServerMsg, 10)
		recvB := make(chan *ClientMsg, 10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go handlerA.ServeNostr(ctx, sendA, recvA)
		go handlerB.ServeNostr(ctx, sendB, recvB)

		synctest.Wait()

		// Client B subscribes to kind 1
		recvB <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Kinds: []int64{1}}},
		}

		synctest.Wait()

		// Drain EOSE from B
		<-sendB

		// Client A sends an event
		recvA <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "event123",
				Pubkey: "pubkeyA",
				Kind:   1,
			},
		}

		synctest.Wait()

		// Drain OK from A
		<-sendA

		// Client B should receive the event
		select {
		case msg := <-sendB:
			if msg.Type != MsgTypeEvent {
				t.Fatalf("expected EVENT, got %v", msg.Type)
			}
			if msg.Event.ID != "event123" {
				t.Fatalf("expected event ID 'event123', got %q", msg.Event.ID)
			}
			if msg.SubscriptionID != "sub1" {
				t.Fatalf("expected subscription ID 'sub1', got %q", msg.SubscriptionID)
			}
		default:
			t.Fatal("client B should receive the event from client A")
		}
	})
}

func TestRouterHandler_Close_Unsubscribes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		handlerA := NewRouterHandler(router)
		sendA := make(chan *ServerMsg, 10)
		recvA := make(chan *ClientMsg, 10)

		handlerB := NewRouterHandler(router)
		sendB := make(chan *ServerMsg, 10)
		recvB := make(chan *ClientMsg, 10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go handlerA.ServeNostr(ctx, sendA, recvA)
		go handlerB.ServeNostr(ctx, sendB, recvB)

		synctest.Wait()

		// Client B subscribes
		recvB <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Kinds: []int64{1}}},
		}

		synctest.Wait()
		<-sendB // drain EOSE

		// Client B unsubscribes
		recvB <- &ClientMsg{
			Type:           MsgTypeClose,
			SubscriptionID: "sub1",
		}

		synctest.Wait()

		// Client A sends an event
		recvA <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "event123",
				Pubkey: "pubkeyA",
				Kind:   1,
			},
		}

		synctest.Wait()
		<-sendA // drain OK

		// Client B should NOT receive the event (unsubscribed)
		select {
		case msg := <-sendB:
			t.Fatalf("client B should not receive after CLOSE, but got %v", msg)
		default:
			// OK - no message
		}
	})
}

func TestRouterHandler_Disconnect_CleansUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		handlerA := NewRouterHandler(router)
		sendA := make(chan *ServerMsg, 10)
		recvA := make(chan *ClientMsg, 10)

		handlerB := NewRouterHandler(router)
		sendB := make(chan *ServerMsg, 10)
		recvB := make(chan *ClientMsg, 10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go handlerA.ServeNostr(ctx, sendA, recvA)

		doneB := make(chan struct{})
		go func() {
			handlerB.ServeNostr(ctx, sendB, recvB)
			close(doneB)
		}()

		synctest.Wait()

		// Client B subscribes
		recvB <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Kinds: []int64{1}}},
		}

		synctest.Wait()
		<-sendB // drain EOSE

		// Client B disconnects (close recv channel)
		close(recvB)

		synctest.Wait()

		// Wait for handler B to finish
		<-doneB

		// Client A sends an event
		recvA <- &ClientMsg{
			Type: MsgTypeEvent,
			Event: &Event{
				ID:     "event123",
				Pubkey: "pubkeyA",
				Kind:   1,
			},
		}

		synctest.Wait()
		<-sendA // drain OK

		// No panic, no deadlock - router should have cleaned up B's subscription
		// sendB channel should not receive anything (B is disconnected)
		select {
		case msg := <-sendB:
			t.Fatalf("disconnected client should not receive, but got %v", msg)
		default:
			// OK
		}
	})
}
