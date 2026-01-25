//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"sync"

	"github.com/cockroachdb/pebble"
)

// Key prefixes for Pebble storage
const (
	// Main data: [0x01][event_id:32] -> event_json
	prefixEvent byte = 0x01

	// Indexes (value is empty):
	// [0x02][inverted_ts:8][id:32]
	prefixCreatedAt byte = 0x02
	// [0x03][pubkey:32][inverted_ts:8][id:32]
	prefixPubkey byte = 0x03
	// [0x04][kind:8][inverted_ts:8][id:32]
	prefixKind byte = 0x04
	// [0x05][tag_name:1][tag_hash:32][inverted_ts:8][id:32]
	prefixTag byte = 0x05

	// Replaceable/Addressable: [0x06][addr_hash:32] -> [event_id:32]
	prefixAddr byte = 0x06

	// Deletion markers:
	// [0x08][event_id:32] -> [pubkey:32][created_at:8]
	prefixDeletedID byte = 0x08
	// [0x09][addr_hash:32] -> [pubkey:32][created_at:8]
	prefixDeletedAddr byte = 0x09
)

// PebbleStorage implements Storage using Pebble (LSM-tree based KV store).
type PebbleStorage struct {
	db *pebble.DB
	mu sync.RWMutex // For atomic operations spanning multiple keys
}

// NewPebbleStorage creates a new Pebble-backed storage.
// path is the directory where Pebble will store its data.
func NewPebbleStorage(path string) (*PebbleStorage, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, err
	}
	return &PebbleStorage{db: db}, nil
}

// Close closes the Pebble database.
func (s *PebbleStorage) Close() error {
	return s.db.Close()
}

// Store implements Storage.Store.
func (s *PebbleStorage) Store(ctx context.Context, event *Event) (bool, error) {
	if event == nil {
		return false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already exists
	eventIDBytes := hexToBytes32(event.ID)
	eventKey := makeEventKey(eventIDBytes)
	_, closer, err := s.db.Get(eventKey)
	if err == nil {
		closer.Close()
		return false, nil // Duplicate
	}
	if err != pebble.ErrNotFound {
		return false, err
	}

	// TODO: Check deletion markers
	// TODO: Handle kind 5
	// TODO: Handle replaceable/addressable

	// For now, just store regular events
	eventType := event.EventType()
	if eventType == EventTypeEphemeral {
		return false, nil
	}

	// Serialize event
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return false, err
	}

	// Create batch for atomic write
	batch := s.db.NewBatch()
	defer batch.Close()

	// Write main event data
	if err := batch.Set(eventKey, eventJSON, pebble.Sync); err != nil {
		return false, err
	}

	// Write indexes
	invertedTS := invertTimestamp(event.CreatedAt.Unix())
	pubkeyBytes := hexToBytes32(event.Pubkey)

	// Created at index
	createdAtKey := makeCreatedAtKey(invertedTS, eventIDBytes)
	if err := batch.Set(createdAtKey, nil, pebble.Sync); err != nil {
		return false, err
	}

	// Pubkey index
	pubkeyKey := makePubkeyKey(pubkeyBytes, invertedTS, eventIDBytes)
	if err := batch.Set(pubkeyKey, nil, pebble.Sync); err != nil {
		return false, err
	}

	// Kind index
	kindKey := makeKindKey(event.Kind, invertedTS, eventIDBytes)
	if err := batch.Set(kindKey, nil, pebble.Sync); err != nil {
		return false, err
	}

	// Tag indexes (single-letter tags only)
	for _, tag := range event.Tags {
		if len(tag) < 2 || len(tag[0]) != 1 {
			continue
		}
		tagName := tag[0][0]
		tagHash := hashTagValue(tag[1])
		tagKey := makeTagKey(tagName, tagHash, invertedTS, eventIDBytes)
		if err := batch.Set(tagKey, nil, pebble.Sync); err != nil {
			return false, err
		}
	}

	// Commit batch
	if err := batch.Commit(pebble.Sync); err != nil {
		return false, err
	}

	return true, nil
}

