package mocrelay

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
)

type EventCache struct {
	Cap int

	mu sync.RWMutex

	// map[eventKey]*Event
	evs map[string]*Event

	// map[deletedEventKey]map[kind5ID]bool
	deleted map[deletedEventKey]map[string]bool
}

func NewEventCache(capacity int) *EventCache {
	return &EventCache{
		Cap:     capacity,
		evs:     make(map[string]*Event, capacity),
		deleted: make(map[deletedEventKey]map[string]bool),
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

	if old, ok := c.evs[eventKey]; ok && old.CreatedAt >= event.CreatedAt {
		return false
	}

	c.evs[eventKey] = event

	if event.Kind == 5 {
		c.addKind5(event)
		c.deleteByKind5(event)
	}

	if len(c.evs) > c.Cap {
		if oldest := c.getOldestEvent(); oldest != nil {
			key := c.getEventKey(oldest)
			c.delete(deletedEventKey{key, oldest.Pubkey})
		}
	}

	return true
}

func (c *EventCache) isDeleted(eventKey, pubkey string) bool {
	return c.deleted[deletedEventKey{eventKey, pubkey}] != nil
}

func (c *EventCache) addKind5(event *Event) {
	keys := c.getEventKeyFromKind5Tags(event)
	for _, key := range keys {
		k := deletedEventKey{key, event.Pubkey}
		if c.deleted[k] == nil {
			c.deleted[k] = make(map[string]bool)
		}
		c.deleted[deletedEventKey{key, event.Pubkey}][event.ID] = true
	}
}

func (c *EventCache) deleteByKind5(event *Event) {
	keys := c.getEventKeyFromKind5Tags(event)

	for _, key := range keys {
		c.delete(deletedEventKey{key, event.Pubkey})
	}
}

func (c *EventCache) delete(delEvKey deletedEventKey) (deleted bool) {
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
			delete(c.deleted[deletedEventKey{key, cand.Pubkey}], cand.ID)
			if len(c.deleted[deletedEventKey{key, cand.Pubkey}]) == 0 {
				delete(c.deleted, deletedEventKey{key, cand.Pubkey})
			}
		}
	}

	// evs
	delete(c.evs, delEvKey.EventKey)

	return true
}

func (c *EventCache) getOldestEvent() *Event {
	var oldest *Event
	for _, ev := range c.evs {
		if oldest == nil || ev.CreatedAt < oldest.CreatedAt {
			oldest = ev
		}
	}
	return oldest
}

func (c *EventCache) Find(filters []*ReqFilter) []*Event {
	c.mu.RLock()
	defer c.mu.RUnlock()

	events := make(map[string]*Event)
	for _, f := range filters {
		evs := c.findByFilter(f)
		for _, ev := range evs {
			events[ev.ID] = ev
		}
	}

	var ret []*Event
	for _, ev := range events {
		ret = append(ret, ev)
	}

	slices.SortFunc(ret, c.findResultCmp)

	return ret
}

func (c *EventCache) findByFilter(f *ReqFilter) []*Event {
	var ret []*Event

	m := NewReqFilterMatcher(f)
	for _, ev := range c.evs {
		if m.Match(ev) {
			ret = append(ret, ev)
		}
	}

	slices.SortFunc(ret, c.findResultCmp)

	limit := len(ret)
	if f.Limit != nil {
		// TODO
		limit = min(limit, int(*f.Limit))
	}

	ret = slices.Clip(ret[:limit])

	return ret
}

func (c *EventCache) findResultCmp(a, b *Event) int {
	if res := -cmp.Compare(a.CreatedAt, b.CreatedAt); res != 0 {
		return res
	}
	return -cmp.Compare(a.ID, b.ID)
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

type deletedEventKey struct {
	EventKey string
	Pubkey   string
}
