package nostr

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

var ErrInvalidClientMsg = errors.New("invalid client message")

type ClientMsgType int

const (
	ClientMsgTypeUnknown ClientMsgType = iota
	ClientMsgTypeEvent
	ClientMsgTypeReq
	ClientMsgTypeClose
	ClientMsgTypeAuth
	ClientMsgTypeCount
)

type ClientMsg interface {
	MsgType() ClientMsgType
}

var clientMsgRegexp = regexp.MustCompile(`^\[\s*"(\w*)"`)

func ParseClientMsg(b []byte) (msg ClientMsg, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidClientMsg)
		}
	}()

	if !utf8.Valid(b) {
		return nil, errors.New("not a utf8 string")
	}
	if !json.Valid(b) {
		return nil, errors.New("not a valid json")
	}

	match := clientMsgRegexp.FindSubmatch(b)
	if len(match) == 0 {
		return nil, errors.New("not a client msg")
	}

	switch string(match[1]) {
	case "EVENT":
		return ParseClientEventMsg(b)

	case "REQ":
		return ParseClientReqMsg(b)

	case "CLOSE":
		return ParseClientCloseMsg(b)

	case "AUTH":
		return ParseClientAuthMsg(b)

	case "COUNT":
		return ParseClientCountMsg(b)

	default:
		return ParseClientUnknownMsg(b)
	}
}

var _ ClientMsg = (*ClientUnknownMsg)(nil)

type ClientUnknownMsg struct {
	MsgTypeStr string
	Msg        []interface{}
}

var ErrInvalidClientUnknownMsg = errors.New("invalid client message")

func ParseClientUnknownMsg(b []byte) (msg *ClientUnknownMsg, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidClientUnknownMsg)
		}
	}()

	var arr []interface{}

	if err = json.Unmarshal(b, &arr); err != nil {
		return
	}

	if len(arr) < 1 {
		err = fmt.Errorf("too short client message: len=%d", len(arr))
		return
	}

	s, ok := arr[0].(string)
	if !ok {
		err = errors.New("message json array[0] must be a string")
		return
	}

	msg = &ClientUnknownMsg{
		MsgTypeStr: s,
		Msg:        arr,
	}

	return
}

func (msg *ClientUnknownMsg) MsgType() ClientMsgType {
	return ClientMsgTypeUnknown
}

var _ ClientMsg = (*ClientEventMsg)(nil)

type ClientEventMsg struct {
	Event *Event
}

var ErrInvalidClientEventMsg = errors.New("invalid client event msg")

func ParseClientEventMsg(b []byte) (msg *ClientEventMsg, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidClientEventMsg)
		}
	}()

	var arr []json.RawMessage
	if err = json.Unmarshal(b, &arr); err != nil {
		return
	}
	if len(arr) != 2 {
		err = fmt.Errorf("client event msg len must be 2 but %d", len(arr))
		return
	}

	ev, err := ParseEvent(arr[1])
	if err != nil {
		return
	}

	msg = &ClientEventMsg{
		Event: ev,
	}
	return
}

func (*ClientEventMsg) MsgType() ClientMsgType {
	return ClientMsgTypeEvent
}

var _ ClientMsg = (*ClientReqMsg)(nil)

type ClientReqMsg struct {
	SubscriptionID string
	ReqFilters     []*ReqFilter
}

var ErrInvalidClientReqMsg = errors.New("invalid client req message")

func ParseClientReqMsg(b []byte) (msg *ClientReqMsg, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidClientReqMsg)
		}
	}()

	var arr []json.RawMessage
	if err = json.Unmarshal(b, &arr); err != nil {
		return
	}
	if len(arr) < 3 {
		err = fmt.Errorf("too short client req message array: len=%d", len(arr))
		return
	}

	var subID string
	if err = json.Unmarshal(arr[1], &subID); err != nil {
		return
	}

	filters := make([]*ReqFilter, 0, len(arr)-2)

	for i := 2; i < len(arr); i++ {
		var filter *ReqFilter
		filter, err = ParseReqFilter(arr[i])
		if err != nil {
			return
		}
		filters = append(filters, filter)
	}

	msg = &ClientReqMsg{
		SubscriptionID: subID,
		ReqFilters:     filters,
	}

	return
}

func (*ClientReqMsg) MsgType() ClientMsgType {
	return ClientMsgTypeReq
}

var _ ClientMsg = (*ClientCloseMsg)(nil)

var ErrInvalidClientCloseMsg = errors.New("invalid client close msg")

type ClientCloseMsg struct {
	SubscriptionID string
}

