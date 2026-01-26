//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cockroachdb/pebble/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStorageDifferential verifies that InMemoryStorage and PebbleStorage
// behave identically for the same sequence of operations.
func TestStorageDifferential(t *testing.T) {
	seeds := []uint64{0, 1, 42, 12345, 98765}
	for _, seed := range seeds {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			testStorageDifferentialWithSeed(t, seed)
		})
	}
}

func testStorageDifferentialWithSeed(t *testing.T, seed uint64) {
	ctx := context.Background()

	// Create both storages
	inMemory := NewInMemoryStorage()

	pebbleStorage, err := NewPebbleStorageWithFS("test", vfs.NewMem())
	require.NoError(t, err)
	defer pebbleStorage.Close()

	// Use deterministic random generator
	rng := rand.New(rand.NewPCG(seed, seed))

	// Generate and store random events
	numEvents := 100
	events := generateRandomEvents(rng, numEvents)

	// Store events in both storages and compare results
	for i, event := range events {
		storedInMem, errInMem := inMemory.Store(ctx, event)
		storedPebble, errPebble := pebbleStorage.Store(ctx, event)

		assert.Equal(t, errInMem == nil, errPebble == nil,
			"event %d: error mismatch (inMem=%v, pebble=%v)", i, errInMem, errPebble)
		assert.Equal(t, storedInMem, storedPebble,
			"event %d: stored mismatch (inMem=%v, pebble=%v)", i, storedInMem, storedPebble)
	}

	// Generate random queries and compare results
	numQueries := 50
	for i := range numQueries {
		filters := generateRandomFilters(rng, events)

		eventsInMem := queryStorage(t, inMemory, ctx, filters)
		eventsPebble := queryStorage(t, pebbleStorage, ctx, filters)

		// Compare event IDs (order matters for limit queries)
		idsInMem := extractIDs(eventsInMem)
		idsPebble := extractIDs(eventsPebble)

		assert.Equal(t, idsInMem, idsPebble,
			"query %d: results mismatch\nfilters: %+v\ninMem: %v\npebble: %v",
			i, filters, idsInMem, idsPebble)
	}
}

// queryStorage is a helper that works with any Storage implementation.
func queryStorage(t *testing.T, s Storage, ctx context.Context, filters []*ReqFilter) []*Event {
	t.Helper()
	events, errFn, closeFn := s.Query(ctx, filters)
	defer closeFn()
	var result []*Event
	for event := range events {
		result = append(result, event)
	}
	require.NoError(t, errFn())
	return result
}

func extractIDs(events []*Event) []string {
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids
}

// generateRandomEvents creates random events for testing.
func generateRandomEvents(rng *rand.Rand, n int) []*Event {
	events := make([]*Event, n)
	pubkeys := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}
	kinds := []int64{1, 3, 5, 1000, 10000, 20000, 30000} // regular, replaceable, deletion, ephemeral, addressable

	for i := range n {
		pubkey := pubkeys[rng.IntN(len(pubkeys))]
		kind := kinds[rng.IntN(len(kinds))]
		createdAt := int64(1000 + rng.IntN(10000))

		// Generate unique ID
		id := fmt.Sprintf("%064x", rng.Uint64())[:64]

		var tags []Tag
		// Add d-tag for addressable events
		if kind >= 30000 && kind < 40000 {
			dValue := fmt.Sprintf("d-%d", rng.IntN(5))
			tags = append(tags, Tag{"d", dValue})
		}
		// Sometimes add e/p tags
		if rng.Float64() < 0.3 {
			tags = append(tags, Tag{"e", fmt.Sprintf("%064x", rng.Uint64())[:64]})
		}
		if rng.Float64() < 0.3 {
			tags = append(tags, Tag{"p", pubkeys[rng.IntN(len(pubkeys))]})
		}

		events[i] = &Event{
			ID:        id,
			Pubkey:    pubkey,
			Kind:      kind,
			CreatedAt: time.Unix(createdAt, 0),
			Tags:      tags,
		}
	}
	return events
}

