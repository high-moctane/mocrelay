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
