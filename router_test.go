//go:build goexperiment.jsonv2

package mocrelay

import (
	"testing"
	"testing/synctest"
)

func TestRouter_RegisterUnregister(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh := make(chan *ServerMsg, 10)
		connID := router.Register(sendCh)

		if connID == "" {
			t.Fatal("expected non-empty connection ID")
		}

		// Should be able to subscribe
		router.Subscribe(connID, "sub1", []*ReqFilter{{Kinds: []int64{1}}})

		// Unregister
		router.Unregister(connID)

		// After unregister, subscribe should be a no-op (no panic)
		router.Subscribe(connID, "sub2", []*ReqFilter{{Kinds: []int64{1}}})
	})
}

func TestRouter_UniqueConnectionIDs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh1 := make(chan *ServerMsg, 10)
		sendCh2 := make(chan *ServerMsg, 10)

		connID1 := router.Register(sendCh1)
		connID2 := router.Register(sendCh2)

		if connID1 == connID2 {
			t.Fatalf("connection IDs should be unique: got %q and %q", connID1, connID2)
		}
	})
}

func TestRouter_Broadcast_MatchingSubscription(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh := make(chan *ServerMsg, 10)
		connID := router.Register(sendCh)

		// Subscribe to kind 1
		router.Subscribe(connID, "sub1", []*ReqFilter{{Kinds: []int64{1}}})

		// Broadcast a kind 1 event
		event := &Event{
			ID:     "abc123",
			Pubkey: "pubkey123",
			Kind:   1,
		}
		router.Broadcast(event)

		synctest.Wait()

		// Should receive the event
		select {
		case msg := <-sendCh:
			if msg.Type != MsgTypeEvent {
				t.Fatalf("expected EVENT message, got %v", msg.Type)
			}
			if msg.SubscriptionID != "sub1" {
				t.Fatalf("expected subscription ID 'sub1', got %q", msg.SubscriptionID)
			}
			if msg.Event.ID != event.ID {
				t.Fatalf("expected event ID %q, got %q", event.ID, msg.Event.ID)
			}
		default:
			t.Fatal("expected to receive event, but channel was empty")
		}
	})
}

func TestRouter_Broadcast_NonMatchingSubscription(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh := make(chan *ServerMsg, 10)
		connID := router.Register(sendCh)

		// Subscribe to kind 1
		router.Subscribe(connID, "sub1", []*ReqFilter{{Kinds: []int64{1}}})

		// Broadcast a kind 2 event (doesn't match)
		event := &Event{
			ID:     "abc123",
			Pubkey: "pubkey123",
			Kind:   2,
		}
		router.Broadcast(event)

		synctest.Wait()

		// Should NOT receive the event
		select {
		case msg := <-sendCh:
			t.Fatalf("expected no message, but got %v", msg)
		default:
			// OK - no message
		}
	})
}

func TestRouter_Broadcast_MultipleSubscriptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh := make(chan *ServerMsg, 10)
		connID := router.Register(sendCh)

		// Two subscriptions that both match kind 1
		router.Subscribe(connID, "sub1", []*ReqFilter{{Kinds: []int64{1}}})
		router.Subscribe(connID, "sub2", []*ReqFilter{{Kinds: []int64{1, 2}}})

		// Broadcast a kind 1 event
		event := &Event{
			ID:     "abc123",
			Pubkey: "pubkey123",
			Kind:   1,
		}
		router.Broadcast(event)

		synctest.Wait()

		// Should receive two messages (one per subscription)
		received := make(map[string]bool)
		for i := range 2 {
			select {
			case msg := <-sendCh:
				received[msg.SubscriptionID] = true
			default:
				t.Fatalf("expected 2 messages, got %d", i)
			}
		}

		if !received["sub1"] || !received["sub2"] {
			t.Fatalf("expected both sub1 and sub2 to receive, got %v", received)
		}
	})
}

