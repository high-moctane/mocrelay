package mocrelay

import (
	"fmt"
	"slices"
)

type eventCache struct {
	rb   *ringBuffer[*Event]
	ids  map[string]*Event
	keys map[string]*Event
}

func newEventCache(capacity int) *eventCache {
	return &eventCache{
		rb:   newRingBuffer[*Event](capacity),
		ids:  make(map[string]*Event, capacity),
		keys: make(map[string]*Event, capacity),
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

	idx := c.rb.IdxFunc(func(v *Event) bool {
		return v.CreatedAt < event.CreatedAt
	})
	if c.rb.Len() == c.rb.Cap && idx < 0 {
		return
	}

	c.ids[event.ID] = event
	c.keys[key] = event

	if c.rb.Len() == c.rb.Cap {
		old := c.rb.Dequeue()
		if k, _ := c.eventKey(old); c.keys[k] == old {
			delete(c.keys, k)
		}
		delete(c.ids, old.ID)
	}
	c.rb.Enqueue(event)

	for i := 0; i+1 < c.rb.Len(); i++ {
		if c.rb.At(i).CreatedAt < c.rb.At(i+1).CreatedAt {
			c.rb.Swap(i, i+1)
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

	for i := 0; i < c.rb.Len(); i++ {
		ev := c.rb.At(i)

		if c.ids[ev.ID] == nil {
			continue
		}
		if k, _ := c.eventKey(ev); c.keys[k] != ev {
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
