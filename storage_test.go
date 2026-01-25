//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create events easily
func makeEvent(id, pubkey string, kind int64, createdAt int64, tags ...Tag) *Event {
	return &Event{
		ID:        id,
		Pubkey:    pubkey,
		Kind:      kind,
		CreatedAt: time.Unix(createdAt, 0),
		Tags:      tags,
	}
}

func toPtr[T any](v T) *T {
	return &v
}

func TestInMemoryStorage_Store_Regular(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Store regular events
	ev1 := makeEvent("event-1", "pubkey01", 1, 100)
	ev2 := makeEvent("event-2", "pubkey01", 1, 200)
	ev3 := makeEvent("event-3", "pubkey01", 1, 300)

	stored, err := s.Store(ctx, ev1)
	require.NoError(t, err)
	assert.True(t, stored)

	stored, err = s.Store(ctx, ev2)
	require.NoError(t, err)
	assert.True(t, stored)

	stored, err = s.Store(ctx, ev3)
	require.NoError(t, err)
	assert.True(t, stored)

	assert.Equal(t, 3, s.Len())

	// Duplicate should not be stored
	stored, err = s.Store(ctx, ev1)
	require.NoError(t, err)
	assert.False(t, stored)
	assert.Equal(t, 3, s.Len())
}

func TestInMemoryStorage_Store_Replaceable(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Replaceable events: kind 10000-19999, also kind 0 and 3
	// Same (pubkey, kind) -> keep only the newest

	ev1 := makeEvent("event-1", "pubkey01", 10000, 100)
	ev2 := makeEvent("event-2", "pubkey01", 10000, 200)
	ev3 := makeEvent("event-3", "pubkey01", 10000, 50) // Older

	stored, err := s.Store(ctx, ev1)
	require.NoError(t, err)
	assert.True(t, stored)
	assert.Equal(t, 1, s.Len())

	// Newer replaces older
	stored, err = s.Store(ctx, ev2)
	require.NoError(t, err)
	assert.True(t, stored)
	assert.Equal(t, 1, s.Len())

	// Older should not replace newer
	stored, err = s.Store(ctx, ev3)
	require.NoError(t, err)
	assert.False(t, stored)
	assert.Equal(t, 1, s.Len())

	// Query should return only the newest
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "event-2", events[0].ID)
}

func TestInMemoryStorage_Store_Replaceable_SameTimestamp(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Same timestamp: keep lexically smaller ID
	ev1 := makeEvent("event-b", "pubkey01", 10000, 100)
	ev2 := makeEvent("event-a", "pubkey01", 10000, 100) // Same timestamp, smaller ID

	stored, err := s.Store(ctx, ev1)
	require.NoError(t, err)
	assert.True(t, stored)

	stored, err = s.Store(ctx, ev2)
	require.NoError(t, err)
	assert.True(t, stored) // Should replace because "event-a" < "event-b"

	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "event-a", events[0].ID)
}

func TestInMemoryStorage_Store_Addressable(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Addressable events: kind 30000-39999
	// Same (pubkey, kind, d-tag) -> keep only the newest

	ev1 := makeEvent("event-1", "pubkey01", 30000, 100, Tag{"d", "param1"})
	ev2 := makeEvent("event-2", "pubkey01", 30000, 200, Tag{"d", "param1"}) // Same address
	ev3 := makeEvent("event-3", "pubkey01", 30000, 300, Tag{"d", "param2"}) // Different address

	stored, err := s.Store(ctx, ev1)
	require.NoError(t, err)
	assert.True(t, stored)

	// Same address, newer -> replaces
	stored, err = s.Store(ctx, ev2)
	require.NoError(t, err)
	assert.True(t, stored)
	assert.Equal(t, 1, s.Len())

	// Different address -> new entry
	stored, err = s.Store(ctx, ev3)
	require.NoError(t, err)
	assert.True(t, stored)
	assert.Equal(t, 2, s.Len())
}

func TestInMemoryStorage_Store_Ephemeral(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Ephemeral events: kind 20000-29999
	// Should NOT be stored
	ev := makeEvent("event-1", "pubkey01", 20000, 100)

	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.False(t, stored)
	assert.Equal(t, 0, s.Len())
}

func TestInMemoryStorage_Store_Kind5_DeleteByEventID(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Store an event
	ev := makeEvent("event-1", "pubkey01", 1, 100)
	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.True(t, stored)

	// Delete it with kind 5
	delReq := makeEvent("kind5-1", "pubkey01", 5, 200, Tag{"e", "event-1"})
	stored, err = s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Only the kind 5 event should remain
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "kind5-1", events[0].ID)
}

