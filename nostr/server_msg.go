package nostr

import (
	"encoding/json"
	"errors"
)

type ServerMsgType int

const (
	ServerMsgTypeUnknown ServerMsgType = iota
	ServerMsgTypeEOSE
	ServerMsgTypeEvent
	ServerMsgTypeNotice
	ServerMsgTypeOK
	ServerMsgTypeAuth
	ServerMsgTypeCount
)

type ServerMsg interface {
	MsgType() ServerMsgType
	MarshalJSON() ([]byte, error)
}

type ServerEOSEMsg struct {
	SubscriptionID string
}

func NewServerEOSEMsg(subID string) *ServerEOSEMsg {
	return &ServerEOSEMsg{
		SubscriptionID: subID,
	}
}

func (*ServerEOSEMsg) MsgType() ServerMsgType {
	return ServerMsgTypeEOSE
}

var ErrMarshalServerEOSEMsg = errors.New("failed to marshal server eose msg")

func (msg *ServerEOSEMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerEOSEMsg
	}

	v := [2]string{"EOSE", msg.SubscriptionID}
	ret, err := json.Marshal(&v)
	if err != nil {
		return nil, errors.Join(err, ErrMarshalServerEOSEMsg)
	}

	return ret, nil
}

type ServerEventMsg struct {
	SubscriptionID string
	Event          *Event
}

var ErrServerEventMsgNilEvent = errors.New("server msg event must be non nil value")

func NewServerEventMsg(subID string, event *Event) (*ServerEventMsg, error) {
	if event == nil {
		return nil, ErrServerEventMsgNilEvent
	}
	ret := &ServerEventMsg{
		SubscriptionID: subID,
		Event:          event,
	}
	return ret, nil
}

func (*ServerEventMsg) MsgType() ServerMsgType {
	return ServerMsgTypeEvent
}

var ErrMarshalServerEventMsg = errors.New("failed to marshal server event msg")

func (msg *ServerEventMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerEventMsg
	}

	v := [3]interface{}{"EVENT", msg.SubscriptionID, msg.Event}
	ret, err := json.Marshal(&v)
	if err != nil {
		return nil, errors.Join(err, ErrMarshalServerEventMsg)
	}

	return ret, nil
}

type ServerNoticeMsg struct {
	Message string
}

func NewServerNoticeMsg(message string) *ServerNoticeMsg {
	return &ServerNoticeMsg{
		Message: message,
	}
}

func (*ServerNoticeMsg) MsgType() ServerMsgType {
	return ServerMsgTypeNotice
}

var ErrMarshalServerNoticeMsg = errors.New("failed to marshal server notice msg")

func (msg *ServerNoticeMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerNoticeMsg
	}

	v := [2]string{"NOTICE", msg.Message}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerNoticeMsg)
	}

	return ret, err
}

type ServerOKMsg struct {
	SubscriptionID string
	Accepted       bool
	Message        string
	MessagePrefix  string
}

const (
	ServerOKMsgPrefixNoPrefix    = ""
	ServerOKMsgPrefixPoW         = "pow: "
	ServerOKMsgPrefixDuplicate   = "duplicate: "
	ServerOkMsgPrefixBlocked     = "blocked: "
	ServerOkMsgPrefixRateLimited = "rate-limited: "
	ServerOkMsgPrefixRateInvalid = "invalid: "
	ServerOkMsgPrefixError       = "error: "
)

func NewServerOKMsg(subID string, accepted bool, prefix, msg string) *ServerOKMsg {
	return &ServerOKMsg{
		SubscriptionID: subID,
		Accepted:       accepted,
		MessagePrefix:  prefix,
		Message:        msg,
	}
}

func (*ServerOKMsg) MsgType() ServerMsgType {
	return ServerMsgTypeOK
}

var ErrMarshalServerOKMsg = errors.New("failed to marshal server ok msg")

func (msg *ServerOKMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerOKMsg
	}

	v := [4]interface{}{"OK", msg.SubscriptionID, msg.Accepted, msg.MessagePrefix + msg.Message}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerOKMsg)
	}

	return ret, err
}

type ServerAuthMsg struct {
	Event *Event
}

var ErrServerAuthMsgNilEvent = errors.New("server auth msg event must be non nil value")

func NewServerAuthMsg(event *Event) (*ServerAuthMsg, error) {
	if event == nil {
		return nil, ErrServerAuthMsgNilEvent
	}

	return &ServerAuthMsg{Event: event}, nil
}

func (*ServerAuthMsg) MsgType() ServerMsgType {
	return ServerMsgTypeAuth
}

var ErrMarshalServerAuthMsg = errors.New("failed to marshal server auth msg")

func (msg *ServerAuthMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerAuthMsg
	}

	v := [2]interface{}{"AUTH", msg.Event}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerAuthMsg)
	}

	return ret, err
}

type ServerCountMsg struct {
	SubscriptionID string
	Count          uint64
	Approximate    *bool
}

func NewServerCountMsg(subID string, count uint64, approx *bool) *ServerCountMsg {
	return &ServerCountMsg{
		SubscriptionID: subID,
		Count:          count,
		Approximate:    approx,
	}
}

func (*ServerCountMsg) MsgType() ServerMsgType {
	return ServerMsgTypeCount
}

var ErrMarshalServerCountMsg = errors.New("failed to marshal server count msg")

func (msg *ServerCountMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerCountMsg
	}

	type payload struct {
		Count       uint64 `json:"count"`
		Approximate *bool  `json:"approximate,omitempty"`
	}

	v := [3]interface{}{"COUNT", msg.SubscriptionID, payload{Count: msg.Count, Approximate: msg.Approximate}}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerCountMsg)
	}

	return ret, err
}