// generateRandomFilters creates random query filters.
func generateRandomFilters(rng *rand.Rand, existingEvents []*Event) []*ReqFilter {
	filter := &ReqFilter{}

	// Randomly pick conditions
	if rng.Float64() < 0.3 && len(existingEvents) > 0 {
		// Filter by IDs
		numIDs := rng.IntN(3) + 1
		for range numIDs {
			event := existingEvents[rng.IntN(len(existingEvents))]
			filter.IDs = append(filter.IDs, event.ID)
		}
	}

	if rng.Float64() < 0.5 {
		// Filter by authors
		authors := []string{
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}
		filter.Authors = append(filter.Authors, authors[rng.IntN(len(authors))])
	}

	if rng.Float64() < 0.5 {
		// Filter by kinds
		kinds := []int64{1, 3, 1000, 10000, 30000}
		filter.Kinds = append(filter.Kinds, kinds[rng.IntN(len(kinds))])
	}

	if rng.Float64() < 0.3 {
		// Filter by since/until
		since := int64(1000 + rng.IntN(5000))
		filter.Since = &since
	}
	if rng.Float64() < 0.3 {
		until := int64(5000 + rng.IntN(5000))
		filter.Until = &until
	}

	// Always set a limit
	limit := int64(rng.IntN(20) + 1)
	filter.Limit = &limit

	return []*ReqFilter{filter}
}

// TestStorageDifferential_Kind5 specifically tests deletion behavior.
func TestStorageDifferential_Kind5(t *testing.T) {
	ctx := context.Background()

	inMemory := NewInMemoryStorage()
	pebbleStorage, err := NewPebbleStorageWithFS("test", vfs.NewMem())
	require.NoError(t, err)
	defer pebbleStorage.Close()

	pubkey := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Store a regular event
	event1 := &Event{
		ID:        "1111111111111111111111111111111111111111111111111111111111111111",
		Pubkey:    pubkey,
		Kind:      1,
		CreatedAt: time.Unix(1000, 0),
	}

	storedInMem, _ := inMemory.Store(ctx, event1)
	storedPebble, _ := pebbleStorage.Store(ctx, event1)
	assert.Equal(t, storedInMem, storedPebble, "initial store")

	// Store kind 5 deletion request
	deletion := &Event{
		ID:        "5555555555555555555555555555555555555555555555555555555555555555",
		Pubkey:    pubkey,
		Kind:      5,
		CreatedAt: time.Unix(2000, 0),
		Tags:      []Tag{{"e", event1.ID}},
	}

	storedInMem, _ = inMemory.Store(ctx, deletion)
	storedPebble, _ = pebbleStorage.Store(ctx, deletion)
	assert.Equal(t, storedInMem, storedPebble, "deletion store")

	// Query for the deleted event - should not be found
	filters := []*ReqFilter{{IDs: []string{event1.ID}}}
	eventsInMem := queryStorage(t, inMemory, ctx, filters)
	eventsPebble := queryStorage(t, pebbleStorage, ctx, filters)

	assert.Equal(t, len(eventsInMem), len(eventsPebble), "deleted event query")

	// Try to store the same event again - should be rejected
	storedInMem, _ = inMemory.Store(ctx, event1)
	storedPebble, _ = pebbleStorage.Store(ctx, event1)
	assert.Equal(t, storedInMem, storedPebble, "re-store deleted event")
}

