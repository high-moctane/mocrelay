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
