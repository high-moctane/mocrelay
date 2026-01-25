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
// All events have the same created_at to avoid sort drop interference.
func TestMergeHandler_Req_EventsWithDedupe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Two storages with overlapping events (same created_at, different IDs)
		// IDs are chosen so they sort correctly: aaa < bbb < ccc (lexical order)
		storage1 := NewInMemoryStorage()
		storage1.Store(ctx, makeEvent("aaa", "pubkey01", 1, 100))
		storage1.Store(ctx, makeEvent("bbb", "pubkey01", 1, 100)) // duplicate

		storage2 := NewInMemoryStorage()
		storage2.Store(ctx, makeEvent("bbb", "pubkey01", 1, 100)) // duplicate
		storage2.Store(ctx, makeEvent("ccc", "pubkey01", 1, 100))

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

		// Last message should be EOSE
		assert.Equal(t, MsgTypeEOSE, events[len(events)-1].Type)

		// Check event IDs (should be unique, no duplicates)
		eventIDs := make(map[string]int)
		for _, msg := range events[:len(events)-1] {
			if msg.Type == MsgTypeEvent {
				eventIDs[msg.Event.ID]++
			}
		}

		// Each event should appear exactly once
		for id, count := range eventIDs {
			assert.Equal(t, 1, count, "event %s should appear exactly once", id)
		}

		// bbb should be present (deduplicated, not dropped)
		assert.Equal(t, 1, eventIDs["bbb"], "bbb should be deduplicated to 1")
	})
}

// TestMergeHandler_Req_SortDrop tests that events breaking sort order are dropped.
func TestMergeHandler_Req_SortDrop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Storage 1: newer events (will arrive first due to faster response)
		storage1 := NewInMemoryStorage()
		storage1.Store(ctx, makeEvent("event-a", "pubkey01", 1, 300)) // newest

		// Storage 2: older events
		storage2 := NewInMemoryStorage()
		storage2.Store(ctx, makeEvent("event-b", "pubkey01", 1, 400)) // even newer! should be dropped
		storage2.Store(ctx, makeEvent("event-c", "pubkey01", 1, 100)) // oldest, OK

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

		// event-b (created_at=400) should be dropped because it breaks sort order
		// Expected: event-a (300), event-c (100), EOSE = 3 messages
		// OR: event-b (400), event-c (100), EOSE = 3 messages (if event-b arrives first)
		// The key is: we should NOT see both event-a and event-b

		// Last message should be EOSE
		assert.Equal(t, MsgTypeEOSE, events[len(events)-1].Type)

		// Check that events are in descending order by created_at
		var createdAts []int64
		for _, msg := range events[:len(events)-1] {
			if msg.Type == MsgTypeEvent {
				createdAts = append(createdAts, msg.Event.CreatedAt.Unix())
			}
		}

		// Verify descending order
		for i := 1; i < len(createdAts); i++ {
			assert.GreaterOrEqual(t, createdAts[i-1], createdAts[i],
				"events should be in descending order by created_at")
		}
	})
}

// TestMergeHandler_Req_Limit tests that EOSE is sent after limit is reached.
func TestMergeHandler_Req_Limit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Storage with many events
		storage := NewInMemoryStorage()
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 500))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 1, 400))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))
		storage.Store(ctx, makeEvent("event-4", "pubkey01", 1, 200))
		storage.Store(ctx, makeEvent("event-5", "pubkey01", 1, 100))

		handler := NewStorageHandler(storage)
		mergeHandler := NewMergeHandler(handler)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go mergeHandler.ServeNostr(ctx, send, recv)

		// Send a REQ with limit=2
		limit := int64(2)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &limit}},
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

		// Should have exactly 2 EVENTs + 1 EOSE = 3 messages
		if len(events) != 3 {
			t.Fatalf("expected 3 messages (2 events + EOSE), got %d", len(events))
		}

		// First 2 should be EVENT, last should be EOSE
		assert.Equal(t, MsgTypeEvent, events[0].Type)
		assert.Equal(t, MsgTypeEvent, events[1].Type)
		assert.Equal(t, MsgTypeEOSE, events[2].Type)
	})
}

