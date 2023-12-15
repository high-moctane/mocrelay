package mocrelay

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
)

type EventCache struct {
	Cap int

	mu  sync.RWMutex
	evs map[string]*Event
}

func NewEventCache(capacity int) *EventCache {
	return &EventCache{
		Cap: capacity,
		evs: make(map[string]*Event, capacity),
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

	eventKey := c.getEventCache(event)

	if old, ok := c.evs[eventKey]; ok && old.CreatedAt >= event.CreatedAt {
		return false
	}

	c.evs[eventKey] = event

	if len(c.evs) > c.Cap {
		var oldest *Event
		var key string
		for k, ev := range c.evs {
			if oldest == nil || ev.CreatedAt < oldest.CreatedAt {
				oldest = ev
				key = k
			}
		}
		delete(c.evs, key)
	}

	return true
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

func (c *EventCache) getEventCache(event *Event) string {
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
