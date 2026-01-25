//go:build goexperiment.jsonv2

package mocrelay

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"
)

// Storage defines the interface for event storage.
type Storage interface {
	// Store saves an event and returns whether it was actually stored.
	// Returns false for duplicates, older replaceable events, or deleted events.
	Store(ctx context.Context, event *Event) (stored bool, err error)

	// Query returns events matching any of the filters.
	// Results are sorted by created_at DESC, then id ASC (lexical).
	// Each filter's limit is applied independently, then results are merged.
	Query(ctx context.Context, filters []*ReqFilter) ([]*Event, error)

	// Delete removes an event by ID. Only the author (pubkey) can delete.
	Delete(ctx context.Context, eventID string, pubkey string) error

	// DeleteByAddr removes a replaceable/addressable event by its address.
	// Only the author (pubkey) can delete.
	DeleteByAddr(ctx context.Context, kind int64, pubkey, dTag string) error
}

// deletionRecord holds information about a deletion request.
type deletionRecord struct {
	Pubkey    string
	CreatedAt time.Time
}

// InMemoryStorage is a simple in-memory implementation of Storage.
// This is a "brute force" implementation: O(n) for most operations.
// Suitable for testing and small datasets.
type InMemoryStorage struct {
	mu sync.RWMutex

	// All events stored
	events []*Event

	// Deleted event IDs: eventID -> deletion record
	// Used to reject events that were already deleted
	deletedIDs map[string]*deletionRecord

	// Deleted addresses: address -> deletion record
	// Used to reject replaceable/addressable events that were already deleted
	// NIP-09: only delete versions up to the deletion request's created_at
	deletedAddrs map[string]*deletionRecord
}

// NewInMemoryStorage creates a new in-memory storage.
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		events:       make([]*Event, 0),
		deletedIDs:   make(map[string]*deletionRecord),
		deletedAddrs: make(map[string]*deletionRecord),
	}
}

// Store implements Storage.Store.
func (s *InMemoryStorage) Store(ctx context.Context, event *Event) (bool, error) {
	if event == nil {
		return false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already deleted (by same pubkey)
	if rec, ok := s.deletedIDs[event.ID]; ok && rec.Pubkey == event.Pubkey {
		return false, nil
	}

	// Check if address was deleted (for replaceable/addressable)
	// NIP-09: only reject if event was created before or at the deletion request time
	addr := event.Address()
	if addr != "" {
		if rec, ok := s.deletedAddrs[addr]; ok && rec.Pubkey == event.Pubkey {
			// Only reject if event.CreatedAt <= deletion request's CreatedAt
			if !event.CreatedAt.After(rec.CreatedAt) {
				return false, nil
			}
		}
	}

	// Check for duplicate ID
	for _, e := range s.events {
		if e.ID == event.ID {
			return false, nil
		}
	}

	// Handle kind 5 (deletion request)
	if event.Kind == 5 {
		s.processKind5(event)

		// Check if this kind 5 deleted itself
		if rec, ok := s.deletedIDs[event.ID]; ok && rec.Pubkey == event.Pubkey {
			// Processed but not stored (deleted itself)
			return true, nil
		}
	}

	// Handle replaceable/addressable events
	eventType := event.EventType()
	if eventType == EventTypeReplaceable || eventType == EventTypeAddressable {
		// Find existing event with same address
		for i, e := range s.events {
			if e.Address() == addr {
				// Keep the newer one
				if event.CreatedAt.After(e.CreatedAt) {
					// Replace with new event
					s.events[i] = event
					return true, nil
				} else if event.CreatedAt.Equal(e.CreatedAt) {
					// Same timestamp: keep lexically smaller ID
					if event.ID < e.ID {
						s.events[i] = event
						return true, nil
					}
				}
				// New event is older, don't store
				return false, nil
			}
		}
	}

	// Ephemeral events are not stored
	if eventType == EventTypeEphemeral {
		return false, nil
	}

	// Store the event
	s.events = append(s.events, event)
	return true, nil
}

// processKind5 handles deletion requests.
// Caller must hold the lock.
func (s *InMemoryStorage) processKind5(event *Event) {
	rec := &deletionRecord{
		Pubkey:    event.Pubkey,
		CreatedAt: event.CreatedAt,
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "e":
			// Delete by event ID
			eventID := tag[1]
			// Record the deletion (for rejecting future events)
			s.deletedIDs[eventID] = rec

			// Remove from storage if it exists and belongs to the same pubkey
			// NIP-09: "deletion request event against a deletion request has no effect"
			s.events = slices.DeleteFunc(s.events, func(e *Event) bool {
				if e.Kind == 5 {
					return false // Don't delete kind 5 events
				}
				return e.ID == eventID && e.Pubkey == event.Pubkey
			})

		case "a":
			// Delete by address (for replaceable/addressable)
			addr := tag[1]
			// Record the deletion with timestamp
			s.deletedAddrs[addr] = rec

			// Remove from storage if it exists, belongs to the same pubkey,
			// and was created before or at the deletion request time
			s.events = slices.DeleteFunc(s.events, func(e *Event) bool {
				return e.Address() == addr &&
					e.Pubkey == event.Pubkey &&
					!e.CreatedAt.After(event.CreatedAt)
			})
		}
	}
}