// TestMergeHandler_Req_EventAfterHandlerEOSE tests that events sent by a handler
// after its own EOSE are passed through (not dropped by sort order).
// This is important for RouterHandler integration where real-time events
// arrive after EOSE but before all handlers have sent their EOSE.
func TestMergeHandler_Req_EventAfterHandlerEOSE(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Storage with one event (will send EOSE quickly)
		storage := NewInMemoryStorage()
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))

		// A handler that sends events after EOSE (simulating RouterHandler)
		// event-2 has higher created_at than event-1, which would normally be
		// dropped by sort order enforcement. But since it comes after the
		// handler's EOSE, it should be passed through.
		lateEventHandler := &lateEventHandler{
			event: makeEvent("event-2", "pubkey01", 1, 200),
		}

		mergeHandler := NewMergeHandler(NewStorageHandler(storage), lateEventHandler)

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
		var messages []*ServerMsg
		for {
			select {
			case msg := <-send:
				messages = append(messages, msg)
			default:
				goto done
			}
		}
	done:

		// Should have: event-2, event-1, EOSE (order may vary, but all should be present)
		assert.GreaterOrEqual(t, len(messages), 3, "should have at least 3 messages")

		// event-2 should be delivered (not dropped by sort order)
		// This is the key assertion: event-2 (created_at=200) comes after the
		// lateEventHandler's EOSE, so it should pass through even though it
		// has a higher created_at than event-1 (created_at=100).
		foundEvent2 := false
		for _, msg := range messages {
			if msg.Type == MsgTypeEvent && msg.Event.ID == "event-2" {
				foundEvent2 = true
				break
			}
		}
		assert.True(t, foundEvent2, "event-2 should be delivered (not dropped by sort order)")

		// Should have exactly one EOSE
		eoseCount := 0
		for _, msg := range messages {
			if msg.Type == MsgTypeEOSE {
				eoseCount++
			}
		}
		assert.Equal(t, 1, eoseCount, "should have exactly one EOSE")
	})
}

// TestMergeHandler_Count tests that COUNT responses are merged by taking max.
func TestMergeHandler_Count(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Two storages with different event counts
		storage1 := NewInMemoryStorage()
		storage1.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage1.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
		storage1.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		storage2 := NewInMemoryStorage()
		storage2.Store(ctx, makeEvent("event-4", "pubkey01", 1, 400))
		storage2.Store(ctx, makeEvent("event-5", "pubkey01", 1, 500))

		handler1 := NewStorageHandler(storage1) // 3 events
		handler2 := NewStorageHandler(storage2) // 2 events
		mergeHandler := NewMergeHandler(handler1, handler2)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go mergeHandler.ServeNostr(ctx, send, recv)

		// Send a COUNT request
		recv <- &ClientMsg{
			Type:           MsgTypeCount,
			SubscriptionID: "count1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive exactly one COUNT (merged, max of 3 and 2 = 3)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeCount, msg.Type)
			assert.Equal(t, "count1", msg.SubscriptionID)
			assert.Equal(t, uint64(3), msg.Count, "should be max of 3 and 2")
		default:
			t.Fatal("expected COUNT message")
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

// lateEventHandler sends an event after receiving EOSE from storage.
// It simulates RouterHandler behavior (sending events after EOSE).
type lateEventHandler struct {
	event *Event
}

func (h *lateEventHandler) ServeNostr(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-recv:
			if !ok {
				return nil
			}
			if msg.Type == MsgTypeReq {
				// Send EOSE first, then send the late event
				send <- NewServerEOSEMsg(msg.SubscriptionID)
				send <- &ServerMsg{
					Type:           MsgTypeEvent,
					SubscriptionID: msg.SubscriptionID,
					Event:          h.event,
				}
			}
		}
	}
}

// TestMergeHandler_Req_Limit_MultipleHandlers tests limit with multiple handlers.
func TestMergeHandler_Req_Limit_MultipleHandlers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()

		// Two storages, each with 3 events (no overlap)
		storage1 := NewInMemoryStorage()
		storage1.Store(ctx, makeEvent("event-1", "pubkey01", 1, 600))
		storage1.Store(ctx, makeEvent("event-3", "pubkey01", 1, 400))
		storage1.Store(ctx, makeEvent("event-5", "pubkey01", 1, 200))

		storage2 := NewInMemoryStorage()
		storage2.Store(ctx, makeEvent("event-2", "pubkey01", 1, 500))
		storage2.Store(ctx, makeEvent("event-4", "pubkey01", 1, 300))
		storage2.Store(ctx, makeEvent("event-6", "pubkey01", 1, 100))

		handler1 := NewStorageHandler(storage1)
		handler2 := NewStorageHandler(storage2)
		mergeHandler := NewMergeHandler(handler1, handler2)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go mergeHandler.ServeNostr(ctx, send, recv)

		// Send a REQ with limit=3
		// Each handler will return 3 events, but MergeHandler should limit to 3 total
		limit := int64(3)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &limit}},
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

		// Should have at most 3 EVENTs + 1 EOSE = 4 messages
		// (MergeHandler should enforce the limit across all handlers)
		eventCount := 0
		for _, msg := range events {
			if msg.Type == MsgTypeEvent {
				eventCount++
			}
		}

		assert.LessOrEqual(t, eventCount, 3, "should have at most 3 events (limit)")
		assert.Equal(t, MsgTypeEOSE, events[len(events)-1].Type)
	})
}
