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

	// Check if deleted by event ID
	if rec, err := s.getDeletionRecord(prefixDeletedID, eventIDBytes); err != nil {
		return false, err
	} else if rec != nil && rec.pubkey == event.Pubkey {
		return false, nil // Already deleted
	}

	eventType := event.EventType()
	if eventType == EventTypeEphemeral {
		return false, nil
	}

	// Check if address was deleted (for replaceable/addressable)
	addr := event.Address()
	if addr != "" {
		addrHash := hashAddress(addr)
		if rec, err := s.getDeletionRecord(prefixDeletedAddr, addrHash); err != nil {
			return false, err
		} else if rec != nil && rec.pubkey == event.Pubkey {
			// Only reject if event.CreatedAt <= deletion request's CreatedAt
			if event.CreatedAt.Unix() <= rec.createdAt {
				return false, nil
			}
		}
	}

	// Handle kind 5 (deletion request)
	if event.Kind == 5 {
		if err := s.processKind5(event); err != nil {
			return false, err
		}
	}

	// Handle replaceable/addressable events
	var oldEventID []byte
	if eventType == EventTypeReplaceable || eventType == EventTypeAddressable {
		addr := event.Address()
		addrHash := hashAddress(addr)
		addrKey := makeAddrKey(addrHash)

		// Check if an event with this address already exists
		existingID, closer, err := s.db.Get(addrKey)
		if err == nil {
			defer closer.Close()
			// Get the existing event to compare timestamps
			existingEvent, err := s.getEventByID(existingID)
			if err != nil {
				return false, err
			}
			if existingEvent != nil {
				// Compare: keep newer, or if same timestamp, keep smaller ID
				if event.CreatedAt.Before(existingEvent.CreatedAt) {
					return false, nil // New event is older, reject
				}
				if event.CreatedAt.Equal(existingEvent.CreatedAt) && event.ID >= existingEvent.ID {
					return false, nil // Same timestamp but new ID is not smaller, reject
				}
				// New event wins, need to delete the old one
				oldEventID = existingID
			}
		} else if err != pebble.ErrNotFound {
			return false, err
		}
	}

	// Serialize event
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return false, err
	}

	// Create batch for atomic write
	batch := s.db.NewBatch()
	defer batch.Close()

	// Delete old event if replacing
	if oldEventID != nil {
		if err := s.deleteEventFromBatch(batch, oldEventID); err != nil {
			return false, err
		}
	}

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

	// Address index for replaceable/addressable
	if eventType == EventTypeReplaceable || eventType == EventTypeAddressable {
		addr := event.Address()
		addrHash := hashAddress(addr)
		addrKey := makeAddrKey(addrHash)
		if err := batch.Set(addrKey, eventIDBytes, pebble.Sync); err != nil {
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

	seen := make(map[string]bool)
	var result []*Event

	for _, filter := range filters {
		count := int64(0)
		limit := int64(-1)
		if filter.Limit != nil {
			limit = *filter.Limit
		}

		// Select the best index based on filter conditions
		selection := s.selectIndex(filter)

		iter, err := s.db.NewIter(&pebble.IterOptions{
			LowerBound: selection.lowerBound,
			UpperBound: selection.upperBound,
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
			eventID := key[selection.eventIDOffset : selection.eventIDOffset+32]

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

// indexSelection holds information about which index to use for a query.
type indexSelection struct {
	lowerBound    []byte
	upperBound    []byte
	eventIDOffset int // position of event ID in the key
}

// selectIndex chooses the best index based on filter conditions.
// Priority: kinds (single) > created_at (fallback)
func (s *PebbleStorage) selectIndex(filter *ReqFilter) *indexSelection {
	// Single kind: use kind index
	if len(filter.Kinds) == 1 {
		kind := filter.Kinds[0]
		// Key format: [0x04][kind:8][inverted_ts:8][id:32]
		lower := make([]byte, 9)
		lower[0] = prefixKind
		binary.BigEndian.PutUint64(lower[1:9], uint64(kind))

		upper := make([]byte, 9)
		upper[0] = prefixKind
		binary.BigEndian.PutUint64(upper[1:9], uint64(kind+1))

		return &indexSelection{
			lowerBound:    lower,
			upperBound:    upper,
			eventIDOffset: 17, // 1 + 8 + 8
		}
	}

	// Fallback: created_at index (full scan)
	return &indexSelection{
		lowerBound:    []byte{prefixCreatedAt},
		upperBound:    []byte{prefixCreatedAt + 1},
		eventIDOffset: 9, // 1 + 8
	}
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

// deletionRecordData holds information about a deletion request.
type deletionRecordData struct {
	pubkey    string
	createdAt int64 // Unix timestamp
}

// getDeletionRecord retrieves a deletion record by prefix and key.
func (s *PebbleStorage) getDeletionRecord(prefix byte, keyData []byte) (*deletionRecordData, error) {
	key := makeDeletionKey(prefix, keyData)
	value, closer, err := s.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	// Value format: [pubkey:32][created_at:8]
	if len(value) != 40 {
		return nil, nil
	}
	return &deletionRecordData{
		pubkey:    bytesToHex(value[:32]),
		createdAt: int64(binary.BigEndian.Uint64(value[32:40])),
	}, nil
}

// processKind5 handles deletion requests.
func (s *PebbleStorage) processKind5(event *Event) error {
	batch := s.db.NewBatch()
	defer batch.Close()

	pubkeyBytes := hexToBytes32(event.Pubkey)
	createdAtBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(createdAtBytes, uint64(event.CreatedAt.Unix()))

	// Value format: [pubkey:32][created_at:8]
	value := make([]byte, 40)
	copy(value[:32], pubkeyBytes)
	copy(value[32:], createdAtBytes)

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "e":
			// Delete by event ID
			eventID := tag[1]
			if len(eventID) != 64 {
				continue
			}
			eventIDBytes := hexToBytes32(eventID)

			// Record the deletion
			delKey := makeDeletionKey(prefixDeletedID, eventIDBytes)
			if err := batch.Set(delKey, value, pebble.Sync); err != nil {
				return err
			}

			// Get the event to check if it should be deleted
			targetEvent, err := s.getEventByID(eventIDBytes)
			if err != nil {
				return err
			}
			if targetEvent != nil && targetEvent.Pubkey == event.Pubkey && targetEvent.Kind != 5 {
				// Delete the event
				if err := s.deleteEventFromBatch(batch, eventIDBytes); err != nil {
					return err
				}
			}

		case "a":
			// Delete by address
			addr := tag[1]
			addrHash := hashAddress(addr)

			// Record the deletion
			delKey := makeDeletionKey(prefixDeletedAddr, addrHash)
			if err := batch.Set(delKey, value, pebble.Sync); err != nil {
				return err
			}

			// Find and delete the event with this address
			addrKey := makeAddrKey(addrHash)
			existingID, closer, err := s.db.Get(addrKey)
			if err == nil {
				closer.Close()
				targetEvent, err := s.getEventByID(existingID)
				if err != nil {
					return err
				}
				if targetEvent != nil && targetEvent.Pubkey == event.Pubkey && !targetEvent.CreatedAt.After(event.CreatedAt) {
					// Delete the event
					if err := s.deleteEventFromBatch(batch, existingID); err != nil {
						return err
					}
					// Also delete the addr index
					if err := batch.Delete(addrKey, pebble.Sync); err != nil {
						return err
					}
				}
			} else if err != pebble.ErrNotFound {
				return err
			}
		}
	}

	return batch.Commit(pebble.Sync)
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

// deleteEventFromBatch deletes an event and all its indexes from the batch.
func (s *PebbleStorage) deleteEventFromBatch(batch *pebble.Batch, eventID []byte) error {
	// Get the event to know its indexes
	event, err := s.getEventByID(eventID)
	if err != nil {
		return err
	}
	if event == nil {
		return nil // Already deleted
	}

	invertedTS := invertTimestamp(event.CreatedAt.Unix())
	pubkeyBytes := hexToBytes32(event.Pubkey)

	// Delete main event data
	eventKey := makeEventKey(eventID)
	if err := batch.Delete(eventKey, pebble.Sync); err != nil {
		return err
	}

	// Delete created_at index
	createdAtKey := makeCreatedAtKey(invertedTS, eventID)
	if err := batch.Delete(createdAtKey, pebble.Sync); err != nil {
		return err
	}

	// Delete pubkey index
	pubkeyKey := makePubkeyKey(pubkeyBytes, invertedTS, eventID)
	if err := batch.Delete(pubkeyKey, pebble.Sync); err != nil {
		return err
	}

	// Delete kind index
	kindKey := makeKindKey(event.Kind, invertedTS, eventID)
	if err := batch.Delete(kindKey, pebble.Sync); err != nil {
		return err
	}

	// Delete tag indexes
	for _, tag := range event.Tags {
		if len(tag) < 2 || len(tag[0]) != 1 {
			continue
		}
		tagName := tag[0][0]
		tagHash := hashTagValue(tag[1])
		tagKey := makeTagKey(tagName, tagHash, invertedTS, eventID)
		if err := batch.Delete(tagKey, pebble.Sync); err != nil {
			return err
		}
	}

	// Note: We don't delete the addr index here because the caller will overwrite it

	return nil
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

func makeAddrKey(addrHash []byte) []byte {
	key := make([]byte, 33)
	key[0] = prefixAddr
	copy(key[1:], addrHash)
	return key
}

func hashAddress(addr string) []byte {
	h := sha256.Sum256([]byte(addr))
	return h[:]
}

func makeDeletionKey(prefix byte, keyData []byte) []byte {
	key := make([]byte, 33)
	key[0] = prefix
	copy(key[1:], keyData)
	return key
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
