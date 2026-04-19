package mocrelay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestCompositeStorage_StoreAndQuery(t *testing.T) {
	ctx := context.Background()

	// Create primary (InMemory) and search (Bleve)
	primary := NewInMemoryStorage()
	search := setupBleveIndex(t)

	storage := NewCompositeStorage(primary, search, nil)

	// Store some events
	events := []*Event{
		{ID: id64("event1"), Pubkey: pk64("pk1"), Kind: 1, CreatedAt: time.Unix(1000, 0), Content: "今日は天気がいいですね"},
		{ID: id64("event2"), Pubkey: pk64("pk2"), Kind: 1, CreatedAt: time.Unix(1001, 0), Content: "Nostrリレーを作っています"},
		{ID: id64("event3"), Pubkey: pk64("pk1"), Kind: 1, CreatedAt: time.Unix(1002, 0), Content: "全文検索機能を実装したい"},
		{ID: id64("event4"), Pubkey: pk64("pk3"), Kind: 1, CreatedAt: time.Unix(1003, 0), Content: "mocrelayは最高のリレーです"},
		{ID: id64("event5"), Pubkey: pk64("pk2"), Kind: 1, CreatedAt: time.Unix(1004, 0), Content: "Hello, this is English text"},
	}

	for _, e := range events {
		stored, err := storage.Store(ctx, e)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
		if !stored {
			t.Errorf("Event %s was not stored", e.ID)
		}
	}

	// Test regular query (no search)
	t.Run("regular query", func(t *testing.T) {
		limit := int64(10)
		filters := []*ReqFilter{{Limit: &limit}}

		evts, errFn, closeFn := storage.Query(ctx, filters)
		defer closeFn()

		var count int
		for range evts {
			count++
		}
		if err := errFn(); err != nil {
			t.Fatalf("Query error: %v", err)
		}
		if count != 5 {
			t.Errorf("Regular query returned %d events, want 5", count)
		}
	})

	// Test search query
	t.Run("search query", func(t *testing.T) {
		searchStr := "リレー"
		limit := int64(10)
		filters := []*ReqFilter{{Search: &searchStr, Limit: &limit}}

		evts, errFn, closeFn := storage.Query(ctx, filters)
		defer closeFn()

		var results []*Event
		for e := range evts {
			results = append(results, e)
		}
		if err := errFn(); err != nil {
			t.Fatalf("Query error: %v", err)
		}

		// Should find event2 and event4 (both contain "リレー")
		if len(results) < 2 {
			t.Errorf("Search query returned %d events, want at least 2", len(results))
		}

		// Verify contents
		foundEvent2, foundEvent4 := false, false
		for _, e := range results {
			if e.ID == id64("event2") {
				foundEvent2 = true
			}
			if e.ID == id64("event4") {
				foundEvent4 = true
			}
		}
		if !foundEvent2 || !foundEvent4 {
			t.Errorf("Search missing expected events: event2=%v, event4=%v", foundEvent2, foundEvent4)
		}
	})

	// Test search with kind filter
	t.Run("search with kind filter", func(t *testing.T) {
		searchStr := "天気"
		limit := int64(10)
		filters := []*ReqFilter{{
			Search: &searchStr,
			Kinds:  []int64{1},
			Limit:  &limit,
		}}

		evts, errFn, closeFn := storage.Query(ctx, filters)
		defer closeFn()

		var count int
		for range evts {
			count++
		}
		if err := errFn(); err != nil {
			t.Fatalf("Query error: %v", err)
		}
		if count == 0 {
			t.Error("Search with kind filter returned 0 events")
		}
	})

	// Test empty search results
	t.Run("empty search results", func(t *testing.T) {
		searchStr := "存在しないワード12345"
		limit := int64(10)
		filters := []*ReqFilter{{Search: &searchStr, Limit: &limit}}

		evts, errFn, closeFn := storage.Query(ctx, filters)
		defer closeFn()

		var count int
		for range evts {
			count++
		}
		if err := errFn(); err != nil {
			t.Fatalf("Query error: %v", err)
		}
		if count != 0 {
			t.Errorf("Empty search returned %d events, want 0", count)
		}
	})
}

func TestCompositeStorage_NoSearchIndex(t *testing.T) {
	ctx := context.Background()

	// Create composite without search index
	primary := NewInMemoryStorage()
	storage := NewCompositeStorage(primary, nil, nil)

	// Store an event
	event := &Event{
		ID:        id64("event1"),
		Pubkey:    pk64("pk1"),
		Kind:      1,
		CreatedAt: time.Unix(1000, 0),
		Content:   "テスト",
	}

	stored, err := storage.Store(ctx, event)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if !stored {
		t.Error("Event was not stored")
	}

	// Search query should fall back to primary (which doesn't support search, so returns all)
	searchStr := "テスト"
	limit := int64(10)
	filters := []*ReqFilter{{Search: &searchStr, Limit: &limit}}

	evts, errFn, closeFn := storage.Query(ctx, filters)
	defer closeFn()

	var count int
	for range evts {
		count++
	}
	if err := errFn(); err != nil {
		t.Fatalf("Query error: %v", err)
	}

	// Falls back to primary, which returns all events
	if count != 1 {
		t.Errorf("Query returned %d events, want 1", count)
	}
}

