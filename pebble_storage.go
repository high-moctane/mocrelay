package mocrelay

import (
	"bytes"
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"iter"
	"math"
	"sort"
	"strconv"
	"sync"

	"github.com/cockroachdb/pebble"
)

// Key prefixes for Pebble storage
const (
	// Main data: [0x01][event_id:32] -> event_json
	prefixEvent byte = 0x01

	// Index (value is empty):
	// [0x02][inverted_ts:8][id:32]
	prefixCreatedAt byte = 0x02

	// Replaceable/Addressable: [0x03][addr_hash:32] -> [event_id:32]
	prefixAddr byte = 0x03

	// Deletion markers:
	// [0x04][event_id:32] -> [pubkey:32][created_at:8]
	prefixDeletedID byte = 0x04
	// [0x05][addr_hash:32] -> [pubkey:32][created_at:8]
	prefixDeletedAddr byte = 0x05

	// Field hash index (value is empty):
	// [0x06][field_hash:8][inverted_ts:8][event_id:32]
	prefixField byte = 0x06
)

// PebbleStorage implements Storage using Pebble (LSM-tree based KV store).
//
// The *pebble.DB is owned by the caller: PebbleStorage does not open or
// close the database. The caller is responsible for opening the DB with
// whatever [pebble.Options] are appropriate (cache size, bloom filters,
// event listeners, etc.) and for closing it when done. This lets the
// caller expose [pebble.DB.Metrics] directly and pin their own Pebble
// version independent of mocrelay's go.mod.
type PebbleStorage struct {
	db *pebble.DB
	mu sync.RWMutex // For atomic operations spanning multiple keys
}

// PebbleStorageOptions is reserved for future PebbleStorage configuration.
// It is currently empty but exists so that adding options later is not a
// breaking API change. Pass nil for now.
type PebbleStorageOptions struct{}

// NewPebbleStorage wraps an already-open *pebble.DB as a Storage.
//
// The caller retains ownership of db: they are responsible for opening it
// (with their own [pebble.Options]) and for calling db.Close() when done.
// PebbleStorage itself has no Close method.
//
// opts can be nil; there are currently no configurable options.
func NewPebbleStorage(db *pebble.DB, opts *PebbleStorageOptions) *PebbleStorage {
	_ = opts // reserved for future use
	return &PebbleStorage{db: db}
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
		return false, fmt.Errorf("check duplicate: %w", err)
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
				return false, fmt.Errorf("get existing replaceable event: %w", err)
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
			return false, fmt.Errorf("check replaceable: %w", err)
		}
	}

	// Serialize event
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return false, fmt.Errorf("marshal event: %w", err)
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

	// Created at index
	createdAtKey := makeCreatedAtKey(invertedTS, eventIDBytes)
	if err := batch.Set(createdAtKey, nil, pebble.Sync); err != nil {
		return false, err
	}

	// Field hash indexes
	authorHash := fieldHashAuthor(event.Pubkey)
	if err := batch.Set(makeFieldKey(authorHash, invertedTS, eventIDBytes), nil, pebble.Sync); err != nil {
		return false, err
	}

	kindHash := fieldHashKind(event.Kind)
	if err := batch.Set(makeFieldKey(kindHash, invertedTS, eventIDBytes), nil, pebble.Sync); err != nil {
		return false, err
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 || len(tag[0]) != 1 {
			continue
		}
		tagHash := fieldHashTag(tag[0][0], tag[1])
		if err := batch.Set(makeFieldKey(tagHash, invertedTS, eventIDBytes), nil, pebble.Sync); err != nil {
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
		return false, fmt.Errorf("commit batch: %w", err)
	}

	return true, nil
}

