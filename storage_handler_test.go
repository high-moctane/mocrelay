//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageHandler_Event_Store(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send an EVENT
		event := makeEvent("event-1", "pubkey01", 1, 100)
		recv <- &ClientMsg{
			Type:  MsgTypeEvent,
			Event: event,
		}

		synctest.Wait()

		// Should receive OK
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeOK, msg.Type)
			assert.Equal(t, "event-1", msg.EventID)
			assert.True(t, msg.Accepted)
		default:
			t.Fatal("expected OK message")
		}

		// Event should be stored
		assert.Equal(t, 1, storage.Len())
	})
}

func TestStorageHandler_Event_Duplicate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send same event twice
		event := makeEvent("event-1", "pubkey01", 1, 100)
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		synctest.Wait()
		<-send // First OK

		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		synctest.Wait()

		// Second should also be OK (duplicate is not an error)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeOK, msg.Type)
			assert.True(t, msg.Accepted)
			assert.Contains(t, msg.Message, "duplicate")
		default:
			t.Fatal("expected OK message")
		}

		// Still only 1 event
		assert.Equal(t, 1, storage.Len())
	})
}

func TestStorageHandler_Req_Empty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ with empty storage
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive EOSE only (no events)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeEOSE, msg.Type)
			assert.Equal(t, "sub1", msg.SubscriptionID)
		default:
			t.Fatal("expected EOSE message")
		}
	})
}

func TestStorageHandler_Req_WithEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive 3 EVENTs + EOSE
		var events []*ServerMsg
		for i := range 4 {
			select {
			case msg := <-send:
				events = append(events, msg)
			default:
				t.Fatalf("expected message %d", i)
			}
		}

		// First 3 should be EVENT, last should be EOSE
		require.Len(t, events, 4)
		assert.Equal(t, MsgTypeEvent, events[0].Type)
		assert.Equal(t, MsgTypeEvent, events[1].Type)
		assert.Equal(t, MsgTypeEvent, events[2].Type)
		assert.Equal(t, MsgTypeEOSE, events[3].Type)

		// Events should be sorted by created_at DESC
		assert.Equal(t, "event-3", events[0].Event.ID)
		assert.Equal(t, "event-2", events[1].Event.ID)
		assert.Equal(t, "event-1", events[2].Event.ID)
	})
}

func TestStorageHandler_Req_WithLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ with limit
		limit := int64(2)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &limit}},
		}

		synctest.Wait()

		// Should receive 2 EVENTs + EOSE
		var events []*ServerMsg
		for i := range 3 {
			select {
			case msg := <-send:
				events = append(events, msg)
			default:
				t.Fatalf("expected message %d", i)
			}
		}

		require.Len(t, events, 3)
		assert.Equal(t, MsgTypeEvent, events[0].Type)
		assert.Equal(t, MsgTypeEvent, events[1].Type)
		assert.Equal(t, MsgTypeEOSE, events[2].Type)

		// Should be the 2 newest
		assert.Equal(t, "event-3", events[0].Event.ID)
		assert.Equal(t, "event-2", events[1].Event.ID)
	})
}

func TestStorageHandler_Close_NoOp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send CLOSE - should be a no-op
		recv <- &ClientMsg{
			Type:           MsgTypeClose,
			SubscriptionID: "sub1",
		}

		synctest.Wait()

		// Should not receive any message
		select {
		case msg := <-send:
			t.Fatalf("unexpected message: %v", msg)
		default:
			// Expected: no message
		}
	})
}

func TestStorageHandler_Count(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send COUNT
		recv <- &ClientMsg{
			Type:           MsgTypeCount,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive COUNT with 3
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeCount, msg.Type)
			assert.Equal(t, "sub1", msg.SubscriptionID)
			assert.Equal(t, uint64(3), msg.Count)
		default:
			t.Fatal("expected COUNT message")
		}
	})
}

func TestStorageHandler_Count_WithFilter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage with different kinds
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 2, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send COUNT with kind filter
		recv <- &ClientMsg{
			Type:           MsgTypeCount,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Kinds: []int64{1}}},
		}

		synctest.Wait()

		// Should receive COUNT with 2 (only kind 1)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeCount, msg.Type)
			assert.Equal(t, uint64(2), msg.Count)
		default:
			t.Fatal("expected COUNT message")
		}
	})
}
