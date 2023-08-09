package main

import (
	"errors"
	"fmt"
	"strings"
)

type ReqFilterMinPrefixError struct {
	What   string
	Length int
}

func NewReqFilterMinPrefixError(what string, length int) *ReqFilterMinPrefixError {
	return &ReqFilterMinPrefixError{
		What:   what,
		Length: length,
	}
}

func (e *ReqFilterMinPrefixError) Error() string {
	return fmt.Sprintf("too short %s id prefix: min prefix should be %d or more", e.What, e.Length)
}

func NewFilter(json *FilterJSON) (*Filter, error) {
	if json == nil {
		return nil, errors.New("nil filter")
	}

	if json.IDs != nil {
		for _, id := range *json.IDs {
			if len(id) < *Cfg.MinPrefix {
				return nil, NewReqFilterMinPrefixError("ids", len(id))
			}
		}
	}

	if json.Authors != nil {
		for _, id := range *json.Authors {
			if len(id) < *Cfg.MinPrefix {
				return nil, NewReqFilterMinPrefixError("authors", len(id))
			}
		}
	}

	if json.Tags != nil {
		for _, tag := range []string{"#e", "#p"} {
			arr, ok := (*json.Tags)[tag]
			if !ok {
				continue
			}

			for _, id := range arr {
				if len(id) < *Cfg.MinPrefix {
					return nil, NewReqFilterMinPrefixError(tag, len(id))
				}
			}
		}
	}

	return &Filter{FilterJSON: json}, nil
}

type Filter struct {
	*FilterJSON
}

func (fil *Filter) Match(event *Event) bool {
	return fil.MatchIDs(event) &&
		fil.MatchAuthors(event) &&
		fil.MatchKinds(event) &&
		fil.MatchTags(event) &&
		fil.MatchSince(event) &&
		fil.MatchUntil(event)
}

func (fil *Filter) MatchIDs(event *Event) bool {
	if fil == nil || fil.IDs == nil {
		return true
	}

	for _, prefix := range *fil.IDs {
		if strings.HasPrefix(event.ID, prefix) {
			return true
		}
	}
	return false
}

func (fil *Filter) MatchAuthors(event *Event) bool {
	if fil == nil || fil.Authors == nil {
		return true
	}

	for _, prefix := range *fil.Authors {
		if strings.HasPrefix(event.Pubkey, prefix) {
			return true
		}
	}
	return false
}

func (fil *Filter) MatchKinds(event *Event) bool {
	if fil == nil || fil.Kinds == nil {
		return true
	}

	for _, k := range *fil.Kinds {
		if event.Kind == k {
			return true
		}
	}
	return false
}

func (fil *Filter) MatchTags(event *Event) bool {
	if fil == nil || fil.Tags == nil {
		return true
	}

	for tag := range *fil.Tags {
		if match := fil.MatchTag(tag, event); match {
			return true
		}
	}

	return false
}

func (fil *Filter) MatchTag(tag string, event *Event) bool {
	if fil == nil || fil.Tags == nil {
		return true
	}
	vs, ok := (*fil.Tags)[tag]
	if !ok {
		return true
	}

	eTags := event.GetTagsByName(tag[1:2])
	if len(eTags) == 0 {
		return false
	}

	for _, et := range eTags {
		for _, v := range vs {
			var target string
			if len(et) > 1 {
				target = et[1]
			}

			if strings.HasPrefix(target, v) {
				return true
			}
		}
	}

	return false
}

func (fil *Filter) MatchSince(event *Event) bool {
	return fil == nil || fil.Since == nil || event.CreatedAt > *fil.Since
}

func (fil *Filter) MatchUntil(event *Event) bool {
	return fil == nil || fil.Until == nil || event.CreatedAt < *fil.Until
}

func NewFiltersFromFilterJSONs(jsons []*FilterJSON) (Filters, error) {
	if len(jsons) > Cfg.MaxFilters+2 {
		return nil, fmt.Errorf("filter is too long: %v", jsons)
	}

	res := make(Filters, 0, len(jsons))

	var errs error
	for _, json := range jsons {
		fil, err := NewFilter(json)
		if err != nil {
			var target *ReqFilterMinPrefixError
			if errors.As(err, &target) {
				errs = errors.Join(errs, err)
				continue
			}
			return nil, err
		}
		res = append(res, fil)
	}

	return res, errs
}

type Filters []*Filter

func (fils Filters) Match(event *Event) bool {
	for _, fil := range fils {
		if fil.Match(event) {
			return true
		}
	}
	return false
}

type FilterCounter struct {
	*Filter
	count int
}

func NewFilterCounter(fil *Filter) *FilterCounter {
	return &FilterCounter{
		Filter: fil,
		count:  0,
	}
}

func (fil *FilterCounter) Done() bool {
	if fil.Filter.Limit == nil {
		return false
	}
	return fil.count >= *fil.Filter.Limit
}

func (fil *FilterCounter) Match(event *Event) bool {
	if fil.Done() {
		return false
	}
	if match := fil.Filter.Match(event); match {
		fil.count++
		return true
	}
	return false
}

type FiltersCounter []*FilterCounter

func NewFiltersCounter(fils Filters) FiltersCounter {
	res := make(FiltersCounter, len(fils))
	for i := 0; i < len(fils); i++ {
		res[i] = NewFilterCounter(fils[i])
	}
	return res
}

func (fils FiltersCounter) Done() bool {
	for _, fil := range fils {
		if !fil.Done() {
			return false
		}
	}
	return true
}

func (fils *FiltersCounter) Match(event *Event) bool {
	res := false
	for _, fil := range *fils {
		res = fil.Match(event) || res
	}
	return res
}