func TestInMemoryStorage_Store_Kind5_PreventFutureEvent(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Kind 5 arrives first
	delReq := makeEvent("kind5-1", "pubkey01", 5, 200, Tag{"e", "event-1"})
	stored, err := s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Now the deleted event arrives -> should be rejected
	ev := makeEvent("event-1", "pubkey01", 1, 100)
	stored, err = s.Store(ctx, ev)
	require.NoError(t, err)
	assert.False(t, stored)

	assert.Equal(t, 1, s.Len()) // Only kind 5 remains
}

func TestInMemoryStorage_Store_Kind5_DifferentPubkey(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Store an event
	ev := makeEvent("event-1", "pubkey01", 1, 100)
	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.True(t, stored)

	// Try to delete with different pubkey -> should NOT delete
	delReq := makeEvent("kind5-1", "pubkey02", 5, 200, Tag{"e", "event-1"})
	stored, err = s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored) // The kind 5 itself is stored

	// Both events should exist
	assert.Equal(t, 2, s.Len())
}

func TestInMemoryStorage_Store_Kind5_DeleteByAddress(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Store an addressable event
	ev := makeEvent("event-1", "pubkey01", 30000, 100, Tag{"d", "param"})
	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.True(t, stored)

	// Delete by address
	delReq := makeEvent("kind5-1", "pubkey01", 5, 200, Tag{"a", "30000:pubkey01:param"})
	stored, err = s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Only the kind 5 should remain
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "kind5-1", events[0].ID)
}

func TestInMemoryStorage_Store_Kind5_DeleteItself(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Kind 5 that deletes itself
	delReq := makeEvent("kind5-1", "pubkey01", 5, 100, Tag{"e", "kind5-1"})
	stored, err := s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// The kind 5 should have deleted itself
	assert.Equal(t, 0, s.Len())
}

func TestInMemoryStorage_Store_Kind5_DeleteKind5(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// First kind 5
	delReq1 := makeEvent("kind5-1", "pubkey01", 5, 100, Tag{"e", "event-1"})
	stored, err := s.Store(ctx, delReq1)
	require.NoError(t, err)
	assert.True(t, stored)

	// Second kind 5 that tries to delete the first
	delReq2 := makeEvent("kind5-2", "pubkey01", 5, 200, Tag{"e", "kind5-1"})
	stored, err = s.Store(ctx, delReq2)
	require.NoError(t, err)
	assert.True(t, stored)

	// NIP-09: "deletion request event against a deletion request has no effect"
	// Both kind 5 events should remain
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "kind5-2", events[0].ID)
	assert.Equal(t, "kind5-1", events[1].ID)
}

func TestInMemoryStorage_Store_Kind5_DeleteByAddress_Timestamp(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Delete request at timestamp 200
	delReq := makeEvent("kind5-1", "pubkey01", 5, 200, Tag{"a", "30000:pubkey01:param"})
	stored, err := s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Event created BEFORE the deletion (timestamp 100) -> should be rejected
	evBefore := makeEvent("event-before", "pubkey01", 30000, 100, Tag{"d", "param"})
	stored, err = s.Store(ctx, evBefore)
	require.NoError(t, err)
	assert.False(t, stored, "event created before deletion should be rejected")

	// Event created AT the same time as deletion (timestamp 200) -> should be rejected
	evSame := makeEvent("event-same", "pubkey01", 30000, 200, Tag{"d", "param"})
	stored, err = s.Store(ctx, evSame)
	require.NoError(t, err)
	assert.False(t, stored, "event created at same time as deletion should be rejected")

	// Event created AFTER the deletion (timestamp 300) -> should be stored!
	evAfter := makeEvent("event-after", "pubkey01", 30000, 300, Tag{"d", "param"})
	stored, err = s.Store(ctx, evAfter)
	require.NoError(t, err)
	assert.True(t, stored, "event created after deletion should be stored")

	// Query should return kind5 and the event created after
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "event-after", events[0].ID)
	assert.Equal(t, "kind5-1", events[1].ID)
}

func TestInMemoryStorage_Query_Empty(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	assert.Nil(t, events)
}

func TestInMemoryStorage_Query_Sorted(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Store in random order
	_, _ = s.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))
	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))

	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Should be sorted by created_at DESC
	assert.Equal(t, "event-3", events[0].ID)
	assert.Equal(t, "event-2", events[1].ID)
	assert.Equal(t, "event-1", events[2].ID)
}

func TestInMemoryStorage_Query_SortedSameTimestamp(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	// Same timestamp, different IDs
	_, _ = s.Store(ctx, makeEvent("event-c", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-a", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-b", "pubkey01", 1, 100))

	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Same timestamp: sorted by ID ASC (lexical)
	assert.Equal(t, "event-a", events[0].ID)
	assert.Equal(t, "event-b", events[1].ID)
	assert.Equal(t, "event-c", events[2].ID)
}

