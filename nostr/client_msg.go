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
	Raw() []byte
}

var clientMsgRegexp = regexp.MustCompile(`^\[\s*"(EVENT|REQ|CLOSE|AUTH|COUNT)"`)

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
		panic("unimplemented")

	case "COUNT":
		panic("unimplemented")

	default:
		panic("unreachable")
	}
}

var _ ClientMsg = (*ClientEventMsg)(nil)

type ClientEventMsg struct {
	Event *Event

	raw []byte
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
		raw:   b,
	}
	return
}

func (*ClientEventMsg) MsgType() ClientMsgType {
	return ClientMsgTypeEvent
}

func (msg *ClientEventMsg) Raw() []byte {
	if msg == nil {
		return nil
	}
	return msg.raw
}

var _ ClientMsg = (*ClientReqMsg)(nil)

type ClientReqMsg struct {
	SubscriptionID string
	Filters        Filters

	raw []byte
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

	filters := make(Filters, 0, len(arr)-2)

	for i := 2; i < len(arr); i++ {
		var vi Filter
		if err = json.Unmarshal(arr[i], &vi); err != nil {
			return
		}
		filters = append(filters, &vi)
	}

	msg = &ClientReqMsg{
		SubscriptionID: subID,
		Filters:        filters,
		raw:            b,
	}

	return
}

func (*ClientReqMsg) MsgType() ClientMsgType {
	return ClientMsgTypeReq
}

func (msg *ClientReqMsg) Raw() []byte {
	if msg == nil {
		return nil
	}
	return msg.raw
}

var _ ClientMsg = (*ClientCloseMsg)(nil)

var ErrInvalidClientCloseMsg = errors.New("invalid client close msg")

type ClientCloseMsg struct {
	SubscriptionID string
	raw            []byte
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
		raw:            b,
	}
	return
}

func (*ClientCloseMsg) MsgType() ClientMsgType {
	return ClientMsgTypeClose
}

func (msg *ClientCloseMsg) Raw() []byte {
	if msg == nil {
		return nil
	}
	return msg.raw
}

var _ ClientMsg = (*ClientAuthMsg)(nil)

type ClientAuthMsg struct{}

func (*ClientAuthMsg) MsgType() ClientMsgType {
	return ClientMsgTypeAuth
}

func (*ClientAuthMsg) Raw() []byte {
	return nil
}

var _ ClientMsg = (*ClientCountMsg)(nil)

type ClientCountMsg struct{}

func (*ClientCountMsg) MsgType() ClientMsgType {
	return ClientMsgTypeCount
}

func (*ClientCountMsg) Raw() []byte {
	return nil
}

type Filter struct {
	IDs     *[]string
	Authors *[]string
	Kinds   *[]int64
	Tags    *map[string][]string
	Since   *int64
	Until   *int64
	Limit   *int64

	raw []byte
}

var ErrInvalidFilter = errors.New("invalid filter")

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

func ParseFilter(b []byte) (fil *Filter, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidFilter)
		}
	}()

	var obj map[string]json.RawMessage
	if err = json.Unmarshal(b, &obj); err != nil {
		return
	}

	var ret Filter
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

			(*ret.Tags)[string(k[1])] = vals
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

	ret.raw = b
	fil = &ret

	return
}

func (fil *Filter) Raw() []byte {
	if fil == nil {
		return nil
	}
	return fil.raw
}

type Filters []*Filter
