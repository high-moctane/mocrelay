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

func TestPebbleStorage_Query_FilterBySince(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 1, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 1, 300))
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey, 1, 400))

	// since=200 means created_at >= 200
	events, err := s.Query(ctx, []*ReqFilter{{Since: toPtr[int64](200)}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	assert.Equal(t, "4444444444444444444444444444444444444444444444444444444444444444", events[0].ID)
	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[1].ID)
	assert.Equal(t, "2222222222222222222222222222222222222222222222222222222222222222", events[2].ID)
}

func TestPebbleStorage_Query_FilterByUntil(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 1, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 1, 300))
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey, 1, 400))

	// until=300 means created_at <= 300
	events, err := s.Query(ctx, []*ReqFilter{{Until: toPtr[int64](300)}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[0].ID)
	assert.Equal(t, "2222222222222222222222222222222222222222222222222222222222222222", events[1].ID)
	assert.Equal(t, "1111111111111111111111111111111111111111111111111111111111111111", events[2].ID)
}

func TestPebbleStorage_Query_FilterBySinceAndUntil(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 1, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 1, 300))
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey, 1, 400))

	// since=200, until=300 means 200 <= created_at <= 300
	events, err := s.Query(ctx, []*ReqFilter{{Since: toPtr[int64](200), Until: toPtr[int64](300)}})
	require.NoError(t, err)
	require.Len(t, events, 2)

	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[0].ID)
	assert.Equal(t, "2222222222222222222222222222222222222222222222222222222222222222", events[1].ID)
}

func TestPebbleStorage_Query_MultipleKinds(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Store events with different kinds and timestamps
	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 2, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 3, 300)) // Not in filter
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey, 1, 400))
	_, _ = s.Store(ctx, makeEvent("5555555555555555555555555555555555555555555555555555555555555555", pubkey, 2, 500))

	// Query kinds 1 and 2 (multi-cursor merge!)
	events, err := s.Query(ctx, []*ReqFilter{{Kinds: []int64{1, 2}}})
	require.NoError(t, err)
	require.Len(t, events, 4)

	// Should be sorted by created_at DESC
	assert.Equal(t, "5555555555555555555555555555555555555555555555555555555555555555", events[0].ID) // 500
	assert.Equal(t, "4444444444444444444444444444444444444444444444444444444444444444", events[1].ID) // 400
	assert.Equal(t, "2222222222222222222222222222222222222222222222222222222222222222", events[2].ID) // 200
	assert.Equal(t, "1111111111111111111111111111111111111111111111111111111111111111", events[3].ID) // 100
}

func TestPebbleStorage_Query_MultipleAuthors(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pubkey2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pubkey3 := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey1, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey2, 1, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey3, 1, 300)) // Not in filter
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey1, 1, 400))
	_, _ = s.Store(ctx, makeEvent("5555555555555555555555555555555555555555555555555555555555555555", pubkey2, 1, 500))

	// Query pubkey1 and pubkey2 (multi-cursor merge!)
	events, err := s.Query(ctx, []*ReqFilter{{Authors: []string{pubkey1, pubkey2}}})
	require.NoError(t, err)
	require.Len(t, events, 4)

	// Should be sorted by created_at DESC
	assert.Equal(t, "5555555555555555555555555555555555555555555555555555555555555555", events[0].ID) // 500
	assert.Equal(t, "4444444444444444444444444444444444444444444444444444444444444444", events[1].ID) // 400
	assert.Equal(t, "2222222222222222222222222222222222222222222222222222222222222222", events[2].ID) // 200
	assert.Equal(t, "1111111111111111111111111111111111111111111111111111111111111111", events[3].ID) // 100
}

func TestPebbleStorage_Query_MultipleKindsWithLimit(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 2, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 1, 300))
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey, 2, 400))
	_, _ = s.Store(ctx, makeEvent("5555555555555555555555555555555555555555555555555555555555555555", pubkey, 1, 500))

	// Query kinds 1 and 2 with limit=3
	events, err := s.Query(ctx, []*ReqFilter{{Kinds: []int64{1, 2}, Limit: toPtr[int64](3)}})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Should be the 3 newest
	assert.Equal(t, "5555555555555555555555555555555555555555555555555555555555555555", events[0].ID) // 500
	assert.Equal(t, "4444444444444444444444444444444444444444444444444444444444444444", events[1].ID) // 400
	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[2].ID) // 300
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

// Step 4: Multiple filters (OR search)

func TestPebbleStorage_Query_MultipleFilters(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Store events with different kinds
	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 2, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 3, 300))
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey, 1, 400))
	_, _ = s.Store(ctx, makeEvent("5555555555555555555555555555555555555555555555555555555555555555", pubkey, 2, 500))

	// Query with two filters: kind=1 OR kind=3
	// This is [filter1, filter2] style OR, not {"kinds": [1, 3]}
	events, err := s.Query(ctx, []*ReqFilter{
		{Kinds: []int64{1}},
		{Kinds: []int64{3}},
	})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Should be sorted by created_at DESC across all filters
	assert.Equal(t, "4444444444444444444444444444444444444444444444444444444444444444", events[0].ID) // kind=1, 400
	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[1].ID) // kind=3, 300
	assert.Equal(t, "1111111111111111111111111111111111111111111111111111111111111111", events[2].ID) // kind=1, 100
}

func TestPebbleStorage_Query_MultipleFilters_WithLimit(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))
	_, _ = s.Store(ctx, makeEvent("2222222222222222222222222222222222222222222222222222222222222222", pubkey, 2, 200))
	_, _ = s.Store(ctx, makeEvent("3333333333333333333333333333333333333333333333333333333333333333", pubkey, 3, 300))
	_, _ = s.Store(ctx, makeEvent("4444444444444444444444444444444444444444444444444444444444444444", pubkey, 1, 400))
	_, _ = s.Store(ctx, makeEvent("5555555555555555555555555555555555555555555555555555555555555555", pubkey, 2, 500))

	// NIP-01: "only return events from the first filter's limit"
	// filter1 has limit=2, filter2 has no limit
	events, err := s.Query(ctx, []*ReqFilter{
		{Kinds: []int64{1}, Limit: toPtr[int64](2)},
		{Kinds: []int64{3}},
	})
	require.NoError(t, err)
	require.Len(t, events, 2, "should respect first filter's limit")

	// Should be the 2 newest events across all filters
	assert.Equal(t, "4444444444444444444444444444444444444444444444444444444444444444", events[0].ID) // kind=1, 400
	assert.Equal(t, "3333333333333333333333333333333333333333333333333333333333333333", events[1].ID) // kind=3, 300
}

func TestPebbleStorage_Query_MultipleFilters_Dedup(t *testing.T) {
	ctx := context.Background()
	s := setupPebbleStorage(t)

	pubkey := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Event that matches BOTH filters
	_, _ = s.Store(ctx, makeEvent("1111111111111111111111111111111111111111111111111111111111111111", pubkey, 1, 100))

	// Query with two filters that both match the same event
	events, err := s.Query(ctx, []*ReqFilter{
		{Kinds: []int64{1}},
		{Authors: []string{pubkey}},
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "should deduplicate events that match multiple filters")

	assert.Equal(t, "1111111111111111111111111111111111111111111111111111111111111111", events[0].ID)
}
