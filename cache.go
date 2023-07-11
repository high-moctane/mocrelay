package main

import (
	"sync"
	"time"
)

type Cache struct {
	Size int
	fil  Filters

	mu     sync.RWMutex
	events []*Event
	ids    map[string]bool
	ptr    int
}

func NewCache(size int, fil Filters) *Cache {
	return &Cache{
		events: nil,
		ids:    make(map[string]bool),
		Size:   size,
		ptr:    0,
		fil:    fil,
	}
}

func (cache *Cache) Save(event *Event) (saved bool) {
	if !cache.fil.Match(event) {
		return
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.ids[event.ID] {
		return
	}

	if len(cache.events) < cache.Size {
		cache.events = append(cache.events, event)
		cache.ids[event.ID] = true
	} else {
		delete(cache.ids, cache.events[cache.ptr].ID)
		cache.events[cache.ptr] = event
		cache.ids[event.ID] = true
	}
	cache.ptr = (cache.ptr + 1) % cache.Size

	saved = true
	return
}

func (cache *Cache) FindAll(fils Filters) []*Event {
	start := time.Now()
	defer promCacheQueryTime.Observe(time.Since(start).Seconds())

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

	cache.mu.RLock()
	defer cache.mu.RUnlock()

	if len(cache.events) == 0 {
		return nil
	}

	mod := func(v int) int { return (v + len(cache.events)) % len(cache.events) }

	i := mod(cache.ptr - 1)
	for {
		ev := cache.events[i]

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

		if mod(i) == mod(cache.ptr) || (sum != nil && len(res) >= *sum) {
			break
		}

		i = mod(i - 1)
	}

	return res
}
