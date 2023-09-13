package mocrelay

import (
	"slices"
	"strings"

	"github.com/high-moctane/mocrelay/nostr"
)

type EventMatcher interface {
	Match(*nostr.Event) bool
}

type EventCountMatcher interface {
	EventMatcher
	CountMatch(*nostr.Event) bool
	Count() int64
	Done() bool
}

type EventCountMatchers[T EventCountMatcher] []T

func (m EventCountMatchers[T]) Match(event *nostr.Event) bool {
	match := false
	for _, mm := range m {
		match = mm.Match(event) || match
	}
	return match
}

func (m EventCountMatchers[T]) CountMatch(event *nostr.Event) bool {
	match := false
	for _, mm := range m {
		match = mm.CountMatch(event) || match
	}
	return match
}

func (m EventCountMatchers[T]) Count() int64 {
	var ret int64
	for _, mm := range m {
		ret = max(ret, mm.Count())
	}
	return ret
}

func (m EventCountMatchers[T]) Done() bool {
	done := true
	for _, mm := range m {
		done = done && mm.Done()
	}
	return done
}

var _ EventCountMatcher = (*ReqFilterEventMatcher)(nil)

type ReqFilterEventMatcher struct {
	cnt int64
	f   *nostr.ReqFilter
}

func NewReqFilterMatcher(filter *nostr.ReqFilter) *ReqFilterEventMatcher {
	if filter == nil {
		panic("filter must be non-nil pointer")
	}
	return &ReqFilterEventMatcher{
		cnt: 0,
		f:   filter,
	}
}

func (m *ReqFilterEventMatcher) Match(event *nostr.Event) bool {
	match := true

	if m.f.IDs != nil {
		match = match && slices.ContainsFunc(*m.f.IDs, func(id string) bool {
			return strings.HasPrefix(event.ID, id)
		})
	}

	if m.f.Kinds != nil {
		match = match && slices.ContainsFunc(*m.f.Kinds, func(kind int64) bool {
			return event.Kind == kind
		})
	}

	if m.f.Authors != nil {
		match = match && slices.ContainsFunc(*m.f.Authors, func(author string) bool {
			return strings.HasPrefix(event.Pubkey, author)
		})
	}

	if m.f.Tags != nil {
		for tag, vs := range *m.f.Tags {
			match = match && slices.ContainsFunc(vs, func(v string) bool {
				return slices.ContainsFunc(event.Tags, func(tagArr nostr.Tag) bool {
					return len(tagArr) >= 1 && tagArr[0] == string(tag[1]) && (len(tagArr) == 1 || strings.HasPrefix(tagArr[1], v))
				})
			})
		}
	}

	if m.f.Since != nil {
		match = match && *m.f.Since <= event.CreatedAt
	}

	if m.f.Until != nil {
		match = match && event.CreatedAt <= *m.f.Until
	}

	return match
}

func (m *ReqFilterEventMatcher) CountMatch(event *nostr.Event) bool {
	match := m.Match(event)
	if match {
		m.cnt++
	}
	return match
}

func (m *ReqFilterEventMatcher) Count() int64 {
	return m.cnt
}

func (m *ReqFilterEventMatcher) Done() bool {
	return m.f.Limit != nil && *m.f.Limit <= m.cnt
}

type ReqFiltersMatcher []*ReqFilterEventMatcher

func NewReqFiltersEventMatchers(filters []*nostr.ReqFilter) EventCountMatchers[*ReqFilterEventMatcher] {
	if filters == nil {
		panic("filters must be non-nil slice")
	}
	ret := make([]*ReqFilterEventMatcher, len(filters))
	for i, f := range filters {
		ret[i] = NewReqFilterMatcher(f)
	}
	return ret
}