// Query implements Storage.Query.
func (s *InMemoryStorage) Query(ctx context.Context, filters []*ReqFilter) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(filters) == 0 {
		return nil, nil
	}

	// Collect matching events with deduplication
	seen := make(map[string]bool)
	var result []*Event

	for _, filter := range filters {
		// Count for this filter's limit
		count := int64(0)
		limit := int64(-1) // -1 means no limit
		if filter.Limit != nil {
			limit = *filter.Limit
		}

		// We need to iterate in sorted order to apply limit correctly
		// Sort a copy of events first
		sorted := s.sortedEvents()

		for _, e := range sorted {
			// Check limit
			if limit >= 0 && count >= limit {
				break
			}

			// Skip if already seen
			if seen[e.ID] {
				continue
			}

			// Check if matches filter
			if filter.Match(e) {
				seen[e.ID] = true
				result = append(result, e)
				count++
			}
		}
	}

	// Sort final result
	slices.SortFunc(result, compareEvents)

	return result, nil
}

// sortedEvents returns a sorted copy of events.
// Caller must hold at least read lock.
func (s *InMemoryStorage) sortedEvents() []*Event {
	sorted := make([]*Event, len(s.events))
	copy(sorted, s.events)
	slices.SortFunc(sorted, compareEvents)
	return sorted
}

// compareEvents compares two events for sorting.
// Primary: created_at DESC, Secondary: id ASC (lexical).
func compareEvents(a, b *Event) int {
	// created_at DESC (newer first)
	if !a.CreatedAt.Equal(b.CreatedAt) {
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		return 1
	}
	// id ASC (lexical)
	return cmp.Compare(a.ID, b.ID)
}

// Delete implements Storage.Delete.
func (s *InMemoryStorage) Delete(ctx context.Context, eventID string, pubkey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Record the deletion with far-future timestamp to block any version
	s.deletedIDs[eventID] = &deletionRecord{
		Pubkey:    pubkey,
		CreatedAt: time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
	}

	// Remove from storage if it exists and belongs to the same pubkey
	s.events = slices.DeleteFunc(s.events, func(e *Event) bool {
		return e.ID == eventID && e.Pubkey == pubkey
	})

	return nil
}

// DeleteByAddr implements Storage.DeleteByAddr.
func (s *InMemoryStorage) DeleteByAddr(ctx context.Context, kind int64, pubkey, dTag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build address
	var addr string
	if kind >= 30000 && kind < 40000 {
		// Addressable: kind:pubkey:d-tag
		addr = formatAddress(kind, pubkey, dTag)
	} else if kind == 0 || kind == 3 || (kind >= 10000 && kind < 20000) {
		// Replaceable: kind:pubkey:
		addr = formatAddress(kind, pubkey, "")
	} else {
		// Not replaceable/addressable, nothing to do
		return nil
	}

	// Record the deletion with far-future timestamp to block any version
	s.deletedAddrs[addr] = &deletionRecord{
		Pubkey:    pubkey,
		CreatedAt: time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
	}

	// Remove from storage
	s.events = slices.DeleteFunc(s.events, func(e *Event) bool {
		return e.Address() == addr && e.Pubkey == pubkey
	})

	return nil
}

// formatAddress builds an address string for replaceable/addressable events.
func formatAddress(kind int64, pubkey, dTag string) string {
	if dTag == "" {
		return formatAddressReplaceable(kind, pubkey)
	}
	return formatAddressAddressable(kind, pubkey, dTag)
}

func formatAddressReplaceable(kind int64, pubkey string) string {
	// kind:pubkey:
	return string(appendInt(nil, kind)) + ":" + pubkey + ":"
}

func formatAddressAddressable(kind int64, pubkey, dTag string) string {
	// kind:pubkey:d-tag
	return string(appendInt(nil, kind)) + ":" + pubkey + ":" + dTag
}

// appendInt appends an integer to a byte slice.
func appendInt(b []byte, n int64) []byte {
	if n == 0 {
		return append(b, '0')
	}
	if n < 0 {
		b = append(b, '-')
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return append(b, digits[i:]...)
}

// Len returns the number of stored events.
func (s *InMemoryStorage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// DeletedLen returns the number of deleted event IDs being tracked.
func (s *InMemoryStorage) DeletedLen() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.deletedIDs)
}