// Query implements Storage.Query.
func (s *PebbleStorage) Query(ctx context.Context, filters []*ReqFilter) (iter.Seq[*Event], func() error, func() error) {
	// NIP-01: "only return events from the first filter's limit"
	limit := int64(-1)
	if len(filters) > 0 && filters[0].Limit != nil {
		limit = *filters[0].Limit
	}

	// Use Snapshot for consistent reads without blocking writes
	snapshot := s.db.NewSnapshot()

	// State captured by closures
	var iterators []*pebble.Iterator
	var queryErr error

	events := func(yield func(*Event) bool) {
		if len(filters) == 0 {
			return
		}

		// Build per-filter cursors and add to cross-filter heap (OR between filters)
		fh := &filterCursorHeap{}
		heap.Init(fh)

		for _, filter := range filters {
			cursor, err := s.buildFilterCursor(snapshot, filter, &iterators)
			if err != nil {
				queryErr = fmt.Errorf("build filter cursor: %w", err)
				return
			}
			if cursor != nil && cursor.Valid() {
				heap.Push(fh, &filterCursorPair{cursor: cursor, filter: filter})
			}
		}

		seen := make(map[string]bool)
		count := int64(0)

		// Merge across filters (OR) using heap
		for fh.Len() > 0 {
			if limit >= 0 && count >= limit {
				return
			}

			// Pop the entry with smallest (invertedTS, eventID) = newest event
			pair := heap.Pop(fh).(*filterCursorPair)
			eventIDHex := bytesToHex(pair.cursor.EventID())

			if !seen[eventIDHex] {
				event, err := s.getEventByIDFromSnapshot(snapshot, pair.cursor.EventID())
				if err != nil {
					queryErr = fmt.Errorf("query event: %w", err)
					return
				}

				// Match recheck (for FNV-1a collision, IDs filter, etc.)
				if event != nil && pair.filter.Match(event) {
					seen[eventIDHex] = true
					count++
					if !yield(event) {
						return
					}
				}
			}

			if pair.cursor.Next() {
				heap.Push(fh, pair)
			}
		}
	}

	errFn := func() error {
		return queryErr
	}

	closeFn := func() error {
		var closeErr error
		for _, iter := range iterators {
			if err := iter.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		if err := snapshot.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		return closeErr
	}

	return events, errFn, closeFn
}

// --- Query cursor abstraction ---
//
// queryCursor produces (invertedTS, eventID) pairs in sorted order.
// Three implementations form a composable tree:
//   - indexCursor:     single Pebble iterator (leaf)
//   - unionCursor:     heap-based OR merge
//   - intersectCursor: sort-merge join AND (with SeekGE optimization)

type queryCursor interface {
	Valid() bool
	InvertedTS() uint64
	EventID() []byte
	Next() bool
	SeekGE(invertedTS uint64, eventID []byte) bool
}

// indexCursor wraps a Pebble iterator as a queryCursor.
// Works with any key format that has inverted_ts and event_id at known offsets.
type indexCursor struct {
	iter            *pebble.Iterator
	seekPrefix      []byte // key bytes before inverted_ts (for constructing SeekGE keys)
	timestampOffset int
	eventIDOffset   int
	curInvertedTS   uint64
	curEventID      []byte
	curValid        bool
}

func newIndexCursor(iter *pebble.Iterator, seekPrefix []byte, timestampOffset, eventIDOffset int) *indexCursor {
	c := &indexCursor{
		iter:            iter,
		seekPrefix:      seekPrefix,
		timestampOffset: timestampOffset,
		eventIDOffset:   eventIDOffset,
	}
	if iter.First() {
		c.readCurrent()
	}
	return c
}

func (c *indexCursor) readCurrent() {
	key := c.iter.Key()
	c.curInvertedTS = binary.BigEndian.Uint64(key[c.timestampOffset : c.timestampOffset+8])
	c.curEventID = make([]byte, 32)
	copy(c.curEventID, key[c.eventIDOffset:c.eventIDOffset+32])
	c.curValid = true
}

func (c *indexCursor) Valid() bool        { return c.curValid }
func (c *indexCursor) InvertedTS() uint64 { return c.curInvertedTS }
func (c *indexCursor) EventID() []byte    { return c.curEventID }

func (c *indexCursor) Next() bool {
	if !c.iter.Next() {
		c.curValid = false
		return false
	}
	c.readCurrent()
	return true
}

func (c *indexCursor) SeekGE(invertedTS uint64, eventID []byte) bool {
	seekKey := make([]byte, len(c.seekPrefix)+8+32)
	copy(seekKey, c.seekPrefix)
	binary.BigEndian.PutUint64(seekKey[len(c.seekPrefix):len(c.seekPrefix)+8], invertedTS)
	copy(seekKey[len(c.seekPrefix)+8:], eventID)
	if !c.iter.SeekGE(seekKey) {
		c.curValid = false
		return false
	}
	c.readCurrent()
	return true
}

// unionCursor merges multiple queryCursors in sorted order (OR).
type unionCursor struct {
	h queryCursorHeap
}

func newUnionCursor(cursors []queryCursor) *unionCursor {
	h := make(queryCursorHeap, 0, len(cursors))
	for _, c := range cursors {
		if c.Valid() {
			h = append(h, c)
		}
	}
	heap.Init(&h)
	return &unionCursor{h: h}
}

func (c *unionCursor) Valid() bool        { return c.h.Len() > 0 }
func (c *unionCursor) InvertedTS() uint64 { return c.h[0].InvertedTS() }
func (c *unionCursor) EventID() []byte    { return c.h[0].EventID() }

func (c *unionCursor) Next() bool {
	if c.h.Len() == 0 {
		return false
	}
	if c.h[0].Next() {
		heap.Fix(&c.h, 0)
	} else {
		heap.Pop(&c.h)
	}
	return c.h.Len() > 0
}

func (c *unionCursor) SeekGE(invertedTS uint64, eventID []byte) bool {
	newH := make(queryCursorHeap, 0, len(c.h))
	for _, cur := range c.h {
		if cur.SeekGE(invertedTS, eventID) {
			newH = append(newH, cur)
		}
	}
	c.h = newH
	if len(c.h) > 0 {
		heap.Init(&c.h)
	}
	return c.h.Len() > 0
}

// queryCursorHeap implements heap.Interface for queryCursor.
// Sorted by (invertedTS ASC, eventID ASC) = (created_at DESC, id ASC).
type queryCursorHeap []queryCursor

func (h queryCursorHeap) Len() int { return len(h) }
func (h queryCursorHeap) Less(i, j int) bool {
	if h[i].InvertedTS() != h[j].InvertedTS() {
		return h[i].InvertedTS() < h[j].InvertedTS()
	}
	return bytesLess(h[i].EventID(), h[j].EventID())
}
func (h queryCursorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *queryCursorHeap) Push(x any)   { *h = append(*h, x.(queryCursor)) }
func (h *queryCursorHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return x
}

// intersectCursor produces the intersection of multiple queryCursors
// via sort-merge join with SeekGE optimization.
type intersectCursor struct {
	cursors []queryCursor
	valid   bool
}

func newIntersectCursor(cursors []queryCursor) *intersectCursor {
	c := &intersectCursor{cursors: cursors}
	c.findMatch()
	return c
}

// findMatch advances cursors until all point to the same (invertedTS, eventID).
func (c *intersectCursor) findMatch() {
	for {
		for _, cur := range c.cursors {
			if !cur.Valid() {
				c.valid = false
				return
			}
		}

		// Find the maximum (invertedTS, eventID) among all cursor heads.
		// This is the "slowest" cursor — all others must catch up.
		maxIdx := 0
		for i := 1; i < len(c.cursors); i++ {
			ci, cm := c.cursors[i], c.cursors[maxIdx]
			if ci.InvertedTS() > cm.InvertedTS() ||
				(ci.InvertedTS() == cm.InvertedTS() && bytesLess(cm.EventID(), ci.EventID())) {
				maxIdx = i
			}
		}

		maxTS := c.cursors[maxIdx].InvertedTS()
		maxID := c.cursors[maxIdx].EventID()

		// SeekGE all other cursors to at least (maxTS, maxID)
		allMatch := true
		for i, cur := range c.cursors {
			if i == maxIdx {
				continue
			}
			if cur.InvertedTS() == maxTS && bytes.Equal(cur.EventID(), maxID) {
				continue // already at the right position
			}
			if !cur.SeekGE(maxTS, maxID) {
				c.valid = false
				return
			}
			if cur.InvertedTS() != maxTS || !bytes.Equal(cur.EventID(), maxID) {
				allMatch = false
			}
		}

		if allMatch {
			c.valid = true
			return
		}
		// Some cursor jumped past max — loop again to find new max
	}
}

func (c *intersectCursor) Valid() bool        { return c.valid }
func (c *intersectCursor) InvertedTS() uint64 { return c.cursors[0].InvertedTS() }
func (c *intersectCursor) EventID() []byte    { return c.cursors[0].EventID() }

func (c *intersectCursor) Next() bool {
	if !c.valid {
		return false
	}
	// Advance the first cursor (all are at the same position)
	if !c.cursors[0].Next() {
		c.valid = false
		return false
	}
	c.findMatch()
	return c.valid
}

func (c *intersectCursor) SeekGE(invertedTS uint64, eventID []byte) bool {
	for _, cur := range c.cursors {
		if !cur.SeekGE(invertedTS, eventID) {
			c.valid = false
			return false
		}
	}
	c.findMatch()
	return c.valid
}

// sliceCursor is a queryCursor backed by a pre-sorted slice of (invertedTS, eventID) pairs.
// Used for IDs filter where events are fetched by direct Get and then sorted.
type sliceCursor struct {
	entries []sliceCursorEntry
	pos     int
}

type sliceCursorEntry struct {
	invertedTS uint64
	eventID    []byte
}

func newSliceCursor(entries []sliceCursorEntry) *sliceCursor {
	return &sliceCursor{entries: entries}
}

func (c *sliceCursor) Valid() bool        { return c.pos < len(c.entries) }
func (c *sliceCursor) InvertedTS() uint64 { return c.entries[c.pos].invertedTS }
func (c *sliceCursor) EventID() []byte    { return c.entries[c.pos].eventID }

func (c *sliceCursor) Next() bool {
	c.pos++
	return c.pos < len(c.entries)
}

func (c *sliceCursor) SeekGE(invertedTS uint64, eventID []byte) bool {
	remaining := c.entries[c.pos:]
	idx := sort.Search(len(remaining), func(i int) bool {
		e := remaining[i]
		if e.invertedTS != invertedTS {
			return e.invertedTS >= invertedTS
		}
		return bytes.Compare(e.eventID, eventID) >= 0
	})
	c.pos += idx
	return c.pos < len(c.entries)
}

// filterCursorPair associates a queryCursor with its filter for Match checking.
type filterCursorPair struct {
	cursor queryCursor
	filter *ReqFilter
}

// filterCursorHeap implements heap.Interface for cross-filter merge (OR).
type filterCursorHeap []*filterCursorPair

func (h filterCursorHeap) Len() int { return len(h) }
func (h filterCursorHeap) Less(i, j int) bool {
	if h[i].cursor.InvertedTS() != h[j].cursor.InvertedTS() {
		return h[i].cursor.InvertedTS() < h[j].cursor.InvertedTS()
	}
	return bytesLess(h[i].cursor.EventID(), h[j].cursor.EventID())
}
func (h filterCursorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *filterCursorHeap) Push(x any)   { *h = append(*h, x.(*filterCursorPair)) }
func (h *filterCursorHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return x
}

// --- Filter cursor builder ---

// buildIDsCursor creates a sliceCursor by directly fetching events by their IDs.
// This is the most efficient path for IDs filter — O(1) per event via Pebble Get.
func (s *PebbleStorage) buildIDsCursor(snapshot *pebble.Snapshot, ids []string) (queryCursor, error) {
	var entries []sliceCursorEntry

	for _, id := range ids {
		if len(id) != 64 {
			continue
		}
		eventIDBytes := hexToBytes32(id)
		event, err := s.getEventByIDFromSnapshot(snapshot, eventIDBytes)
		if err != nil {
			return nil, fmt.Errorf("get event by id: %w", err)
		}
		if event == nil {
			continue // not found
		}
		entries = append(entries, sliceCursorEntry{
			invertedTS: invertTimestamp(event.CreatedAt.Unix()),
			eventID:    eventIDBytes,
		})
	}

	if len(entries) == 0 {
		return nil, nil
	}

	// Sort by (invertedTS ASC, eventID ASC) = (created_at DESC, id ASC)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].invertedTS != entries[j].invertedTS {
			return entries[i].invertedTS < entries[j].invertedTS
		}
		return bytesLess(entries[i].eventID, entries[j].eventID)
	})

	return newSliceCursor(entries), nil
}

