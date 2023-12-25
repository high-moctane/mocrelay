package mocrelay

import (
	"fmt"
	"slices"
	"sync"

	"github.com/igrmk/treemap/v2"
)

type EventCache struct {
	Cap int

	mu sync.RWMutex

	// map[eventKey]*Event
	evs          map[string]*Event
	evsCreatedAt *treemap.TreeMap[eventCacheEvsCreatedAtKey, *Event]

	// map[eventCacheDeletedEventKey]map[kind5ID]bool
	deleted map[eventCacheDeletedEventKey]map[string]bool
}

func NewEventCache(capacity int) *EventCache {
	return &EventCache{
		Cap: capacity,
		evs: make(map[string]*Event, capacity),
		evsCreatedAt: treemap.NewWithKeyCompare[eventCacheEvsCreatedAtKey, *Event](
			eventCacheEvsCreatedAtKeyTreeCmp,
		),
		deleted: make(map[eventCacheDeletedEventKey]map[string]bool),
	}
}

func (c *EventCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.evs)
}

func (c *EventCache) Add(event *Event) (added bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	eventKey := c.getEventKey(event)

	if c.isDeleted(eventKey, event.Pubkey) {
		return false
	}

	if added = c.add(eventKey, event); !added {
		return
	}

	if event.Kind == 5 {
		c.addKind5(event)
		c.deleteByKind5(event)
	}

	if len(c.evs) > c.Cap {
		if oldest := c.getOldestEvent(); oldest != nil {
			key := c.getEventKey(oldest)
			c.delete(eventCacheDeletedEventKey{key, oldest.Pubkey})
		}
	}

	return true
}

func (c *EventCache) isDeleted(eventKey, pubkey string) bool {
	return c.deleted[eventCacheDeletedEventKey{eventKey, pubkey}] != nil
}

func (c *EventCache) add(eventKey string, event *Event) (added bool) {
	old, ok := c.evs[eventKey]
	if ok {
		if old.CreatedAt >= event.CreatedAt {
			return
		}
		c.evsCreatedAt.Del(eventCacheEvsCreatedAtKey{old.CreatedAt, old.ID})
	}

	c.evs[eventKey] = event
	c.evsCreatedAt.Set(eventCacheEvsCreatedAtKey{event.CreatedAt, event.ID}, event)
	return true
}

func (c *EventCache) addKind5(event *Event) {
	keys := c.getEventKeyFromKind5Tags(event)
	for _, key := range keys {
		k := eventCacheDeletedEventKey{key, event.Pubkey}
		if c.deleted[k] == nil {
			c.deleted[k] = make(map[string]bool)
		}
		c.deleted[eventCacheDeletedEventKey{key, event.Pubkey}][event.ID] = true
	}
}

func (c *EventCache) deleteByKind5(event *Event) {
	keys := c.getEventKeyFromKind5Tags(event)

	for _, key := range keys {
		c.delete(eventCacheDeletedEventKey{key, event.Pubkey})
	}
}

func (c *EventCache) delete(delEvKey eventCacheDeletedEventKey) (deleted bool) {
	cand, ok := c.evs[delEvKey.EventKey]
	if !ok {
		return
	}
	if cand.Pubkey != delEvKey.Pubkey {
		return
	}

	// deleted
	if cand.Kind == 5 {
		keys := c.getEventKeyFromKind5Tags(cand)
		for _, key := range keys {
			delete(c.deleted[eventCacheDeletedEventKey{key, cand.Pubkey}], cand.ID)
			if len(c.deleted[eventCacheDeletedEventKey{key, cand.Pubkey}]) == 0 {
				delete(c.deleted, eventCacheDeletedEventKey{key, cand.Pubkey})
			}
		}
	}

	// evs
	delete(c.evs, delEvKey.EventKey)

	// evsCreatedAt
	c.evsCreatedAt.Del(eventCacheEvsCreatedAtKey{cand.CreatedAt, cand.ID})

	return true
}

func (c *EventCache) getOldestEvent() *Event {
	return c.evsCreatedAt.Reverse().Value()
}

func (c *EventCache) Find(filters []*ReqFilter) []*Event {
	var ret []*Event
	m := NewReqFiltersEventMatchers(filters)

	c.mu.RLock()
	defer c.mu.RUnlock()

	for it := c.evsCreatedAt.Iterator(); it.Valid(); it.Next() {
		if m.Done() {
			break
		}
		if m.CountMatch(it.Value()) {
			ret = append(ret, it.Value())
		}
	}

	return ret
}

func (c *EventCache) getEventKey(event *Event) string {
	switch event.EventType() {
	case EventTypeRegular:
		return event.ID

	case EventTypeReplaceable:
		return fmt.Sprintf("%s:%d", event.Pubkey, event.Kind)

	case EventTypeParamReplaceable:
		idx := slices.IndexFunc(event.Tags, func(t Tag) bool {
			return len(t) >= 1 && t[0] == "d"
		})
		if idx < 0 {
			return ""
		}

		d := ""
		if len(event.Tags[idx]) > 1 {
			d = event.Tags[idx][1]
		}

		return fmt.Sprintf("%s:%d:%s", event.Pubkey, event.Kind, d)
	}

	return ""
}

func (c *EventCache) getEventKeyFromKind5Tags(event *Event) []string {
	var ret []string

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		if tag[0] == "a" || tag[0] == "e" {
			ret = append(ret, tag[1])
		}
	}

	return ret
}

type eventCacheDeletedEventKey struct {
	EventKey string
	Pubkey   string
}

type eventCacheEvsCreatedAtKey struct {
	CreatedAt int64
	ID        string
}

func eventCacheEvsCreatedAtKeyTreeCmp(a, b eventCacheEvsCreatedAtKey) bool {
	return b.CreatedAt < a.CreatedAt || (b.CreatedAt == a.CreatedAt && b.ID < a.ID)
}
