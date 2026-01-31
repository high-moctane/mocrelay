//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestBleveIndex_IndexAndSearch(t *testing.T) {
	ctx := context.Background()

	// Create in-memory index
	idx, err := NewBleveIndex(nil)
	if err != nil {
		t.Fatalf("NewBleveIndex failed: %v", err)
	}
	defer idx.Close()

	// Index some events
	events := []*Event{
		{ID: "event1", Pubkey: "pk1", Kind: 1, CreatedAt: time.Unix(1000, 0), Content: "今日は天気がいいですね"},
		{ID: "event2", Pubkey: "pk2", Kind: 1, CreatedAt: time.Unix(1001, 0), Content: "Nostrリレーを作っています"},
		{ID: "event3", Pubkey: "pk1", Kind: 1, CreatedAt: time.Unix(1002, 0), Content: "全文検索機能を実装したい"},
		{ID: "event4", Pubkey: "pk3", Kind: 1, CreatedAt: time.Unix(1003, 0), Content: "mocrelayは最高のリレーです"},
		{ID: "event5", Pubkey: "pk2", Kind: 1, CreatedAt: time.Unix(1004, 0), Content: "Hello, this is English text"},
	}

	for _, e := range events {
		if err := idx.Index(ctx, e); err != nil {
			t.Fatalf("Index failed for %s: %v", e.ID, err)
		}
	}

	// Verify doc count
	count, err := idx.DocCount()
	if err != nil {
		t.Fatalf("DocCount failed: %v", err)
	}
	if count != 5 {
		t.Errorf("DocCount = %d, want 5", count)
	}

	// Test searches
	tests := []struct {
		query   string
		limit   int64
		wantIDs []string // expected IDs (order may vary by score)
		wantMin int      // minimum expected results
	}{
		{query: "天気", limit: 10, wantIDs: []string{"event1"}, wantMin: 1},
		{query: "リレー", limit: 10, wantIDs: []string{"event2", "event4"}, wantMin: 2},
		{query: "mocrelay", limit: 10, wantIDs: []string{"event4"}, wantMin: 1},
		{query: "English", limit: 10, wantIDs: []string{"event5"}, wantMin: 1},
		{query: "検索", limit: 10, wantIDs: []string{"event3"}, wantMin: 1},
		{query: "存在しないワード", limit: 10, wantIDs: nil, wantMin: 0},
		{query: "", limit: 10, wantIDs: nil, wantMin: 0}, // empty query
	}

	for _, tt := range tests {
		t.Run("query="+tt.query, func(t *testing.T) {
			ids, err := idx.Search(ctx, tt.query, tt.limit)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(ids) < tt.wantMin {
				t.Errorf("Search(%q) returned %d results, want at least %d", tt.query, len(ids), tt.wantMin)
			}

			// Check that expected IDs are in results
			for _, wantID := range tt.wantIDs {
				found := slices.Contains(ids, wantID)
				if !found {
					t.Errorf("Search(%q) missing expected ID %s, got %v", tt.query, wantID, ids)
				}
			}
		})
	}
}

func TestBleveIndex_SearchLimit(t *testing.T) {
	ctx := context.Background()

	idx, err := NewBleveIndex(nil)
	if err != nil {
		t.Fatalf("NewBleveIndex failed: %v", err)
	}
	defer idx.Close()

	// Index 20 events with similar content
	for i := range 20 {
		e := &Event{
			ID:        eventID(i),
			Pubkey:    "pk1",
			Kind:      1,
			CreatedAt: time.Unix(int64(1000+i), 0),
			Content:   "プログラミングは楽しい",
		}
		if err := idx.Index(ctx, e); err != nil {
			t.Fatalf("Index failed: %v", err)
		}
	}

	// Search with limit
	ids, err := idx.Search(ctx, "プログラミング", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(ids) != 5 {
		t.Errorf("Search with limit 5 returned %d results, want 5", len(ids))
	}
}

func TestBleveIndex_Delete(t *testing.T) {
	ctx := context.Background()

	idx, err := NewBleveIndex(nil)
	if err != nil {
		t.Fatalf("NewBleveIndex failed: %v", err)
	}
	defer idx.Close()

	// Index an event
	e := &Event{
		ID:        "event1",
		Pubkey:    "pk1",
		Kind:      1,
		CreatedAt: time.Unix(1000, 0),
		Content:   "削除テスト用のイベント",
	}
	if err := idx.Index(ctx, e); err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	// Verify it's searchable
	ids, err := idx.Search(ctx, "削除テスト", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(ids) != 1 || ids[0] != "event1" {
		t.Fatalf("Expected event1 in search results, got %v", ids)
	}

	// Delete it
	if err := idx.Delete(ctx, "event1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's no longer searchable
	ids, err = idx.Search(ctx, "削除テスト", 10)
	if err != nil {
		t.Fatalf("Search after delete failed: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("Expected no results after delete, got %v", ids)
	}

	// Verify doc count
	count, err := idx.DocCount()
	if err != nil {
		t.Fatalf("DocCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("DocCount after delete = %d, want 0", count)
	}
}

func TestBleveIndex_NilEvent(t *testing.T) {
	ctx := context.Background()

	idx, err := NewBleveIndex(nil)
	if err != nil {
		t.Fatalf("NewBleveIndex failed: %v", err)
	}
	defer idx.Close()

	// Index nil event should not error
	if err := idx.Index(ctx, nil); err != nil {
		t.Errorf("Index(nil) returned error: %v", err)
	}

	count, _ := idx.DocCount()
	if count != 0 {
		t.Errorf("DocCount after nil index = %d, want 0", count)
	}
}

// eventID generates a test event ID.
func eventID(n int) string {
	// Generate a 64-char hex string
	id := make([]byte, 64)
	for i := range id {
		id[i] = '0'
	}
	s := []byte("event")
	copy(id, s)
	id[len(s)] = byte('0' + n%10)
	if n >= 10 {
		id[len(s)+1] = byte('0' + (n/10)%10)
	}
	return string(id)
}
