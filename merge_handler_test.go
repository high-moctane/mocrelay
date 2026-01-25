//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
)

// rejectingHandler is a handler that always rejects events.
type rejectingHandler struct{}

func (h *rejectingHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-recv:
			if !ok {
				return nil
			}
			switch msg.Type {
			case MsgTypeEvent:
				send <- NewServerOKMsg(msg.Event.ID, false, "blocked: test rejection")
			case MsgTypeReq:
				send <- NewServerEOSEMsg(msg.SubscriptionID)
			}
		}
	}
}

// TestMergeHandler_Event_BothOK tests that when both handlers accept an event,
// MergeHandler returns OK with accepted=true.
func TestMergeHandler_Event_BothOK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create two NopHandlers (both will return OK with accepted=true)
		handler1 := NewNopHandler()
		handler2 := NewNopHandler()

		mergeHandler := NewMergeHandler(handler1, handler2)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go mergeHandler.ServeNostr(ctx, send, recv)

		// Send an EVENT
		event := makeEvent("event-1", "pubkey01", 1, 100)
		recv <- &ClientMsg{
			Type:  MsgTypeEvent,
			Event: event,
		}

		synctest.Wait()

		// Should receive exactly one OK (merged from both handlers)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeOK, msg.Type)
			assert.Equal(t, "event-1", msg.EventID)
			assert.True(t, msg.Accepted)
		default:
			t.Fatal("expected OK message")
		}

		// Should not receive any more messages
		select {
		case msg := <-send:
			t.Fatalf("unexpected message: %v", msg)
		default:
			// Expected: no more messages
		}
	})
}

// TestMergeHandler_Event_OneRejects tests that when one handler rejects an event,
// MergeHandler returns OK with accepted=false.
func TestMergeHandler_Event_OneRejects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// One accepts, one rejects
		handler1 := NewNopHandler()
		handler2 := &rejectingHandler{}

		mergeHandler := NewMergeHandler(handler1, handler2)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go mergeHandler.ServeNostr(ctx, send, recv)

		// Send an EVENT
		event := makeEvent("event-1", "pubkey01", 1, 100)
		recv <- &ClientMsg{
			Type:  MsgTypeEvent,
			Event: event,
		}

		synctest.Wait()

		// Should receive OK with accepted=false (one handler rejected)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeOK, msg.Type)
			assert.Equal(t, "event-1", msg.EventID)
			assert.False(t, msg.Accepted)
			assert.Contains(t, msg.Message, "blocked")
		default:
			t.Fatal("expected OK message")
		}
	})
}

// TestMergeHandler_Req_EOSE tests that EOSE is sent after all handlers send EOSE.
func TestMergeHandler_Req_EOSE(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Both NopHandlers will send EOSE immediately
		handler1 := NewNopHandler()
		handler2 := NewNopHandler()

		mergeHandler := NewMergeHandler(handler1, handler2)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go mergeHandler.ServeNostr(ctx, send, recv)

		// Send a REQ
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive exactly one EOSE (merged from both handlers)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeEOSE, msg.Type)
			assert.Equal(t, "sub1", msg.SubscriptionID)
		default:
			t.Fatal("expected EOSE message")
		}

		// Should not receive any more messages
		select {
		case msg := <-send:
			t.Fatalf("unexpected message: %v", msg)
		default:
			// Expected
		}
	})
}

// TestMergeHandler_Req_EventsWithDedupe tests that duplicate events are removed.
func TestMergeHandler_Req_EventsWithDedupe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Two storages with overlapping events
		storage1 := NewInMemoryStorage()
		storage1.Store(ctx, makeEvent("event-1", "pubkey01", 1, 300))
		storage1.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200)) // duplicate

		storage2 := NewInMemoryStorage()
		storage2.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200)) // duplicate
		storage2.Store(ctx, makeEvent("event-3", "pubkey01", 1, 100))

		handler1 := NewStorageHandler(storage1)
		handler2 := NewStorageHandler(storage2)

		mergeHandler := NewMergeHandler(handler1, handler2)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go mergeHandler.ServeNostr(ctx, send, recv)

		// Send a REQ
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Collect all messages
		var events []*ServerMsg
		for {
			select {
			case msg := <-send:
				events = append(events, msg)
			default:
				goto done
			}
		}
	done:

		// Should have 3 EVENTs + 1 EOSE = 4 messages
		// event-2 should appear only once (deduplicated)
		if len(events) != 4 {
			t.Fatalf("expected 4 messages, got %d: %v", len(events), events)
		}

		// Last message should be EOSE
		assert.Equal(t, MsgTypeEOSE, events[len(events)-1].Type)

		// Check event IDs (should be unique)
		eventIDs := make(map[string]bool)
		for _, msg := range events[:len(events)-1] {
			assert.Equal(t, MsgTypeEvent, msg.Type)
			eventIDs[msg.Event.ID] = true
		}
		assert.True(t, eventIDs["event-1"])
		assert.True(t, eventIDs["event-2"])
		assert.True(t, eventIDs["event-3"])
	})
}