func TestRouter_Broadcast_MultipleConnections(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh1 := make(chan *ServerMsg, 10)
		sendCh2 := make(chan *ServerMsg, 10)

		connID1 := router.Register(sendCh1)
		connID2 := router.Register(sendCh2)

		// Both subscribe to kind 1
		router.Subscribe(connID1, "sub1", []*ReqFilter{{Kinds: []int64{1}}})
		router.Subscribe(connID2, "sub1", []*ReqFilter{{Kinds: []int64{1}}})

		// Broadcast a kind 1 event
		event := &Event{
			ID:     "abc123",
			Pubkey: "pubkey123",
			Kind:   1,
		}
		router.Broadcast(event)

		synctest.Wait()

		// Both should receive the event
		for i, ch := range []chan *ServerMsg{sendCh1, sendCh2} {
			select {
			case msg := <-ch:
				if msg.Event.ID != event.ID {
					t.Fatalf("connection %d: expected event ID %q, got %q", i+1, event.ID, msg.Event.ID)
				}
			default:
				t.Fatalf("connection %d: expected to receive event", i+1)
			}
		}
	})
}

func TestRouter_Broadcast_BestEffort_DropWhenFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		// Channel with capacity 1
		sendCh := make(chan *ServerMsg, 1)
		connID := router.Register(sendCh)

		router.Subscribe(connID, "sub1", []*ReqFilter{{Kinds: []int64{1}}})

		// Broadcast 3 events - only 1 should fit
		for i := range 3 {
			event := &Event{
				ID:     "event" + uitoa(uint64(i)),
				Pubkey: "pubkey123",
				Kind:   1,
			}
			router.Broadcast(event)
		}

		synctest.Wait()

		// Should have exactly 1 message (others dropped)
		count := 0
		for {
			select {
			case <-sendCh:
				count++
			default:
				goto done
			}
		}
	done:
		if count != 1 {
			t.Fatalf("expected 1 message (others dropped), got %d", count)
		}
	})
}

func TestRouter_Unsubscribe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh := make(chan *ServerMsg, 10)
		connID := router.Register(sendCh)

		router.Subscribe(connID, "sub1", []*ReqFilter{{Kinds: []int64{1}}})

		// Unsubscribe
		router.Unsubscribe(connID, "sub1")

		// Broadcast a kind 1 event
		event := &Event{
			ID:     "abc123",
			Pubkey: "pubkey123",
			Kind:   1,
		}
		router.Broadcast(event)

		synctest.Wait()

		// Should NOT receive the event (unsubscribed)
		select {
		case msg := <-sendCh:
			t.Fatalf("expected no message after unsubscribe, but got %v", msg)
		default:
			// OK
		}
	})
}

func TestRouter_SubscriptionIDCollision_DifferentConnections(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router := NewRouter()

		sendCh1 := make(chan *ServerMsg, 10)
		sendCh2 := make(chan *ServerMsg, 10)

		connID1 := router.Register(sendCh1)
		connID2 := router.Register(sendCh2)

		// Both use same subscription ID "1" (common for simple clients)
		router.Subscribe(connID1, "1", []*ReqFilter{{Kinds: []int64{1}}})
		router.Subscribe(connID2, "1", []*ReqFilter{{Kinds: []int64{2}}})

		// Broadcast kind 1 event
		event1 := &Event{ID: "event1", Pubkey: "pubkey", Kind: 1}
		router.Broadcast(event1)

		// Broadcast kind 2 event
		event2 := &Event{ID: "event2", Pubkey: "pubkey", Kind: 2}
		router.Broadcast(event2)

		synctest.Wait()

		// Connection 1 should only receive kind 1
		select {
		case msg := <-sendCh1:
			if msg.Event.Kind != 1 {
				t.Fatalf("conn1: expected kind 1, got %d", msg.Event.Kind)
			}
		default:
			t.Fatal("conn1: expected to receive kind 1 event")
		}

		// Connection 2 should only receive kind 2
		select {
		case msg := <-sendCh2:
			if msg.Event.Kind != 2 {
				t.Fatalf("conn2: expected kind 2, got %d", msg.Event.Kind)
			}
		default:
			t.Fatal("conn2: expected to receive kind 2 event")
		}

		// No extra messages
		select {
		case msg := <-sendCh1:
			t.Fatalf("conn1: unexpected extra message: %v", msg)
		default:
		}
		select {
		case msg := <-sendCh2:
			t.Fatalf("conn2: unexpected extra message: %v", msg)
		default:
		}
	})
}
