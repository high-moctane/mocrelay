package main

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

type EventCache interface {
	Push(*Event) bool
	Find(Filters) []*Event
	Delete(string) bool
}

type MultiEventCache struct {
	// TODO(high-moctane) remove mutex
	mu sync.RWMutex

	userdataCache      *ReplaceableEventCache
	shortTextNoteCache *RegularEventCache

	regularEventCache                  *RegularEventCache
	replaceableEventCache              *ReplaceableEventCache
	parameterizedReplaceableEventCache *ParameterizedReplaceableEventCache
}

func NewMultiEventCache() *MultiEventCache {
	// TODO(high-moctane) remove magic numbers
	return &MultiEventCache{
		userdataCache:                      NewReplaceableEventCache(2500),
		shortTextNoteCache:                 NewRegularEventCache(5000),
		regularEventCache:                  NewRegularEventCache(5000),
		replaceableEventCache:              NewReplaceableEventCache(1000),
		parameterizedReplaceableEventCache: NewParameterizedReplaceableEventCache(1000),
	}
}

func (c *MultiEventCache) Push(e *Event) (saved bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// TODO(high-moctane) remove kind5 logic from here
	if e.Kind == KindEventDeletion {
		for _, tag := range e.Tags {
			if len(tag) < 2 {
				continue
			}
			switch tag[0] {
			case "e":
				c.userdataCache.Delete(tag[1])
				c.shortTextNoteCache.Delete(tag[1])
				c.regularEventCache.Delete(tag[1])
				c.replaceableEventCache.Delete(tag[1])
			case "a":
				c.parameterizedReplaceableEventCache.Delete(tag[1])
			}
		}

		return c.regularEventCache.Push(e)
	}

	switch e.EventType() {
	case EventBasic:
		switch e.Kind {
		case KindMetadata, KindRecommendRelay, KindContacts:
			return c.userdataCache.Push(e)

		case KindShortTextNote:
			return c.shortTextNoteCache.Push(e)

		case KindEncryptedDirectMessages, KindRepost, KindReaction, KindGenericRepost:
			return c.regularEventCache.Push(e)
		}

	case EventRegular:
		return c.regularEventCache.Push(e)

	case EventReplaceable:
		return c.replaceableEventCache.Push(e)

	case EventEphemeral:
		return false

	case EventParameterizedReplaceable:
		return c.parameterizedReplaceableEventCache.Push(e)
	}

	return false
}

func (c *MultiEventCache) Find(fil Filters) []*Event {
	// TODO(high-moctane) good implementation
	var es, res []*Event

	func() {
		c.mu.RLock()
		defer c.mu.RUnlock()

		es = append(es, c.userdataCache.Find(fil)...)
		es = append(es, c.shortTextNoteCache.Find(fil)...)
		es = append(es, c.regularEventCache.Find(fil)...)
		es = append(es, c.replaceableEventCache.Find(fil)...)
		es = append(es, c.parameterizedReplaceableEventCache.Find(fil)...)
	}()

	slices.SortFunc(es, func(a, b *Event) int {
		if sub := a.CreatedAtToTime().Sub(b.CreatedAtToTime()); sub < 0 {
			return 1
		} else if sub == 0 {
			return -strings.Compare(a.ID, b.ID)
		} else {
			return -1
		}
	})

	f := NewFiltersCounter(fil)
	for _, e := range es {
		if f.Match(e) {
			res = append(res, e)
		}
		if f.Done() {
			break
		}
	}

	return res
}

func (c *MultiEventCache) Delete(k string) (found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	res1 := c.userdataCache.Delete(k)
	res2 := c.shortTextNoteCache.Delete(k)
	res3 := c.regularEventCache.Delete(k)
	res4 := c.replaceableEventCache.Delete(k)
	res5 := c.parameterizedReplaceableEventCache.Delete(k)
	return res1 || res2 || res3 || res4 || res5
}

type RegularEventCache struct {
	cache *KeyValueCache[string, *Event]
}

func NewRegularEventCache(size int) *RegularEventCache {
	return &RegularEventCache{
		cache: NewKeyValueCache[string, *Event](size),
	}
}

func (c *RegularEventCache) Push(e *Event) (saved bool) {
	return c.cache.Push(e.ID, e)
}

func (c *RegularEventCache) Find(fil Filters) []*Event {
	var res []*Event
	f := NewFiltersCounter(fil)

	c.cache.Loop(func(id string, e *Event) bool {
		if f.Match(e) {
			res = append(res, e)
		}
		return !f.Done()
	})

	return res
}

func (c *RegularEventCache) Delete(id string) (found bool) {
	return c.cache.Delete(id)
}

type ReplaceableEventCache struct {
	cache *KeyValueCache[string, *Event]
}

func NewReplaceableEventCache(size int) *ReplaceableEventCache {
	return &ReplaceableEventCache{
		cache: NewKeyValueCache[string, *Event](size),
	}
}

func (*ReplaceableEventCache) getReplaceableEventKey(e *Event) string {
	return fmt.Sprintf("%d:%s", e.Kind, e.Pubkey)
}

func (c *ReplaceableEventCache) Push(e *Event) (saved bool) {
	k := c.getReplaceableEventKey(e)
	c.cache.Delete(k)
	return c.cache.Push(k, e)
}

func (c *ReplaceableEventCache) Find(fil Filters) []*Event {
	var res []*Event
	f := NewFiltersCounter(fil)

	c.cache.Loop(func(k string, e *Event) bool {
		if f.Match(e) {
			res = append(res, e)
		}
		return !f.Done()
	})

	return res
}

func (c *ReplaceableEventCache) Delete(id string) (found bool) {
	return c.cache.Delete(id)
}

type ParameterizedReplaceableEventCache struct {
	cache *KeyValueCache[string, *Event]
}

func NewParameterizedReplaceableEventCache(size int) *ParameterizedReplaceableEventCache {
	return &ParameterizedReplaceableEventCache{
		cache: NewKeyValueCache[string, *Event](size),
	}
}

func (c *ParameterizedReplaceableEventCache) Push(e *Event) (saved bool) {
	c.cache.Delete(*e.Naddr())
	return c.cache.Push(*e.Naddr(), e)
}

func (c *ParameterizedReplaceableEventCache) Find(fil Filters) []*Event {
	var res []*Event
	f := NewFiltersCounter(fil)

	c.cache.Loop(func(k string, e *Event) bool {
		if f.Match(e) {
			res = append(res, e)
		}
		return !f.Done()
	})

	return res
}

func (c *ParameterizedReplaceableEventCache) Delete(naddr string) (found bool) {
	return c.cache.Delete(naddr)
}