// buildFilterCursor creates a queryCursor tree for a single filter.
// Uses field hash indexes (0x06) with sort-merge join for AND conditions.
// Falls back to created_at index (0x02) when no conditions are specified.
func (s *PebbleStorage) buildFilterCursor(snapshot *pebble.Snapshot, filter *ReqFilter, iters *[]*pebble.Iterator) (queryCursor, error) {
	// If IDs are specified, use direct Get (most efficient path).
	// Other conditions (authors, kinds, tags, since, until) are checked
	// by filter.Match() in the outer Query loop.
	if len(filter.IDs) > 0 {
		return s.buildIDsCursor(snapshot, filter.IDs)
	}

	var conditionCursors []queryCursor

	// Authors condition
	if len(filter.Authors) > 0 {
		var cursors []queryCursor
		for _, author := range filter.Authors {
			if len(author) != 64 {
				continue
			}
			cur, err := s.newFieldIndexCursor(snapshot, fieldHashAuthor(author), filter, iters)
			if err != nil {
				return nil, err
			}
			cursors = append(cursors, cur)
		}
		if len(cursors) > 0 {
			conditionCursors = append(conditionCursors, wrapCursors(cursors))
		}
	}

	// Kinds condition
	if len(filter.Kinds) > 0 {
		var cursors []queryCursor
		for _, kind := range filter.Kinds {
			cur, err := s.newFieldIndexCursor(snapshot, fieldHashKind(kind), filter, iters)
			if err != nil {
				return nil, err
			}
			cursors = append(cursors, cur)
		}
		if len(cursors) > 0 {
			conditionCursors = append(conditionCursors, wrapCursors(cursors))
		}
	}

	// Tag conditions (one group per tag name, AND between tag names)
	for tagName, tagValues := range filter.Tags {
		if len(tagName) != 1 || len(tagValues) == 0 {
			continue
		}
		var cursors []queryCursor
		for _, tagValue := range tagValues {
			cur, err := s.newFieldIndexCursor(snapshot, fieldHashTag(tagName[0], tagValue), filter, iters)
			if err != nil {
				return nil, err
			}
			cursors = append(cursors, cur)
		}
		if len(cursors) > 0 {
			conditionCursors = append(conditionCursors, wrapCursors(cursors))
		}
	}

	// No conditions -> fallback to created_at index (full scan)
	if len(conditionCursors) == 0 {
		return s.newCreatedAtCursor(snapshot, filter, iters)
	}

	// Single condition -> return directly (no intersection needed)
	if len(conditionCursors) == 1 {
		return conditionCursors[0], nil
	}

	// Multiple conditions -> sort-merge join (AND)
	return newIntersectCursor(conditionCursors), nil
}

