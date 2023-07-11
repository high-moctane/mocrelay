package main

import (
	"sync"
	"time"
)

type DB struct {
	Size int
	fil  Filters

	mu     sync.RWMutex
	events []*Event
	ids    map[string]bool
	ptr    int
}

func NewDB(size int, fil Filters) *DB {
	return &DB{
		events: nil,
		ids:    make(map[string]bool),
		Size:   size,
		ptr:    0,
		fil:    fil,
	}
}

func (db *DB) Save(event *Event) (saved bool) {
	if !db.fil.Match(event) {
		return
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if db.ids[event.ID] {
		return
	}

	if len(db.events) < db.Size {
		db.events = append(db.events, event)
		db.ids[event.ID] = true
	} else {
		delete(db.ids, db.events[db.ptr].ID)
		db.events[db.ptr] = event
		db.ids[event.ID] = true
	}
	db.ptr = (db.ptr + 1) % db.Size

	saved = true
	return
}

func (db *DB) FindAll(fils Filters) []*Event {
	start := time.Now()
	defer promDBQueryTime.Observe(time.Since(start).Seconds())

	var res []*Event

	counts := make([]*int, len(fils))
	var sum *int
	for i, fil := range fils {
		if fil.Limit != nil {
			counts[i] = func() *int { v := 0; return &v }()

			if sum == nil {
				sum = func() *int { v := 0; return &v }()
			}
			*sum += *fil.Limit
		}
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	if len(db.events) == 0 {
		return nil
	}

	mod := func(v int) int { return (v + len(db.events)) % len(db.events) }

	i := mod(db.ptr - 1)
	for {
		ev := db.events[i]

		for j, fil := range fils {
			if fil.Match(ev) {
				if counts[j] != nil && *counts[j] >= *fil.Limit {
					break
				}

				res = append(res, ev)

				if counts[j] != nil {
					*counts[j]++
				}
			}
		}

		if mod(i) == mod(db.ptr) || (sum != nil && len(res) >= *sum) {
			break
		}

		i = mod(i - 1)
	}

	return res
}
