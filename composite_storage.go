//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"iter"
)

// CompositeStorage combines a primary Storage with a SearchIndex.
// The primary storage (e.g., PebbleStorage) is the source of truth.
// The search index (e.g., BleveIndex) provides full-text search capabilities.
//
// Query behavior:
//   - If filter has Search field: use SearchIndex to get event IDs, then fetch from primary
//   - Otherwise: use primary storage directly
//
// Store behavior:
//   - Store in primary first (source of truth)
//   - If successful, index in search index (best effort)
type CompositeStorage struct {
	primary Storage
	search  SearchIndex
}

// NewCompositeStorage creates a new CompositeStorage.
// primary is the source of truth for all data.
// search provides full-text search capabilities (can be nil to disable search).
func NewCompositeStorage(primary Storage, search SearchIndex) *CompositeStorage {
	return &CompositeStorage{
		primary: primary,
		search:  search,
	}
}

// Store implements Storage.Store.
// Stores in primary first, then indexes in search (if available).
func (s *CompositeStorage) Store(ctx context.Context, event *Event) (bool, error) {
	// Primary is source of truth
	stored, err := s.primary.Store(ctx, event)
	if err != nil || !stored {
		return stored, err
	}

	// Index in search (best effort - don't fail the store if search indexing fails)
	if s.search != nil && event != nil {
		// Only index non-ephemeral events with content
		if event.EventType() != EventTypeEphemeral && event.Content != "" {
			_ = s.search.Index(ctx, event) // Ignore errors - primary is truth
		}
	}

	return true, nil
}

// Query implements Storage.Query.
// If filter contains Search field, uses search index. Otherwise, uses primary.
func (s *CompositeStorage) Query(ctx context.Context, filters []*ReqFilter) (iter.Seq[*Event], func() error, func() error) {
	// Check if any filter has search field
	var searchQuery string
	for _, f := range filters {
		if f != nil && f.Search != nil && *f.Search != "" {
			searchQuery = *f.Search
			break
		}
	}

	// If no search query, delegate to primary
	if searchQuery == "" || s.search == nil {
		return s.primary.Query(ctx, filters)
	}

	// Get limit from first filter (NIP-01)
	limit := int64(100) // default
	if len(filters) > 0 && filters[0].Limit != nil {
		limit = *filters[0].Limit
	}

	// Search for event IDs
	eventIDs, err := s.search.Search(ctx, searchQuery, limit)
	if err != nil {
		// Fall back to primary on search error
		return s.primary.Query(ctx, filters)
	}

	// If no results, return empty iterator
	if len(eventIDs) == 0 {
		return func(yield func(*Event) bool) {}, func() error { return nil }, func() error { return nil }
	}

	// Create filter with found IDs and other constraints from original filters
	// This allows combining search with kinds, authors, etc.
	searchFilter := &ReqFilter{
		IDs:   eventIDs,
		Limit: &limit,
	}

	// Copy constraints from the original filter (if any)
	if len(filters) > 0 && filters[0] != nil {
		f := filters[0]
		if len(f.Kinds) > 0 {
			searchFilter.Kinds = f.Kinds
		}
		if len(f.Authors) > 0 {
			searchFilter.Authors = f.Authors
		}
		if f.Since != nil {
			searchFilter.Since = f.Since
		}
		if f.Until != nil {
			searchFilter.Until = f.Until
		}
		// Tags are intentionally not copied - search should match content
	}

	// Query primary storage with the IDs from search
	return s.primary.Query(ctx, []*ReqFilter{searchFilter})
}

// Delete removes an event from both primary and search index.
// This is called when processing kind 5 deletion requests.
func (s *CompositeStorage) Delete(ctx context.Context, eventID string) error {
	if s.search != nil {
		_ = s.search.Delete(ctx, eventID) // Best effort
	}
	return nil // Primary handles deletion through Store (kind 5 processing)
}

// Primary returns the underlying primary storage.
// Useful for operations that don't need search.
func (s *CompositeStorage) Primary() Storage {
	return s.primary
}

// Search returns the underlying search index (may be nil).
func (s *CompositeStorage) Search() SearchIndex {
	return s.search
}