// wrapCursors returns a single cursor or a union of cursors.
func wrapCursors(cursors []queryCursor) queryCursor {
	if len(cursors) == 1 {
		return cursors[0]
	}
	return newUnionCursor(cursors)
}

// newFieldIndexCursor creates an indexCursor for a field hash prefix with time bounds.
func (s *PebbleStorage) newFieldIndexCursor(snapshot *pebble.Snapshot, hash uint64, filter *ReqFilter, iters *[]*pebble.Iterator) (*indexCursor, error) {
	prefix := make([]byte, 9)
	prefix[0] = prefixField
	binary.BigEndian.PutUint64(prefix[1:9], hash)

	lower := prefix
	upper := incrementBytes(prefix)

	if filter.Until != nil {
		invertedUntil := invertTimestamp(*filter.Until)
		newLower := make([]byte, 17)
		copy(newLower[:9], prefix)
		binary.BigEndian.PutUint64(newLower[9:17], invertedUntil)
		lower = newLower
	}
	if filter.Since != nil {
		invertedSince := invertTimestamp(*filter.Since)
		newUpper := make([]byte, 17)
		copy(newUpper[:9], prefix)
		binary.BigEndian.PutUint64(newUpper[9:17], invertedSince+1)
		upper = newUpper
	}

	iter, err := snapshot.NewIter(&pebble.IterOptions{
		LowerBound: lower,
		UpperBound: upper,
	})
	if err != nil {
		return nil, fmt.Errorf("create field iterator: %w", err)
	}
	*iters = append(*iters, iter)

	return newIndexCursor(iter, prefix, 9, 17), nil
}

