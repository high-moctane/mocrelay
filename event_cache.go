package mocrelay

import (
	"cmp"
	"fmt"
	"slices"
)

type eventCache struct {
	size int

	latest *skipList[eventCacheKeyLatest, *Event]

	// *skipList[evKey, *Event]
	evKeys *skipList[string, *Event]

	// *skipList[id, *Event]
	kind5 *skipList[string, *Event]

	// *skipList[eventKey, struct{}]
	delEvKeys *skipList[string, struct{}]
}

type eventCacheKeyLatest struct {
	CreatedAt int64
	ID        string
}

var eventCacheKeyLatestSentinel = eventCacheKeyLatest{CreatedAt: -1}

func eventCacheKeyLatestCmp(a, b eventCacheKeyLatest) int {
	res := -cmp.Compare(a.CreatedAt, b.CreatedAt)
	if res != 0 {
		return res
	}
	return -cmp.Compare(a.ID, b.ID)
}

func newEventCache(size int) *eventCache {
	latest := newSkipList[eventCacheKeyLatest, *Event](eventCacheKeyLatestCmp)
	latest.Add(eventCacheKeyLatestSentinel, nil)

	return &eventCache{
		size:      size,
		latest:    latest,
		evKeys:    newSkipList[string, *Event](cmp.Compare),
		kind5:     newSkipList[string, *Event](cmp.Compare),
		delEvKeys: newSkipList[string, struct{}](cmp.Compare),
	}
}

func eventCacheEventKey(event *Event) string {
	switch event.EventType() {
	case EventTypeRegular:
		return event.ID

	case EventTypeReplaceable:
		return eventCacheReplaceableEventKey(event)

	case EventTypeParamReplaceable:
		return eventCacheParamReplaceableEventKey(event)

	default:
		return ""
	}
}

func eventCacheReplaceableEventKey(event *Event) string {
	return fmt.Sprintf("%s:%d", event.Pubkey, event.Kind)
}

func eventCacheParamReplaceableEventKey(event *Event) string {
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

func (c *eventCache) Add(event *Event) (added bool) {
	evKey := eventCacheEventKey(event)
	if evKey == "" {
		return
	}

	if c.deleted(evKey) {
		return
	}

	if !c.updateEvKeys(event, evKey) {
		return
	}

	if event.Kind == 5 {
		c.doKind5(event, evKey)
	}

	added = c.add(event, evKey)
	c.truncate()

	return
}

func (c *eventCache) deleted(evKey string) bool {
	_, ok := c.delEvKeys.Find(evKey)
	return ok
}

func (c *eventCache) doKind5(event *Event, evKey string) {
	c.kind5.Add(event.ID, event)

	for _, evKey := range c.kind5EventKeys(event) {
		c.delEvKeys.Add(evKey, struct{}{})
	}
}

func (*eventCache) kind5EventKeys(event *Event) []string {
	var ret []string

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		if tag[0] == "e" || tag[0] == "a" {
			ret = append(ret, tag[1])
		}
	}

	return ret
}

func (c *eventCache) updateEvKeys(event *Event, evKey string) (updated bool) {
	old, ok := c.evKeys.Find(evKey)
	if ok {
		if old.CreatedAt >= event.CreatedAt {
			return
		}
		c.delete(old, evKey)
	}

	return c.evKeys.Add(evKey, event)
}

func (c *eventCache) add(event *Event, evKey string) (added bool) {
	key := eventCacheKeyLatest{
		CreatedAt: event.CreatedAt,
		ID:        event.ID,
	}
	return c.latest.Add(key, event)
}

func (c *eventCache) truncate() {
	for c.latest.Len() > c.size+1 {
		oldest, ok := c.latest.FindPre(eventCacheKeyLatestSentinel)
		if !ok {
			break
		}
		c.delete(oldest, eventCacheEventKey(oldest))
	}
}

func (c *eventCache) delete(event *Event, evKey string) {
	key := eventCacheKeyLatest{
		CreatedAt: event.CreatedAt,
		ID:        event.ID,
	}

	if event.Kind == 5 {
		c.kind5.Delete(event.ID)
		for _, k := range c.kind5EventKeys(event) {
			c.delEvKeys.Delete(k)
		}
	}

	if old, ok := c.evKeys.Find(evKey); ok && old.ID == event.ID {
		c.evKeys.Delete(evKey)
	}

	c.latest.Delete(key)
}

func (c *eventCache) Find(fs []*ReqFilter) []*Event {
	var ret []*Event
	m := NewReqFiltersEventMatchers(fs)

	for node := c.latest.Head.Next(); node != nil; node = node.Next() {
		if m.Done() {
			break
		}

		ev := node.V
		if ev == nil {
			break
		}

		if c.deleted(eventCacheEventKey(ev)) {
			continue
		}

		if m.CountMatch(ev) {
			ret = append(ret, ev)
		}
	}

	return ret
}
