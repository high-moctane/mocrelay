package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
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

func (*ClientEventMsgJSON) clientMsgJSON() {}

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

func (e *EventJSON) Verify() (bool, error) {
	if e == nil {
		return false, errors.New("empty event cannot be verified")
	}

	ser, err := e.Serialize()
	if err != nil {
		return false, fmt.Errorf("failed to serialize event: %w", err)
	}

	// ID
	hash := sha256.Sum256(ser)

	idBin, err := hex.DecodeString(e.ID)
	if err != nil {
		return false, fmt.Errorf("invalid event id: %w", err)
	}

	if !bytes.Equal(hash[:], idBin) {
		return false, nil
	}

	// Sig
	pKeyBin, err := hex.DecodeString(e.Pubkey)
	if err != nil {
		return false, fmt.Errorf("failed to decode public key: %w", err)
	}

	pKey, err := schnorr.ParsePubKey(pKeyBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse public key: %w", err)
	}

	sigBin, err := hex.DecodeString(e.Sig)
	if err != nil {
		return false, fmt.Errorf("invalid event sig: %w", err)
	}

	sig, err := schnorr.ParseSignature(sigBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse event sig: %w", err)
	}

	return sig.Verify(hash[:], pKey), nil
}

func (e *EventJSON) Serialize() ([]byte, error) {
	if e == nil {
		return nil, errors.New("empty event json cannot be serialized")
	}

	arr := []interface{}{0, e.Pubkey, e.CreatedAt, e.Kind, e.Tags, e.Content}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(arr)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize event: %w", err)
	}

	return res, nil
}

type ClientReqMsgJSON struct {
	SubscriptionID string
	FilterJSONs    []*FilterJSON
}

func (*ClientReqMsgJSON) clientMsgJSON() {}

func ParseFilterJSON(json string) (*FilterJSON, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	var fil FilterJSON
	if err := ji.UnmarshalFromString(json, &fil); err != nil {
		return nil, fmt.Errorf("failed to unmarshal filter json: %q", err)
	}

	return &fil, nil
}

type FilterJSON struct {
	IDs     *[]string `json:"ids"`
	Authors *[]string `json:"authors"`
	Kinds   *[]int    `json:"kinds"`
	Etags   *[]string `json:"#e"`
	Ptags   *[]string `json:"#p"`
	Since   *int      `json:"since"`
	Until   *int      `json:"until"`
	Limit   *int      `json:"limit"`
}

type ClientCloseMsgJSON struct {
	SubscriptionID string
}

func (*ClientCloseMsgJSON) clientMsgJSON() {}

type ServerMsg interface {
	serverMsg()
	json.Marshaler
}

type ServerEventMsg struct {
	SubscriptionID string
	*EventJSON
}

func (ServerEventMsg) serverMsg() {}

func (msg *ServerEventMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, errors.New("cannot marshal nil server event msg")
	}

	payload := []interface{}{"EVENT", msg.SubscriptionID, msg.EventJSON}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal server event msg: %v", msg)
	}

	return res, nil
}

type ServerEOSEMsg struct {
	SubscriptionID string
}

func (ServerEOSEMsg) serverMsg() {}

func (msg *ServerEOSEMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, errors.New("cannot marshal nil server eose msg")
	}

	payload := []string{"EOSE", msg.SubscriptionID}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal eose msg: %v", msg)
	}

	return res, nil
}

type ServerNoticeMsg struct {
	Message string
}

func (ServerNoticeMsg) serverMsg() {}

func (msg *ServerNoticeMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, errors.New("cannot marshal nil server notice msg")
	}

	payload := []string{"NOTICE", msg.Message}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	res, err := ji.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notice msg: %v", msg)
	}

	return res, nil
}

type Event struct {
	*EventJSON
	ReceivedAt time.Time
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

func NewFiltersFromFilterJSONs(jsons []*FilterJSON) Filters {
	res := make(Filters, len(jsons))

	for i, json := range jsons {
		res[i] = &Filter{json}
	}

	return res
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
