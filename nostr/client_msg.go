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
		panic("unimplemented")

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

type ClientReqMsg struct{}

func (*ClientReqMsg) MsgType() ClientMsgType {
	return ClientMsgTypeReq
}

func (*ClientReqMsg) Raw() []byte {
	return nil
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