func ParseClientCloseMsg(b []byte) (msg *ClientCloseMsg, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidClientCloseMsg)
		}
	}()

	var arr []string
	if err = json.Unmarshal(b, &arr); err != nil {
		return
	}
	if len(arr) != 2 {
		err = fmt.Errorf("client close msg len must be 2 but %d", len(arr))
		return
	}

	msg = &ClientCloseMsg{
		SubscriptionID: arr[1],
	}
	return
}

func (*ClientCloseMsg) MsgType() ClientMsgType {
	return ClientMsgTypeClose
}

var _ ClientMsg = (*ClientAuthMsg)(nil)

type ClientAuthMsg struct {
	Challenge string
}

var ErrInvalidClientAuthMsg = errors.New("invalid client auth msg")

func ParseClientAuthMsg(b []byte) (msg *ClientAuthMsg, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidClientAuthMsg)
		}
	}()

	var arr []string
	if err = json.Unmarshal(b, &arr); err != nil {
		return
	}
	if len(arr) != 2 {
		err = fmt.Errorf("client auth msg len must be 2 but %d", len(arr))
		return
	}

	msg = &ClientAuthMsg{
		Challenge: arr[1],
	}

	return
}

func (*ClientAuthMsg) MsgType() ClientMsgType {
	return ClientMsgTypeAuth
}

var _ ClientMsg = (*ClientCountMsg)(nil)

type ClientCountMsg struct {
	SubscriptionID string
	ReqFilters     []*ReqFilter
}

var ErrInvalidClientCountMsg = errors.New("invalid client count msg")

func ParseClientCountMsg(b []byte) (msg *ClientCountMsg, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidClientCountMsg)
		}
	}()

	var arr []json.RawMessage
	if err = json.Unmarshal(b, &arr); err != nil {
		return
	}

	var subID string
	if err = json.Unmarshal(arr[1], &subID); err != nil {
		return
	}

	filters := make([]*ReqFilter, 0, len(arr)-2)
	for i := 2; i < len(arr); i++ {
		var filter *ReqFilter
		filter, err = ParseReqFilter(arr[i])
		if err != nil {
			return
		}
		filters = append(filters, filter)
	}

	msg = &ClientCountMsg{
		SubscriptionID: subID,
		ReqFilters:     filters,
	}

	return
}

func (*ClientCountMsg) MsgType() ClientMsgType {
	return ClientMsgTypeCount
}

type ReqFilter struct {
	IDs     *[]string
	Authors *[]string
	Kinds   *[]int64
	Tags    *map[string][]string
	Since   *int64
	Until   *int64
	Limit   *int64
}

var ErrInvalidReqFilter = errors.New("invalid filter")

var filterKeys = func() []string {
	var ret []string

	for r := 'A'; r <= 'Z'; r++ {
		ret = append(ret, string([]rune{'#', r}))
	}
	for r := 'a'; r <= 'z'; r++ {
		ret = append(ret, string([]rune{'#', r}))
	}

	return ret
}()

func ParseReqFilter(b []byte) (fil *ReqFilter, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidReqFilter)
		}
	}()

	var obj map[string]json.RawMessage
	if err = json.Unmarshal(b, &obj); err != nil {
		return
	}

	var ret ReqFilter
	var v json.RawMessage
	var ok bool

	if v, ok = obj["ids"]; ok {
		if err = json.Unmarshal(v, &ret.IDs); err != nil {
			return
		}
	}

	if v, ok = obj["authors"]; ok {
		if err = json.Unmarshal(v, &ret.Authors); err != nil {
			return
		}
	}

	if v, ok = obj["kinds"]; ok {
		if err = json.Unmarshal(v, &ret.Kinds); err != nil {
			return
		}
	}

	for _, k := range filterKeys {
		if v, ok = obj[k]; ok {
			var vals []string
			if err = json.Unmarshal(v, &vals); err != nil {
				return
			}
			if ret.Tags == nil {
				m := make(map[string][]string)
				ret.Tags = &m
			}

			(*ret.Tags)[k] = vals
		}
	}

	if v, ok = obj["since"]; ok {
		if err = json.Unmarshal(v, &ret.Since); err != nil {
			return
		}
	}

	if v, ok = obj["until"]; ok {
		if err = json.Unmarshal(v, &ret.Until); err != nil {
			return
		}
	}

	if v, ok = obj["limit"]; ok {
		if err = json.Unmarshal(v, &ret.Limit); err != nil {
			return
		}
	}

	fil = &ret

	return
}
