package mocrelay

import (
	"cmp"
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
	evsIndex     eventCacheEvsIndex

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
		evsIndex: make(eventCacheEvsIndex),
		deleted:  make(map[eventCacheDeletedEventKey]map[string]bool),
	}
}

func (c *EventCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.len()
}

func (c *EventCache) len() int {
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
		c.delete(eventCacheDeletedEventKey{eventKey, old.Pubkey})
	}

	c.evs[eventKey] = event
	c.evsCreatedAt.Set(eventCacheEvsCreatedAtKey{event.CreatedAt, event.ID}, event)
	c.evsIndex.Add(event)
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

	// evsIndex
	c.evsIndex.Delete(cand)

	return true
}

func (c *EventCache) getOldestEvent() *Event {
	return c.evsCreatedAt.Reverse().Value()
}

func (c *EventCache) Find(filters []*ReqFilter) []*Event {
	tree := c.findNeedLock(filters)
	if tree == nil {
		return nil
	}

	var ret []*Event
	for it := tree.Iterator(); it.Valid(); it.Next() {
		ret = append(ret, it.Value())
	}

	return ret
}

func (c *EventCache) findNeedLock(
	filters []*ReqFilter,
) *treemap.TreeMap[eventCacheEvsCreatedAtKey, *Event] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.len() == 0 {
		return nil
	}

	ret := treemap.NewWithKeyCompare[eventCacheEvsCreatedAtKey, *Event](
		eventCacheEvsCreatedAtKeyTreeCmp,
	)

	for _, filter := range filters {
		t, ok := c.evsIndex.Find(filter)
		if ok {
			for it := t.Iterator(); it.Valid(); it.Next() {
				ev := it.Value()
				ret.Set(eventCacheEvsCreatedAtKey{ev.CreatedAt, ev.ID}, ev)
			}
		} else {
			m := NewReqFilterMatcher(filter)
			for it := c.evsCreatedAt.Iterator(); it.Valid(); it.Next() {
				if m.Done() {
					break
				}
				if m.CountMatch(it.Value()) {
					ret.Set(it.Key(), it.Value())
				}
			}
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

type eventCacheEvsIndexKey struct {
	What  int8
	Value any
}

const (
	eventCacheEvsIndexKeyWhatID = iota
	eventCacheEvsIndexKeyWhatAuthor
	eventCacheEvsIndexKeyWhatKind
	eventCacheEvsIndexKeyWhatTag
)

type eventCacheEvsIndex map[eventCacheEvsIndexKey]map[string]*Event

func (eventCacheEvsIndex) keysFromEvent(event *Event) []eventCacheEvsIndexKey {
	var ret []eventCacheEvsIndexKey

	ret = append(ret, eventCacheEvsIndexKey{eventCacheEvsIndexKeyWhatID, event.ID})
	ret = append(ret, eventCacheEvsIndexKey{eventCacheEvsIndexKeyWhatAuthor, event.Pubkey})
	ret = append(ret, eventCacheEvsIndexKey{eventCacheEvsIndexKeyWhatKind, event.Kind})

	for _, tag := range event.Tags {
		if len(tag) == 0 {
			continue
		}
		if len(tag[0]) != 1 {
			continue
		}
		var v string
		if len(tag) >= 2 {
			v = tag[1]
		}
		ret = append(ret, eventCacheEvsIndexKey{
			eventCacheEvsIndexKeyWhatTag,
			[2]string{tag[0], v},
		})
	}

	return ret
}

func (c eventCacheEvsIndex) keysFromReqFilter(filter *ReqFilter) [][]eventCacheEvsIndexKey {
	var ret [][]eventCacheEvsIndexKey

	if filter.IDs != nil {
		keys := make([]eventCacheEvsIndexKey, 0, len(filter.IDs))
		for _, id := range filter.IDs {
			keys = append(keys, eventCacheEvsIndexKey{eventCacheEvsIndexKeyWhatID, id})
		}
		ret = append(ret, keys)
	}

	if filter.Authors != nil {
		keys := make([]eventCacheEvsIndexKey, 0, len(filter.Authors))
		for _, author := range filter.Authors {
			keys = append(keys, eventCacheEvsIndexKey{eventCacheEvsIndexKeyWhatAuthor, author})
		}
		ret = append(ret, keys)
	}

	if filter.Kinds != nil {
		keys := make([]eventCacheEvsIndexKey, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			keys = append(keys, eventCacheEvsIndexKey{eventCacheEvsIndexKeyWhatKind, kind})
		}
		ret = append(ret, keys)
	}

	if filter.Tags != nil {
		for tag, vs := range filter.Tags {
			keys := make([]eventCacheEvsIndexKey, 0, len(vs))
			for _, v := range vs {
				keys = append(keys, eventCacheEvsIndexKey{
					eventCacheEvsIndexKeyWhatTag,
					[2]string{tag, v},
				})
			}
			ret = append(ret, keys)
		}
	}

	return ret
}

func (c eventCacheEvsIndex) isFullScanReqFilter(filter *ReqFilter) bool {
	return filter.IDs == nil && filter.Authors == nil && filter.Kinds == nil && filter.Tags == nil
}

func (c eventCacheEvsIndex) Add(event *Event) {
	for _, key := range c.keysFromEvent(event) {
		var m map[string]*Event
		if m = c[key]; m == nil {
			m = make(map[string]*Event)
			c[key] = m
		}
		m[event.ID] = event
	}
}

func (c eventCacheEvsIndex) Delete(event *Event) {
	for _, key := range c.keysFromEvent(event) {
		m, ok := c[key]
		if !ok {
			continue
		}
		delete(m, event.ID)
		if len(m) == 0 {
			delete(c, key)
		}
	}
}

func (c eventCacheEvsIndex) Find(
	filter *ReqFilter,
) (ret *treemap.TreeMap[eventCacheEvsCreatedAtKey, *Event], ok bool) {
	ok = !c.isFullScanReqFilter(filter)
	if !ok {
		return
	}

	ret = treemap.NewWithKeyCompare[eventCacheEvsCreatedAtKey, *Event](
		eventCacheEvsCreatedAtKeyTreeCmp,
	)

	keysSlice := c.keysFromReqFilter(filter)

	idMaps := make([]map[string]*Event, 0, len(keysSlice))

	for _, keys := range keysSlice {
		m := make(map[string]*Event, len(keys))
		for _, key := range keys {
			for id, ev := range c[key] {
				m[id] = ev
			}
		}
		idMaps = append(idMaps, m)
	}

	slices.SortFunc(idMaps, func(a, b map[string]*Event) int {
		return cmp.Compare(len(a), len(b))
	})

	for len(idMaps) > 1 {
		m := idMaps[0]
		mlast := idMaps[len(idMaps)-1]

		for id := range m {
			if _, ok := mlast[id]; !ok {
				delete(m, id)
			}
		}

		idMaps = idMaps[:len(idMaps)-1]
	}

	limit := len(idMaps[0])
	if filter.Limit != nil {
		limit = min(limit, int(*filter.Limit))
	}
	m := NewReqFilterMatcher(&ReqFilter{
		Since: filter.Since,
		Until: filter.Until,
	})

	cnt := 0
	for _, ev := range idMaps[0] {
		if !m.Match(ev) {
			continue
		}
		ret.Set(eventCacheEvsCreatedAtKey{ev.CreatedAt, ev.ID}, ev)
		cnt++
		if cnt > limit {
			k := ret.Reverse().Key()
			ret.Del(k)
			cnt--
		}
	}

	return
}
