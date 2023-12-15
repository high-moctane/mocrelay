package mocrelay

type EventCache struct {
}

func NewEventCache(capacity int) *EventCache {
	return &EventCache{}
}

func (c *EventCache) Add(event *Event) (added bool) {
	panic("unimplemented")
}

func (c *EventCache) Find(filters []*ReqFilter) []*Event {
	panic("unimplemented")
}
