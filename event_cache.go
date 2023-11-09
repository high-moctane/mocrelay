package mocrelay

import (
	"cmp"
	"fmt"
	"slices"
)

type eventCache struct {
	capacity int
	latest   *skipList[*Event, *Event]
	oldest   *skipList[*Event, *Event]
	ids      *skipList[string, *Event]
	keys     *skipList[string, *Event]
	index    *skipList[eventCacheSearchKey, *Event]
}

type eventCacheSearchKey struct {
	Value string
	What  string
	ID    string
}

func newEventCache(capacity int) *eventCache {
	cmpFunc := func(a, b *Event) int {
		res := -cmp.Compare(a.CreatedAt, b.CreatedAt)
		if res != 0 {
			return res
		}

		return -cmp.Compare(a.ID, b.ID)
	}

	revCmpFunc := func(a, b *Event) int { return cmpFunc(b, a) }

	indexCmpFunc := func(a, b eventCacheSearchKey) int {
		if res := cmp.Compare(a.Value, b.Value); res != 0 {
			return res
		}
		if res := cmp.Compare(a.What, b.What); res != 0 {
			return res
		}
		return cmp.Compare(a.ID, b.ID)
	}

	return &eventCache{
		capacity: capacity,
		latest:   newSkipList[*Event, *Event](cmpFunc),
		oldest:   newSkipList[*Event, *Event](revCmpFunc),
		ids:      newSkipList[string, *Event](cmp.Compare),
		keys:     newSkipList[string, *Event](cmp.Compare),
		index:    newSkipList[eventCacheSearchKey, *Event](indexCmpFunc),
	}
}

func (*eventCache) eventKeyRegular(event *Event) string { return event.ID }

func (*eventCache) eventKeyReplaceable(event *Event) string {
	return fmt.Sprintf("%s:%d", event.Pubkey, event.Kind)
}

func (*eventCache) eventKeyParameterized(event *Event) string {
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

func (c *eventCache) eventKey(event *Event) (key string, ok bool) {
	switch event.EventType() {
	case EventTypeRegular:
		return c.eventKeyRegular(event), true
	case EventTypeReplaceable:
		return c.eventKeyReplaceable(event), true
	case EventTypeParamReplaceable:
		key := c.eventKeyParameterized(event)
		return key, key != ""
	default:
		return "", false
	}
}

func (*eventCache) eventIndexKey(event *Event) []eventCacheSearchKey {
	ret := []eventCacheSearchKey{
		{
			Value: event.ID,
			What:  "id",
			ID:    event.ID,
		},
		{
			Value: event.Pubkey,
			What:  "pubkey",
			ID:    event.ID,
		},
	}

	for _, tag := range event.Tags {
		var k string
		if len(tag) < 1 {
			continue
		}
		if len(tag[0]) != 1 {
			continue
		}
		if (tag[0][0] < 'a' || 'z' < tag[0][0]) && (tag[0][0] < 'A' || 'Z' < tag[0][0]) {
			continue
		}
		if len(tag) > 1 {
			k = tag[1]
		}

		key := eventCacheSearchKey{
			Value: k,
			What:  tag[0],
			ID:    event.ID,
		}

		ret = append(ret, key)
	}

	return ret
}

func (c *eventCache) Add(event *Event) (added bool) {
	if _, ok := c.ids.Find(event.ID); ok {
		return
	}
	key, ok := c.eventKey(event)
	if !ok {
		return
	}
	if old, ok := c.keys.Find(key); ok && old.CreatedAt > event.CreatedAt {
		return
	}

	c.ids.Add(event.ID, event)
	c.keys.Delete(key)
	c.keys.Add(key, event)
	c.latest.Add(event, event)
	c.oldest.Add(event, event)
	for _, k := range c.eventIndexKey(event) {
		c.index.Add(k, event)
	}

	if c.latest.Len() > c.capacity {
		c.oldest.Head.NextsMu.RLock()
		head := c.oldest.Head.Nexts[0]
		c.oldest.Head.NextsMu.RUnlock()
		old := head.V

		k, _ := c.eventKey(old)
		if ev, ok := c.keys.Find(k); ok && ev == old {
			c.keys.Delete(k)
		}
		c.ids.Delete(old.ID)

		c.latest.Delete(old)
		c.oldest.Delete(old)

		for _, k := range c.eventIndexKey(event) {
			c.index.Delete(k)
		}
	}

	added = true
	return
}

func (c *eventCache) DeleteID(id, pubkey string) {
	event, ok := c.ids.Find(id)
	if !ok || event.Pubkey != pubkey {
		return
	}

	k, _ := c.eventKey(event)
	if ev, ok := c.keys.Find(k); ok && ev == event {
		c.keys.Delete(k)
	}
	c.ids.Delete(id)
}

func (c *eventCache) DeleteNaddr(naddr, pubkey string) {
	event, ok := c.keys.Find(naddr)
	if !ok || event.Pubkey != pubkey {
		return
	}
	c.ids.Delete(event.ID)
	c.keys.Delete(naddr)
}

func (c *eventCache) Find(matcher EventCountMatcher) []*Event {
	var ret []*Event

	for node := c.latest.Head.Next(); node != nil; node = node.Next() {
		ev := node.V

		if _, ok := c.ids.Find(ev.ID); !ok {
			continue
		}
		k, _ := c.eventKey(ev)
		if e, ok := c.keys.Find(k); !ok || e.ID != ev.ID {
			continue
		}

		if matcher.Done() {
			break
		}
		if matcher.CountMatch(ev) {
			ret = append(ret, ev)
		}
	}

	return ret
}
