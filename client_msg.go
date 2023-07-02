package main

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	"github.com/tidwall/gjson"
)

func ParseClientMsgJSON(json string) (ClientMsgJSON, error) {
	if !gjson.Valid(json) {
		return nil, fmt.Errorf("not a json: %q", json)
	}

	arr := gjson.Parse(json).Array()
	if len(arr) < 2 {
		return nil, fmt.Errorf("too short json array: %q", json)
	}

	if arr[0].Type != gjson.String {
		return nil, fmt.Errorf("client msg arr[0] type is not string: %q", json)
	}

	parsed := make([]interface{}, len(arr)-1)

	for idx, elem := range arr[1:] {
		switch arr[0].Str {
		case "EVENT":
			if idx > 0 {
				return nil, fmt.Errorf("invalid event msg: %q", json)
			}
			ev, err := ParseEventJSON(elem.Raw)
			if err != nil {
				return nil, fmt.Errorf("invalid event json: %w", err)
			}
			parsed[idx] = ev

		case "REQ":
			if idx == 0 {
				if elem.Type != gjson.String {
					return nil, fmt.Errorf("invalid req msg: %q", json)
				}
				parsed[idx] = elem.Str
			} else {
				fil, err := ParseFilterJSON(elem.Raw)
				if err != nil {
					return nil, fmt.Errorf("invalid filter json: %w", err)
				}
				parsed[idx] = fil
			}

		case "CLOSE":
			if idx > 0 {
				return nil, fmt.Errorf("invalid close msg: %q", json)
			}
			if elem.Type != gjson.String {
				return nil, fmt.Errorf("invalid close msg: %q", json)
			}
			parsed[idx] = elem.Str

		default:
			return nil, fmt.Errorf("unknown msg type: %q", json)
		}
	}

	switch arr[0].Str {
	case "EVENT":
		return &ClientEventMsgJSON{EventJSON: parsed[0].(*EventJSON)}, nil

	case "REQ":
		if len(parsed) < 2 {
			return nil, fmt.Errorf("invalid req msg: %q", json)
		}
		filters := make([]*FilterJSON, len(parsed)-1)
		for idx, elem := range parsed[1:] {
			filters[idx] = elem.(*FilterJSON)
		}
		return &ClientReqMsgJSON{SubscriptionID: parsed[0].(string), FilterJSONs: filters}, nil

	case "CLOSE":
		return &ClientCloseMsgJSON{SubscriptionID: parsed[0].(string)}, nil

	default:
		panic("unreachable")
	}
}

type ClientMsgJSON interface {
	clientMsgJSON()
}

type ClientEventMsgJSON struct {
	EventJSON *EventJSON
}

func (ClientEventMsgJSON) clientMsgJSON() {}

func ParseEventJSON(json string) (*EventJSON, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	var ev EventJSON
	if err := ji.UnmarshalFromString(json, &ev); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event json: %q", err)
	}

	return &ev, nil
}

type EventJSON struct {
	ID        string     `json:"id"`
	Pubkey    string     `json:"pubkey"`
	CreatedAt int        `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

type ClientReqMsgJSON struct {
	SubscriptionID string
	FilterJSONs    []*FilterJSON
}

func (ClientReqMsgJSON) clientMsgJSON() {}

func ParseFilterJSON(json string) (*FilterJSON, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	var fil FilterJSON
	if err := ji.UnmarshalFromString(json, &fil); err != nil {
		return nil, fmt.Errorf("failed to unmarshal filter json: %q", err)
	}

	return &fil, nil
}

type FilterJSON struct {
	IDs     []string `json:"ids"`
	Authors []string `json:"authors"`
	Kinds   []int    `json:"kinds"`
	Etags   []string `json:"#e"`
	Ptags   []string `json:"#p"`
	Since   int      `json:"since"`
	Until   int      `json:"until"`
	Limit   int      `json:"limit"`
}

type ClientCloseMsgJSON struct {
	SubscriptionID string
}

func (ClientCloseMsgJSON) clientMsgJSON() {}