// TestStorageDifferential_Replaceable tests replaceable event behavior.
func TestStorageDifferential_Replaceable(t *testing.T) {
	ctx := context.Background()

	inMemory := NewInMemoryStorage()
	pebbleStorage, err := NewPebbleStorageWithFS("test", vfs.NewMem())
	require.NoError(t, err)
	defer pebbleStorage.Close()

	pubkey := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Store replaceable events with same (pubkey, kind)
	events := []*Event{
		{ID: "1111111111111111111111111111111111111111111111111111111111111111", Pubkey: pubkey, Kind: 10000, CreatedAt: time.Unix(1000, 0)},
		{ID: "2222222222222222222222222222222222222222222222222222222222222222", Pubkey: pubkey, Kind: 10000, CreatedAt: time.Unix(2000, 0)}, // newer
		{ID: "3333333333333333333333333333333333333333333333333333333333333333", Pubkey: pubkey, Kind: 10000, CreatedAt: time.Unix(500, 0)},  // older
	}

	for _, event := range events {
		storedInMem, _ := inMemory.Store(ctx, event)
		storedPebble, _ := pebbleStorage.Store(ctx, event)
		assert.Equal(t, storedInMem, storedPebble, "store event %s", event.ID)
	}

	// Query all - should only have one event (the newest)
	filters := []*ReqFilter{{Kinds: []int64{10000}, Authors: []string{pubkey}}}
	eventsInMem := queryStorage(t, inMemory, ctx, filters)
	eventsPebble := queryStorage(t, pebbleStorage, ctx, filters)

	idsInMem := extractIDs(eventsInMem)
	idsPebble := extractIDs(eventsPebble)
	assert.Equal(t, idsInMem, idsPebble, "replaceable query")
	assert.Len(t, idsInMem, 1, "should have only one replaceable event")
}

// TestStorageDifferential_Addressable tests addressable event behavior.
func TestStorageDifferential_Addressable(t *testing.T) {
	ctx := context.Background()

	inMemory := NewInMemoryStorage()
	pebbleStorage, err := NewPebbleStorageWithFS("test", vfs.NewMem())
	require.NoError(t, err)
	defer pebbleStorage.Close()

	pubkey := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Store addressable events with same (pubkey, kind, d-tag)
	events := []*Event{
		{ID: "1111111111111111111111111111111111111111111111111111111111111111", Pubkey: pubkey, Kind: 30000, CreatedAt: time.Unix(1000, 0), Tags: []Tag{{"d", "test"}}},
		{ID: "2222222222222222222222222222222222222222222222222222222222222222", Pubkey: pubkey, Kind: 30000, CreatedAt: time.Unix(2000, 0), Tags: []Tag{{"d", "test"}}},  // newer
		{ID: "3333333333333333333333333333333333333333333333333333333333333333", Pubkey: pubkey, Kind: 30000, CreatedAt: time.Unix(500, 0), Tags: []Tag{{"d", "test"}}},   // older
		{ID: "4444444444444444444444444444444444444444444444444444444444444444", Pubkey: pubkey, Kind: 30000, CreatedAt: time.Unix(1500, 0), Tags: []Tag{{"d", "other"}}}, // different d-tag
	}

	for _, event := range events {
		storedInMem, _ := inMemory.Store(ctx, event)
		storedPebble, _ := pebbleStorage.Store(ctx, event)
		assert.Equal(t, storedInMem, storedPebble, "store event %s", event.ID)
	}

	// Query all addressable - should have 2 (one per d-tag)
	filters := []*ReqFilter{{Kinds: []int64{30000}, Authors: []string{pubkey}}}
	eventsInMem := queryStorage(t, inMemory, ctx, filters)
	eventsPebble := queryStorage(t, pebbleStorage, ctx, filters)

	idsInMem := extractIDs(eventsInMem)
	idsPebble := extractIDs(eventsPebble)

	// Sort for comparison (order may differ)
	slices.Sort(idsInMem)
	slices.Sort(idsPebble)

	assert.Equal(t, idsInMem, idsPebble, "addressable query")
	assert.Len(t, idsInMem, 2, "should have two addressable events (one per d-tag)")
}

// =============================================================================
// StorageHandler-level Differential Testing
// =============================================================================

// TestStorageHandlerDifferential tests both storages through StorageHandler.
// This validates the full Nostr protocol flow: EVENT → OK, REQ → EVENT* + EOSE.
func TestStorageHandlerDifferential(t *testing.T) {
	seeds := []uint64{0, 1, 42, 12345, 98765}
	for _, seed := range seeds {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				testStorageHandlerDifferentialWithSeed(t, seed)
			})
		})
	}
}

