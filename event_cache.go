package mocrelay

import (
	"container/heap"
	"fmt"
	"slices"
)

type eventCache struct {
	size int
	buf  typedHeap[*Event]
	ids  map[string]*Event
	keys map[string]*Event
}

func newEventCache(size int) *eventCache {
	return &eventCache{
		size: size,
		buf: newTypedHeap[*Event](func(a, b *Event) bool {
			return a.CreatedAt < b.CreatedAt
		}),
		ids:  make(map[string]*Event, size),
		keys: make(map[string]*Event, size),
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

func (c *eventCache) Add(event *Event) (added bool) {
	if c.ids[event.ID] != nil {
		return
	}
	key, ok := c.eventKey(event)
	if !ok {
		return
	}
	if old, ok := c.keys[key]; ok && old.CreatedAt > event.CreatedAt {
		return
	}

	c.ids[event.ID] = event
	c.keys[key] = event

	if c.buf.Len() < c.size {
		heap.Push(&c.buf, event)
	} else {
		old := c.buf.S[0]
		if old.CreatedAt < event.CreatedAt {
			if k, _ := c.eventKey(old); c.keys[k] == old {
				delete(c.keys, k)
			}
			delete(c.ids, old.ID)

			c.buf.S[0] = event
			heap.Fix(&c.buf, 0)
		}
	}

	added = true
	return
}

func (c *eventCache) DeleteID(id, pubkey string) {
	event := c.ids[id]
	if event == nil || event.Pubkey != pubkey {
		return
	}

	if k, _ := c.eventKey(event); c.keys[k] == event {
		delete(c.keys, k)
	}
	delete(c.ids, id)
}

func (c *eventCache) DeleteNaddr(naddr, pubkey string) {
	event := c.keys[naddr]
	if event == nil || event.Pubkey != pubkey {
		return
	}
	delete(c.ids, event.ID)
	delete(c.keys, naddr)
}

func (c *eventCache) Find(matcher EventCountMatcher) []*Event {
	var ret []*Event
	newBuf := newTypedHeap[*Event](c.buf.LessFunc)

	for c.buf.Len() > 0 {
		ev := heap.Pop(&c.buf).(*Event)

		if c.ids[ev.ID] == nil {
			continue
		}
		if k, _ := c.eventKey(ev); c.keys[k] != ev {
			continue
		}

		newBuf.PushT(ev)
	}

	for i := newBuf.Len() - 1; i >= 0; i-- {
		if matcher.Done() {
			break
		}
		if matcher.CountMatch(newBuf.S[i]) {
			ret = append(ret, newBuf.S[i])
		}
	}

	heap.Init(&newBuf)

	c.buf = newBuf
	return ret
}
