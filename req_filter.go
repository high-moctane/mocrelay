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

	if json.Ptags != nil {
		for _, id := range *json.Ptags {
			if len(id) < *Cfg.MinPrefix {
				return nil, NewReqFilterMinPrefixError("ptags", len(id))
			}
		}
	}

	if json.Etags != nil {
		for _, id := range *json.Etags {
			if len(id) < *Cfg.MinPrefix {
				return nil, NewReqFilterMinPrefixError("etags", len(id))
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
		fil.MatchEtags(event) &&
		fil.MatchPtags(event) &&
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

func (fil *Filter) MatchEtags(event *Event) bool {
	if fil == nil || fil.Etags == nil {
		return true
	}

	for _, id := range *fil.Etags {
		for _, tag := range event.Tags {
			if len(tag) < 2 {
				continue
			}
			if tag[0] == "e" && strings.HasPrefix(tag[1], id) {
				return true
			}
		}
	}
	return false
}

func (fil *Filter) MatchPtags(event *Event) bool {
	if fil == nil || fil.Ptags == nil {
		return true
	}

	for _, id := range *fil.Ptags {
		for _, tag := range event.Tags {
			if len(tag) < 2 {
				continue
			}
			if tag[0] == "p" && strings.HasPrefix(tag[1], id) {
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
