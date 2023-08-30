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
	ret, err := json.Marshal(v)
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
	ret, err := json.Marshal(v)
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
	ret, err := json.Marshal(v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerNoticeMsg)
	}

	return ret, err
}
