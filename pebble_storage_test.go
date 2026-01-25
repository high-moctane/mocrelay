//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPebbleStorage(t *testing.T) *PebbleStorage {
	t.Helper()
	dir := t.TempDir()
	s, err := NewPebbleStorage(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		s.Close()
	})
	return s
}

func TestPebbleStorage_Store_Regular(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	// Store regular events
	ev1 := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 100)
	ev2 := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 200)
	ev3 := makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 300)

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

func TestPebbleStorage_Store_Ephemeral(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	// Ephemeral events (kind 20000-29999) should NOT be stored
	ev := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 20000, 100)

	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.False(t, stored)
	assert.Equal(t, 0, s.Len())
}

func TestPebbleStorage_Query_Empty(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	assert.Nil(t, events)
}

func TestPebbleStorage_Query_Sorted(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	// Store in random order
	_, _ = s.Store(ctx, makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 300))
	_, _ = s.Store(ctx, makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 100))
	_, _ = s.Store(ctx, makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 200))

	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Should be sorted by created_at DESC
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", events[0].ID)
	assert.Equal(t, "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", events[1].ID)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", events[2].ID)
}

func TestPebbleStorage_Query_WithLimit(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	// Store 5 events
	_, _ = s.Store(ctx, makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 100))
	_, _ = s.Store(ctx, makeEvent("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 200))
	_, _ = s.Store(ctx, makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 300))
	_, _ = s.Store(ctx, makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 400))
	_, _ = s.Store(ctx, makeEvent("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 500))

	events, err := s.Query(ctx, []*ReqFilter{{Limit: toPtr[int64](3)}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Should be the 3 newest
	assert.Equal(t, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", events[0].ID)
	assert.Equal(t, "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", events[1].ID)
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", events[2].ID)
}

func TestPebbleStorage_Query_FilterByKinds(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	_, _ = s.Store(ctx, makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 100))
	_, _ = s.Store(ctx, makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 2, 200))
	_, _ = s.Store(ctx, makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, 300))

	events, err := s.Query(ctx, []*ReqFilter{{Kinds: []int64{1}}})
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", events[0].ID)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", events[1].ID)
}

func TestPebbleStorage_Query_FilterByAuthors(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pubkey2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey1, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey2, 1, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey1, 1, 300))

	events, err := s.Query(ctx, []*ReqFilter{{Authors: []string{pubkey1}}})
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Should be sorted by created_at DESC
	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[0].ID)
	assert.Equal(t, "1111111111111111111111111111111111111111111111111111111111111111", events[1].ID)
}

func TestPebbleStorage_Query_FilterByTag(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	targetID := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

	// Events with "e" tag
	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100, Tag{"e", targetID}))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 1, 200)) // No tag
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 1, 300, Tag{"e", targetID}))

	events, err := s.Query(ctx, []*ReqFilter{{Tags: map[string][]string{"e": {targetID}}}})
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Should be sorted by created_at DESC
	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[0].ID)
	assert.Equal(t, "1111111111111111111111111111111111111111111111111111111111111111", events[1].ID)
}

func TestPebbleStorage_Store_Replaceable(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Replaceable events: kind 10000-19999, also kind 0 and 3
	// Same (pubkey, kind) -> keep only the newest

	ev1 := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 10000, 100)
	ev2 := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 10000, 200)
	ev3 := makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", pubkey, 10000, 50) // Older

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
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", events[0].ID)
}

func TestPebbleStorage_Store_Replaceable_SameTimestamp(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Same timestamp: keep lexically smaller ID
	ev1 := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 10000, 100) // "c..." > "a..."
	ev2 := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 10000, 100) // Same timestamp, smaller ID

	stored, err := s.Store(ctx, ev1)
	require.NoError(t, err)
	assert.True(t, stored)

	stored, err = s.Store(ctx, ev2)
	require.NoError(t, err)
	assert.True(t, stored) // Should replace because "a..." < "c..."

	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", events[0].ID)
}