func testStorageHandlerDifferentialWithSeed(t *testing.T, seed uint64) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create both storages
	inMemory := NewInMemoryStorage()
	pebbleStorage, err := NewPebbleStorageWithFS("test", vfs.NewMem())
	require.NoError(t, err)
	defer pebbleStorage.Close()

	// Create handlers
	handlerInMem := NewStorageHandler(inMemory)
	handlerPebble := NewStorageHandler(pebbleStorage)

	// Create channels for both handlers
	sendInMem := make(chan *ServerMsg, 100)
	recvInMem := make(chan *ClientMsg, 100)
	sendPebble := make(chan *ServerMsg, 100)
	recvPebble := make(chan *ClientMsg, 100)

	// Start both handlers
	go handlerInMem.ServeNostr(ctx, sendInMem, recvInMem)
	go handlerPebble.ServeNostr(ctx, sendPebble, recvPebble)

	// Use deterministic random generator
	rng := rand.New(rand.NewPCG(seed, seed))

	// Generate random events
	numEvents := 50
	events := generateRandomEvents(rng, numEvents)

	// Send EVENT messages and compare OK responses
	for i, event := range events {
		msg := &ClientMsg{Type: MsgTypeEvent, Event: event}
		recvInMem <- msg
		recvPebble <- msg

		synctest.Wait()

		// Collect OK responses
		okInMem := drainOKs(sendInMem)
		okPebble := drainOKs(sendPebble)

		require.Len(t, okInMem, 1, "event %d: expected 1 OK from inMem", i)
		require.Len(t, okPebble, 1, "event %d: expected 1 OK from pebble", i)

		assert.Equal(t, okInMem[0].Accepted, okPebble[0].Accepted,
			"event %d: Accepted mismatch", i)
		assert.Equal(t, okInMem[0].EventID, okPebble[0].EventID,
			"event %d: EventID mismatch", i)
	}

	// Send REQ messages and compare results
	numQueries := 20
	for i := range numQueries {
		filters := generateRandomFilters(rng, events)
		subID := fmt.Sprintf("sub-%d", i)

		msg := &ClientMsg{Type: MsgTypeReq, SubscriptionID: subID, Filters: filters}
		recvInMem <- msg
		recvPebble <- msg

		synctest.Wait()

		// Collect responses until EOSE
		eventsInMem, eoseInMem := drainEventsUntilEOSE(sendInMem, subID)
		eventsPebble, eosePebble := drainEventsUntilEOSE(sendPebble, subID)

		require.True(t, eoseInMem, "query %d: expected EOSE from inMem", i)
		require.True(t, eosePebble, "query %d: expected EOSE from pebble", i)

		// Compare event IDs
		idsInMem := extractEventIDs(eventsInMem)
		idsPebble := extractEventIDs(eventsPebble)

		assert.Equal(t, idsInMem, idsPebble,
			"query %d: results mismatch\nfilters: %+v\ninMem: %v\npebble: %v",
			i, filters, idsInMem, idsPebble)
	}
}

// drainOKs collects all OK messages from a channel.
func drainOKs(ch <-chan *ServerMsg) []*ServerMsg {
	var oks []*ServerMsg
	for {
		select {
		case msg := <-ch:
			if msg.Type == MsgTypeOK {
				oks = append(oks, msg)
			}
		default:
			return oks
		}
	}
}

// drainEventsUntilEOSE collects EVENT messages until EOSE.
func drainEventsUntilEOSE(ch <-chan *ServerMsg, subID string) ([]*ServerMsg, bool) {
	var events []*ServerMsg
	gotEOSE := false
	for {
		select {
		case msg := <-ch:
			switch msg.Type {
			case MsgTypeEvent:
				if msg.SubscriptionID == subID {
					events = append(events, msg)
				}
			case MsgTypeEOSE:
				if msg.SubscriptionID == subID {
					gotEOSE = true
					return events, gotEOSE
				}
			}
		default:
			return events, gotEOSE
		}
	}
}

// extractEventIDs extracts event IDs from ServerMsg slice.
func extractEventIDs(msgs []*ServerMsg) []string {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		if m.Event != nil {
			ids[i] = m.Event.ID
		}
	}
	return ids
}
