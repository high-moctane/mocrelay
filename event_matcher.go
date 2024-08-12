package mocrelay

type EventMatcher interface {
	Match(*Event) bool
}

type EventLimitMatcher interface {
	EventMatcher
	LimitMatch(*Event) bool
	Done() bool
}

type EventLimitMatchers[T EventLimitMatcher] []T

func (m EventLimitMatchers[T]) Match(event *Event) bool {
	match := false
	for _, mm := range m {
		match = mm.Match(event) || match
	}
	return match
}

func (m EventLimitMatchers[T]) LimitMatch(event *Event) bool {
	match := false
	for _, mm := range m {
		match = mm.LimitMatch(event) || match
	}
	return match
}

func (m EventLimitMatchers[T]) Done() bool {
	done := true
	for _, mm := range m {
		done = done && mm.Done()
	}
	return done
}

var _ EventLimitMatcher = (*ReqFilterEventMatcher)(nil)

type ReqFilterEventMatcher struct {
	cnt int64
	f   struct {
		IDs     map[string]bool
		Authors map[string]bool
		Kinds   map[int64]bool
		Tags    map[string]map[string]bool
		Since   *int64
		Until   *int64
		Limit   *int64
	}
}

func NewReqFilterMatcher(filter *ReqFilter) *ReqFilterEventMatcher {
	if filter == nil {
		panic("filter must be non-nil pointer")
	}

	ret := new(ReqFilterEventMatcher)

	if filter.IDs != nil {
		ret.f.IDs = make(map[string]bool)
		for _, id := range filter.IDs {
			ret.f.IDs[id] = true
		}
	}

	if filter.Authors != nil {
		ret.f.Authors = make(map[string]bool)
		for _, author := range filter.Authors {
			ret.f.Authors[author] = true
		}
	}

	if filter.Kinds != nil {
		ret.f.Kinds = make(map[int64]bool)
		for _, kind := range filter.Kinds {
			ret.f.Kinds[kind] = true
		}
	}

	if filter.Tags != nil {
		ret.f.Tags = make(map[string]map[string]bool)
		for tag, vals := range filter.Tags {
			m := make(map[string]bool)
			for _, val := range vals {
				m[val] = true
			}
			ret.f.Tags[tag] = m
		}
	}

	ret.f.Since = filter.Since
	ret.f.Until = filter.Until
	ret.f.Limit = filter.Limit

	return ret
}

func (m *ReqFilterEventMatcher) Match(event *Event) bool {
	if m.f.IDs != nil && !m.f.IDs[event.ID] {
		return false
	}

	if m.f.Kinds != nil && !m.f.Kinds[event.Kind] {
		return false
	}

	if m.f.Authors != nil && !m.f.Authors[event.Pubkey] {
		return false
	}

	if m.f.Tags != nil {
		found := make(map[string]bool)
		for _, tag := range event.Tags {
			if found[tag[0]] {
				continue
			}

			var v string
			if len(tag) >= 2 {
				v = tag[1]
			}
			if m.f.Tags[tag[0]][v] {
				found[tag[0]] = true
			}
		}
		if len(found) < len(m.f.Tags) {
			return false
		}
	}

	if m.f.Since != nil {
		if event.CreatedAt < *m.f.Since {
			return false
		}
	}

	if m.f.Until != nil {
		if *m.f.Until < event.CreatedAt {
			return false
		}
	}

	return true
}

func (m *ReqFilterEventMatcher) LimitMatch(event *Event) bool {
	match := m.Match(event)
	if match {
		m.cnt++
	}
	return match
}

func (m *ReqFilterEventMatcher) Done() bool {
	return m.f.Limit != nil && *m.f.Limit <= m.cnt
}

type ReqFiltersMatcher []*ReqFilterEventMatcher

func NewReqFiltersEventMatchers(
	filters []*ReqFilter,
) EventLimitMatchers[*ReqFilterEventMatcher] {
	if filters == nil {
		panic("filters must be non-nil slice")
	}
	ret := make([]*ReqFilterEventMatcher, len(filters))
	for i, f := range filters {
		ret[i] = NewReqFilterMatcher(f)
	}
	return ret
}