// Query implements Storage.Query.
func (s *PebbleStorage) Query(ctx context.Context, filters []*ReqFilter) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(filters) == 0 {
		return nil, nil
	}

	// For now, simple implementation: scan all events and filter in memory
	// TODO: Use indexes for efficient querying

	seen := make(map[string]bool)
	var result []*Event

	for _, filter := range filters {
		count := int64(0)
		limit := int64(-1)
		if filter.Limit != nil {
			limit = *filter.Limit
		}

		// Scan all events using created_at index (for proper ordering)
		iter, err := s.db.NewIter(&pebble.IterOptions{
			LowerBound: []byte{prefixCreatedAt},
			UpperBound: []byte{prefixCreatedAt + 1},
		})
		if err != nil {
			return nil, err
		}

		for iter.First(); iter.Valid(); iter.Next() {
			if limit >= 0 && count >= limit {
				break
			}

			// Extract event ID from key
			key := iter.Key()
			eventID := key[9:41] // [prefix:1][inverted_ts:8][event_id:32]

			// Skip if already seen
			eventIDHex := bytesToHex(eventID)
			if seen[eventIDHex] {
				continue
			}

			// Get the event
			event, err := s.getEventByID(eventID)
			if err != nil {
				iter.Close()
				return nil, err
			}
			if event == nil {
				continue
			}

			// Check if matches filter
			if filter.Match(event) {
				seen[eventIDHex] = true
				result = append(result, event)
				count++
			}
		}

		if err := iter.Close(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// Delete implements Storage.Delete.
func (s *PebbleStorage) Delete(ctx context.Context, eventID string, pubkey string) error {
	// TODO: Implement
	return nil
}

// DeleteByAddr implements Storage.DeleteByAddr.
func (s *PebbleStorage) DeleteByAddr(ctx context.Context, kind int64, pubkey, dTag string) error {
	// TODO: Implement
	return nil
}

// getEventByID retrieves an event by its ID bytes.
func (s *PebbleStorage) getEventByID(eventID []byte) (*Event, error) {
	eventKey := makeEventKey(eventID)
	value, closer, err := s.db.Get(eventKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var event Event
	if err := json.Unmarshal(value, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// Len returns the number of stored events.
func (s *PebbleStorage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte{prefixEvent},
		UpperBound: []byte{prefixEvent + 1},
	})
	if err != nil {
		return 0
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}
	return count
}

// Helper functions for key construction

func makeEventKey(eventID []byte) []byte {
	key := make([]byte, 33)
	key[0] = prefixEvent
	copy(key[1:], eventID)
	return key
}

func makeCreatedAtKey(invertedTS uint64, eventID []byte) []byte {
	key := make([]byte, 41)
	key[0] = prefixCreatedAt
	binary.BigEndian.PutUint64(key[1:9], invertedTS)
	copy(key[9:], eventID)
	return key
}

func makePubkeyKey(pubkey []byte, invertedTS uint64, eventID []byte) []byte {
	key := make([]byte, 73)
	key[0] = prefixPubkey
	copy(key[1:33], pubkey)
	binary.BigEndian.PutUint64(key[33:41], invertedTS)
	copy(key[41:], eventID)
	return key
}

func makeKindKey(kind int64, invertedTS uint64, eventID []byte) []byte {
	key := make([]byte, 49)
	key[0] = prefixKind
	binary.BigEndian.PutUint64(key[1:9], uint64(kind))
	binary.BigEndian.PutUint64(key[9:17], invertedTS)
	copy(key[17:], eventID)
	return key
}

func makeTagKey(tagName byte, tagHash []byte, invertedTS uint64, eventID []byte) []byte {
	key := make([]byte, 74)
	key[0] = prefixTag
	key[1] = tagName
	copy(key[2:34], tagHash)
	binary.BigEndian.PutUint64(key[34:42], invertedTS)
	copy(key[42:], eventID)
	return key
}

func invertTimestamp(ts int64) uint64 {
	return uint64(math.MaxInt64 - ts)
}

func hashTagValue(value string) []byte {
	h := sha256.Sum256([]byte(value))
	return h[:]
}

func hexToBytes32(hex string) []byte {
	// Simple hex decoder for 64-char hex strings
	b := make([]byte, 32)
	for i := range 32 {
		b[i] = hexByte(hex[i*2])<<4 | hexByte(hex[i*2+1])
	}
	return b
}

func hexByte(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

func bytesToHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	hex := make([]byte, len(b)*2)
	for i, v := range b {
		hex[i*2] = hexChars[v>>4]
		hex[i*2+1] = hexChars[v&0x0f]
	}
	return string(hex)
}