// newCreatedAtCursor creates an indexCursor for the created_at fallback index.
func (s *PebbleStorage) newCreatedAtCursor(snapshot *pebble.Snapshot, filter *ReqFilter, iters *[]*pebble.Iterator) (*indexCursor, error) {
	lower := []byte{prefixCreatedAt}
	upper := []byte{prefixCreatedAt + 1}

	if filter.Until != nil {
		invertedUntil := invertTimestamp(*filter.Until)
		newLower := make([]byte, 9)
		newLower[0] = prefixCreatedAt
		binary.BigEndian.PutUint64(newLower[1:9], invertedUntil)
		lower = newLower
	}
	if filter.Since != nil {
		invertedSince := invertTimestamp(*filter.Since)
		newUpper := make([]byte, 9)
		newUpper[0] = prefixCreatedAt
		binary.BigEndian.PutUint64(newUpper[1:9], invertedSince+1)
		upper = newUpper
	}

	iter, err := snapshot.NewIter(&pebble.IterOptions{
		LowerBound: lower,
		UpperBound: upper,
	})
	if err != nil {
		return nil, fmt.Errorf("create created_at iterator: %w", err)
	}
	*iters = append(*iters, iter)

	return newIndexCursor(iter, []byte{prefixCreatedAt}, 1, 9), nil
}

