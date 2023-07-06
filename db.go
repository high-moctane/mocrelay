package main

import (
	"fmt"
	"sync"
)

type DB struct {
	mu     sync.RWMutex
	events []*Event
	Size   int
	ptr    int
}

func NewDB(size int) *DB {
	return &DB{
		events: nil,
		Size:   size,
		ptr:    0,
	}
}

func (db *DB) Save(event *Event) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if len(db.events) < db.Size {
		db.events = append(db.events, event)
	} else {
		db.events[db.ptr] = event
	}
	db.ptr = (db.ptr + 1) % db.Size

	fmt.Println(db.events)
}

func (db *DB) FindAll(fils Filters) []*Event {
	mod := func(v int) int { return (v + len(db.events)) % len(db.events) }

	var res []*Event
	counts := make([]*int, len(fils))
	var sum *int

	for i, fil := range fils {
		counts[i] = fil.Limit
		if counts[i] != nil {
			if sum == nil {
				sum = func() *int { v := 0; return &v }()
			}
			*sum += *counts[i]
		}
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	if len(db.events) == 0 {
		return nil
	}

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