func TestInMemoryStorage_Query_WithLimit(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	for i := range 10 {
		_, _ = s.Store(ctx, makeEvent(fmt.Sprintf("event-%d", i), "pubkey01", 1, int64(i*100)))
	}

	events, err := s.Query(ctx, []*ReqFilter{{Limit: toPtr[int64](3)}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Should be the 3 newest
	assert.Equal(t, "event-9", events[0].ID)
	assert.Equal(t, "event-8", events[1].ID)
	assert.Equal(t, "event-7", events[2].ID)
}

func TestInMemoryStorage_Query_LimitZero(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))

	events, err := s.Query(ctx, []*ReqFilter{{Limit: toPtr[int64](0)}})
	require.NoError(t, err)
	assert.Nil(t, events)
}

func TestInMemoryStorage_Query_FilterByIDs(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
	_, _ = s.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

	events, err := s.Query(ctx, []*ReqFilter{{IDs: []string{"event-1", "event-3"}}})
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, "event-3", events[0].ID)
	assert.Equal(t, "event-1", events[1].ID)
}

func TestInMemoryStorage_Query_FilterByAuthors(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey02", 1, 200))
	_, _ = s.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

	events, err := s.Query(ctx, []*ReqFilter{{Authors: []string{"pubkey01"}}})
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, "event-3", events[0].ID)
	assert.Equal(t, "event-1", events[1].ID)
}

func TestInMemoryStorage_Query_FilterByKinds(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey01", 2, 200))
	_, _ = s.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

	events, err := s.Query(ctx, []*ReqFilter{{Kinds: []int64{1}}})
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, "event-3", events[0].ID)
	assert.Equal(t, "event-1", events[1].ID)
}

func TestInMemoryStorage_Query_MultipleFilters(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey02", 1, 200))
	_, _ = s.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

	// Multiple filters = OR
	events, err := s.Query(ctx, []*ReqFilter{
		{IDs: []string{"event-1"}},
		{IDs: []string{"event-2"}},
	})
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, "event-2", events[0].ID)
	assert.Equal(t, "event-1", events[1].ID)
}

func TestInMemoryStorage_Query_MultipleFiltersDedup(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))

	// Same event matches both filters -> should appear only once
	events, err := s.Query(ctx, []*ReqFilter{
		{IDs: []string{"event-1"}},
		{IDs: []string{"event-1"}},
	})
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, "event-1", events[0].ID)
}

func TestInMemoryStorage_Query_MultipleFiltersWithLimit(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
	_, _ = s.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

	// Each filter has its own limit
	events, err := s.Query(ctx, []*ReqFilter{
		{Limit: toPtr[int64](1)}, // Gets event-3
		{Limit: toPtr[int64](1)}, // Also tries event-3, but deduped, so gets nothing new
	})
	require.NoError(t, err)
	// Hmm, this depends on implementation. Let's check the result.
	// With dedup happening during iteration, second filter might get event-2.
	// Actually, looking at the implementation:
	// - First filter: count 0, limit 1, sees event-3, count becomes 1, limit reached
	// - Second filter: count 0, limit 1, sees event-3 (already in seen), skips, sees event-2, count becomes 1
	// So result should be event-3 and event-2
	require.Len(t, events, 2)
	assert.Equal(t, "event-3", events[0].ID)
	assert.Equal(t, "event-2", events[1].ID)
}

func TestInMemoryStorage_Delete(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	_, _ = s.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))

	err := s.Delete(ctx, "event-1", "pubkey01")
	require.NoError(t, err)

	assert.Equal(t, 1, s.Len())

	// Should not be able to re-add the deleted event
	stored, err := s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
	require.NoError(t, err)
	assert.False(t, stored)
}

func TestInMemoryStorage_Delete_DifferentPubkey(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))

	// Try to delete with different pubkey -> should not delete
	err := s.Delete(ctx, "event-1", "pubkey02")
	require.NoError(t, err)

	assert.Equal(t, 1, s.Len()) // Still there
}

func TestInMemoryStorage_DeleteByAddr(t *testing.T) {
	ctx := context.Background()
	s := NewInMemoryStorage()

	_, _ = s.Store(ctx, makeEvent("event-1", "pubkey01", 30000, 100, Tag{"d", "param"}))

	err := s.DeleteByAddr(ctx, 30000, "pubkey01", "param")
	require.NoError(t, err)

	assert.Equal(t, 0, s.Len())

	// Should not be able to re-add
	stored, err := s.Store(ctx, makeEvent("event-2", "pubkey01", 30000, 200, Tag{"d", "param"}))
	require.NoError(t, err)
	assert.False(t, stored)
}