// bytesLess returns true if a < b lexicographically.
func bytesLess(a, b []byte) bool {
	minLen := min(len(b), len(a))
	for i := range minLen {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return len(a) < len(b)
}

// incrementBytes returns a byte slice that is lexicographically
// the next value after b. Used for creating exclusive upper bounds.
func incrementBytes(b []byte) []byte {
	result := make([]byte, len(b))
	copy(result, b)
	for i := len(result) - 1; i >= 0; i-- {
		if result[i] < 0xFF {
			result[i]++
			return result
		}
		result[i] = 0
	}
	// All bytes were 0xFF, append 0x00 (overflow case)
	return append(result, 0)
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
		return nil, fmt.Errorf("get deletion record: %w", err)
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
					return fmt.Errorf("get event for address deletion: %w", err)
				}
				if targetEvent != nil && targetEvent.Pubkey == event.Pubkey && !targetEvent.CreatedAt.After(event.CreatedAt) {
					// Delete the event
					if err := s.deleteEventFromBatch(batch, existingID); err != nil {
						return fmt.Errorf("delete event by address: %w", err)
					}
					// Also delete the addr index
					if err := batch.Delete(addrKey, pebble.Sync); err != nil {
						return err
					}
				}
			} else if err != pebble.ErrNotFound {
				return fmt.Errorf("get address for deletion: %w", err)
			}
		}
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("commit kind 5 batch: %w", err)
	}
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
		return nil, fmt.Errorf("get event: %w", err)
	}
	defer closer.Close()

	var event Event
	if err := json.Unmarshal(value, &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return &event, nil
}

// getEventByIDFromSnapshot retrieves an event from a snapshot by its ID bytes.
func (s *PebbleStorage) getEventByIDFromSnapshot(snapshot *pebble.Snapshot, eventID []byte) (*Event, error) {
	eventKey := makeEventKey(eventID)
	value, closer, err := snapshot.Get(eventKey)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get event from snapshot: %w", err)
	}
	defer closer.Close()

	var event Event
	if err := json.Unmarshal(value, &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return &event, nil
}

func (s *PebbleStorage) len() int {
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

	// Delete field hash indexes
	authorHash := fieldHashAuthor(event.Pubkey)
	if err := batch.Delete(makeFieldKey(authorHash, invertedTS, eventID), pebble.Sync); err != nil {
		return err
	}

	kindHash := fieldHashKind(event.Kind)
	if err := batch.Delete(makeFieldKey(kindHash, invertedTS, eventID), pebble.Sync); err != nil {
		return err
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 || len(tag[0]) != 1 {
			continue
		}
		tagHash := fieldHashTag(tag[0][0], tag[1])
		if err := batch.Delete(makeFieldKey(tagHash, invertedTS, eventID), pebble.Sync); err != nil {
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

func invertTimestamp(ts int64) uint64 {
	return uint64(math.MaxInt64 - ts)
}

// Field hash helpers using FNV-1a 64-bit.
// NUL (\x00) is used as delimiter to prevent injection attacks
// (JSON strings cannot contain NUL bytes).

func fnvHash(input string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(input))
	return h.Sum64()
}

func fieldHashAuthor(pubkeyHex string) uint64 {
	return fnvHash("author\x00" + pubkeyHex)
}

func fieldHashKind(kind int64) uint64 {
	return fnvHash("kind\x00" + strconv.FormatInt(kind, 10))
}

func fieldHashTag(tagName byte, tagValue string) uint64 {
	return fnvHash(string(tagName) + "\x00" + tagValue)
}

// makeFieldKey constructs a field hash index key.
// Key format: [0x06][field_hash:8][inverted_ts:8][event_id:32] = 49 bytes
func makeFieldKey(hash uint64, invertedTS uint64, eventID []byte) []byte {
	key := make([]byte, 49)
	key[0] = prefixField
	binary.BigEndian.PutUint64(key[1:9], hash)
	binary.BigEndian.PutUint64(key[9:17], invertedTS)
	copy(key[17:49], eventID)
	return key
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
