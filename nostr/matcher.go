package nostr

import (
	"slices"
	"strings"
)

type Matcher struct {
	cnt int64
	f   *Filter
}

func NewMatcher(filter *Filter) *Matcher {
	if filter == nil {
		panic("filter must be non-nil pointer")
	}
	return &Matcher{
		cnt: 0,
		f:   filter,
	}
}

func (m *Matcher) Match(event *Event) bool {
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
				return slices.ContainsFunc(event.Tags, func(tagArr Tag) bool {
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

func (m *Matcher) CountMatch(event *Event) bool {
	match := m.Match(event)
	if match {
		m.cnt++
	}
	return match
}

func (m *Matcher) Count() int64 {
	return m.cnt
}

func (m *Matcher) Done() bool {
	return m.f.Limit != nil && *m.f.Limit <= m.cnt
}

type Matchers []*Matcher

func NewMatchers(filters Filters) Matchers {
	if filters == nil {
		panic("filters must be non-nil slice")
	}
	ret := make(Matchers, len(filters))
	for i, f := range filters {
		ret[i] = NewMatcher(f)
	}
	return ret
}

func (m Matchers) Match(event *Event) bool {
	match := false
	for _, mm := range m {
		match = mm.Match(event) || match
	}
	return match
}

func (m Matchers) CountMatch(event *Event) bool {
	match := false
	for _, mm := range m {
		match = mm.CountMatch(event) || match
	}
	return match
}

func (m Matchers) Count() int64 {
	var ret int64
	for _, mm := range m {
		ret = max(ret, mm.Count())
	}
	return ret
}

func (m Matchers) Done() bool {
	done := true
	for _, mm := range m {
		done = done && mm.Done()
	}
	return done
}