func TestPebbleStorage_Store_Addressable(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Addressable events: kind 30000-39999
	// Same (pubkey, kind, d-tag) -> keep only the newest

	ev1 := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 30000, 100, Tag{"d", "param1"})
	ev2 := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 30000, 200, Tag{"d", "param1"}) // Same address
	ev3 := makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", pubkey, 30000, 300, Tag{"d", "param2"}) // Different address

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

func TestPebbleStorage_Store_Kind5_DeleteByEventID(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Store an event
	ev := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 1, 100)
	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.True(t, stored)

	// Delete it with kind 5
	delReq := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 5, 200,
		Tag{"e", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	stored, err = s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Only the kind 5 event should remain
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", events[0].ID)
}

func TestPebbleStorage_Store_Kind5_PreventFutureEvent(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Kind 5 arrives first
	delReq := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 5, 200,
		Tag{"e", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	stored, err := s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Now the deleted event arrives -> should be rejected
	ev := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 1, 100)
	stored, err = s.Store(ctx, ev)
	require.NoError(t, err)
	assert.False(t, stored)

	assert.Equal(t, 1, s.Len()) // Only kind 5 remains
}

func TestPebbleStorage_Store_Kind5_DifferentPubkey(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey1 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pubkey2 := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	// Store an event
	ev := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey1, 1, 100)
	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.True(t, stored)

	// Try to delete with different pubkey -> should NOT delete
	delReq := makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", pubkey2, 5, 200,
		Tag{"e", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	stored, err = s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored) // The kind 5 itself is stored

	// Both events should exist
	assert.Equal(t, 2, s.Len())
}

func TestPebbleStorage_Store_Kind5_DeleteByAddress(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Store an addressable event
	ev := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 30000, 100, Tag{"d", "param"})
	stored, err := s.Store(ctx, ev)
	require.NoError(t, err)
	assert.True(t, stored)

	// Delete by address
	delReq := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 5, 200,
		Tag{"a", "30000:" + pubkey + ":param"})
	stored, err = s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Only the kind 5 should remain
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", events[0].ID)
}

func TestPebbleStorage_Store_Kind5_DeleteByAddress_Timestamp(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Delete request at timestamp 200
	delReq := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 5, 200,
		Tag{"a", "30000:" + pubkey + ":param"})
	stored, err := s.Store(ctx, delReq)
	require.NoError(t, err)
	assert.True(t, stored)

	// Event created BEFORE the deletion (timestamp 100) -> should be rejected
	evBefore := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 30000, 100, Tag{"d", "param"})
	stored, err = s.Store(ctx, evBefore)
	require.NoError(t, err)
	assert.False(t, stored, "event created before deletion should be rejected")

	// Event created AT the same time as deletion (timestamp 200) -> should be rejected
	evSame := makeEvent("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", pubkey, 30000, 200, Tag{"d", "param"})
	stored, err = s.Store(ctx, evSame)
	require.NoError(t, err)
	assert.False(t, stored, "event created at same time as deletion should be rejected")

	// Event created AFTER the deletion (timestamp 300) -> should be stored!
	evAfter := makeEvent("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", pubkey, 30000, 300, Tag{"d", "param"})
	stored, err = s.Store(ctx, evAfter)
	require.NoError(t, err)
	assert.True(t, stored, "event created after deletion should be stored")

	// Query should return kind5 and the event created after
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", events[0].ID)
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", events[1].ID)
}

func TestPebbleStorage_Store_Kind5_DeleteKind5(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// First kind 5
	delReq1 := makeEvent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pubkey, 5, 100,
		Tag{"e", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"})
	stored, err := s.Store(ctx, delReq1)
	require.NoError(t, err)
	assert.True(t, stored)

	// Second kind 5 that tries to delete the first
	delReq2 := makeEvent("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", pubkey, 5, 200,
		Tag{"e", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	stored, err = s.Store(ctx, delReq2)
	require.NoError(t, err)
	assert.True(t, stored)

	// NIP-09: "deletion request event against a deletion request has no effect"
	// Both kind 5 events should remain
	events, err := s.Query(ctx, []*ReqFilter{{}})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", events[0].ID)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", events[1].ID)
}