func TestCompositeStorage_EphemeralNotIndexed(t *testing.T) {
	ctx := context.Background()

	primary := NewInMemoryStorage()
	search := setupBleveIndex(t)

	storage := NewCompositeStorage(primary, search, nil)

	// Store ephemeral event (kind 20000-29999)
	event := &Event{
		ID:        id64("ephemeral1"),
		Pubkey:    pk64("pk1"),
		Kind:      20001, // ephemeral
		CreatedAt: time.Unix(1000, 0),
		Content:   "エフェメラルイベント",
	}

	stored, err := storage.Store(ctx, event)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	// Ephemeral events are not stored in primary
	if stored {
		t.Error("Ephemeral event should not be stored")
	}

	// Search index should be empty
	count, _ := search.docCount()
	if count != 0 {
		t.Errorf("Search index has %d docs, want 0 (ephemeral not indexed)", count)
	}
}

// id64 creates a 64-char hex ID for testing
func id64(prefix string) string {
	id := make([]byte, 64)
	for i := range id {
		id[i] = '0'
	}
	copy(id, prefix)
	return string(id)
}

// pk64 creates a 64-char hex pubkey for testing
func pk64(prefix string) string {
	return id64(prefix)
}

// fakeSearchIndex is a [SearchIndex] double that lets tests trigger
// controlled failures without standing up Bleve.
type fakeSearchIndex struct {
	indexErr     error
	searchErr    error
	searchResult []string
}

func (f *fakeSearchIndex) Index(ctx context.Context, event *Event) error {
	return f.indexErr
}

func (f *fakeSearchIndex) Search(ctx context.Context, query string, limit int64) ([]string, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchResult, nil
}

func (f *fakeSearchIndex) Delete(ctx context.Context, eventID string) error {
	return nil
}

// TestCompositeStorage_MetricsSearch asserts SearchTotal increments on
// every search-backend call and SearchErrors increments (plus fallback to
// primary fires) when the backend fails.
func TestCompositeStorage_MetricsSearch(t *testing.T) {
	ctx := context.Background()
	primary := NewInMemoryStorage()
	search := &fakeSearchIndex{}

	reg := prometheus.NewRegistry()
	metrics := NewCompositeStorageMetrics(reg)

	storage := NewCompositeStorage(primary, search, &CompositeStorageOptions{Metrics: metrics})

	// Prime primary with one event so the fallback path returns something.
	event := &Event{
		ID:        id64("event1"),
		Pubkey:    pk64("pk1"),
		Kind:      1,
		CreatedAt: time.Unix(1000, 0),
		Content:   "hello",
	}
	if _, err := storage.Store(ctx, event); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Store triggered exactly one Index attempt (non-ephemeral, non-empty content).
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.IndexTotal))
	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.IndexErrors))

	searchStr := "hello"
	limit := int64(10)
	filters := []*ReqFilter{{Search: &searchStr, Limit: &limit}}

	// Case 1: search succeeds.
	search.searchResult = []string{event.ID}
	search.searchErr = nil

	evts, errFn, closeFn := storage.Query(ctx, filters)
	for range evts {
	}
	if err := errFn(); err != nil {
		t.Fatalf("Query errFn: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("Query closeFn: %v", err)
	}
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.SearchTotal))
	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.SearchErrors))

	// Case 2: search errors out; Query falls back to primary.
	search.searchResult = nil
	search.searchErr = errors.New("bleve kaboom")

	evts, errFn, closeFn = storage.Query(ctx, filters)
	var fallbackCount int
	for range evts {
		fallbackCount++
	}
	if err := errFn(); err != nil {
		t.Fatalf("Query errFn (fallback): %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("Query closeFn (fallback): %v", err)
	}
	assert.Equal(t, float64(2), testutil.ToFloat64(metrics.SearchTotal))
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.SearchErrors))
	assert.Equal(t, 1, fallbackCount,
		"fallback should delegate to primary, which returns the stored event")
}

// TestCompositeStorage_MetricsIndex asserts IndexTotal / IndexErrors
// increment on Store, and ephemeral events never trigger indexing.
func TestCompositeStorage_MetricsIndex(t *testing.T) {
	ctx := context.Background()
	primary := NewInMemoryStorage()
	search := &fakeSearchIndex{indexErr: errors.New("index down")}

	reg := prometheus.NewRegistry()
	metrics := NewCompositeStorageMetrics(reg)

	storage := NewCompositeStorage(primary, search, &CompositeStorageOptions{Metrics: metrics})

	// Store: primary succeeds, index fails. Store returns success
	// because indexing is best-effort.
	event := &Event{
		ID:        id64("event1"),
		Pubkey:    pk64("pk1"),
		Kind:      1,
		CreatedAt: time.Unix(1000, 0),
		Content:   "hello",
	}
	stored, err := storage.Store(ctx, event)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	assert.True(t, stored, "primary should have stored the event even if indexing fails")

	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.IndexTotal))
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.IndexErrors))

	// Ephemeral events bypass indexing entirely.
	eph := &Event{
		ID:        id64("eph1"),
		Pubkey:    pk64("pk2"),
		Kind:      20001,
		CreatedAt: time.Unix(1001, 0),
		Content:   "ephemeral",
	}
	if _, err := storage.Store(ctx, eph); err != nil {
		t.Fatalf("Store ephemeral: %v", err)
	}
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.IndexTotal),
		"ephemeral events must not trigger indexing")

	// Events with empty content also bypass indexing.
	empty := &Event{
		ID:        id64("empty1"),
		Pubkey:    pk64("pk3"),
		Kind:      1,
		CreatedAt: time.Unix(1002, 0),
		Content:   "",
	}
	if _, err := storage.Store(ctx, empty); err != nil {
		t.Fatalf("Store empty: %v", err)
	}
	assert.Equal(t, float64(1), testutil.ToFloat64(metrics.IndexTotal),
		"empty-content events must not trigger indexing")
}
