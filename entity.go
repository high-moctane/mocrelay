package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
)

func NewEvent(json *EventJSON, receivedAt time.Time) *Event {
	return &Event{
		EventJSON:  json,
		ReceivedAt: receivedAt,
	}
}

type Event struct {
	*EventJSON
	ReceivedAt time.Time
}

func (e *Event) ValidCreatedAt() bool {
	sub := time.Until(e.CreatedAtToTime())
	// TODO(high-moctane) no magic number
	return -10*time.Minute <= sub && sub <= 5*time.Minute
}

func (e *Event) MarshalJSON() ([]byte, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary
	return ji.Marshal(e.EventJSON)
}

func NewFilter(json *FilterJSON) (*Filter, error) {
	if json == nil {
		return nil, errors.New("nil filter")
	}

	if json.IDs != nil {
		for _, id := range *json.IDs {
			if len(id) < *Cfg.MinPrefix {
				return nil, errors.New("too short id prefix")
			}
		}
	}

	if json.Authors != nil {
		for _, id := range *json.Authors {
			if len(id) < *Cfg.MinPrefix {
				return nil, errors.New("too short author id prefix")
			}
		}
	}

	if json.Ptags != nil {
		for _, id := range *json.Ptags {
			if len(id) < *Cfg.MinPrefix {
				return nil, errors.New("too short ptag id prefix")
			}
		}
	}

	if json.Etags != nil {
		for _, id := range *json.Etags {
			if len(id) < *Cfg.MinPrefix {
				return nil, errors.New("too short etag id prefix")
			}
		}
	}

	if (json.IDs == nil || len(*json.IDs) == 0) &&
		(json.Authors == nil || len(*json.Authors) == 0) &&
		(json.Ptags == nil || len(*json.Ptags) == 0) &&
		(json.Etags == nil || len(*json.Etags) == 0) {
		return nil, errors.New("empty ids, authors, #p, #e")
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

	res := make(Filters, len(jsons))

	var err error
	for i, json := range jsons {
		res[i], err = NewFilter(json)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
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
