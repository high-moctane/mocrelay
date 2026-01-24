//go:build goexperiment.jsonv2

package mocrelay

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"regexp"
)

// Message type labels
const (
	MsgTypeEvent  = "EVENT"
	MsgTypeReq    = "REQ"
	MsgTypeClose  = "CLOSE"
	MsgTypeAuth   = "AUTH"
	MsgTypeCount  = "COUNT"
	MsgTypeOK     = "OK"
	MsgTypeEOSE   = "EOSE"
	MsgTypeClosed = "CLOSED"
	MsgTypeNotice = "NOTICE"
)

// ClientMsg represents a message from client to relay.
// This is a union type - check Type field to determine which fields are valid.
type ClientMsg struct {
	Type string

	// EVENT, AUTH: the event being submitted
	Event *Event

	// REQ, CLOSE, COUNT: subscription identifier
	SubscriptionID string

	// REQ, COUNT: filters for the subscription
	Filters []*ReqFilter
}

// clientMsgLabelRegexp extracts the message type from JSON array.
var clientMsgLabelRegexp = regexp.MustCompile(`^\[\s*"(\w+)"`)

// ParseClientMsg parses a JSON message from client.
func ParseClientMsg(data []byte) (*ClientMsg, error) {
	match := clientMsgLabelRegexp.FindSubmatch(data)
	if match == nil {
		return nil, fmt.Errorf("invalid client message format")
	}

	msgType := string(match[1])
	switch msgType {
	case MsgTypeEvent:
		return parseClientEventMsg(data)
	case MsgTypeReq:
		return parseClientReqMsg(data)
	case MsgTypeClose:
		return parseClientCloseMsg(data)
	case MsgTypeAuth:
		return parseClientAuthMsg(data)
	case MsgTypeCount:
		return parseClientCountMsg(data)
	default:
		return nil, fmt.Errorf("unknown client message type: %s", msgType)
	}
}

func parseClientEventMsg(data []byte) (*ClientMsg, error) {
	// ["EVENT", <event>]
	dec := jsontext.NewDecoder(bytes.NewReader(data))

	if _, err := dec.ReadToken(); err != nil { // [
		return nil, err
	}
	if _, err := dec.ReadToken(); err != nil { // "EVENT"
		return nil, err
	}

	var ev Event
	if err := json.UnmarshalDecode(dec, &ev); err != nil {
		return nil, fmt.Errorf("failed to parse event: %w", err)
	}

	if _, err := dec.ReadToken(); err != nil { // ]
		return nil, err
	}

	return &ClientMsg{
		Type:  MsgTypeEvent,
		Event: &ev,
	}, nil
}

func parseClientReqMsg(data []byte) (*ClientMsg, error) {
	// ["REQ", <subscription_id>, <filter>, ...]
	dec := jsontext.NewDecoder(bytes.NewReader(data))

	if _, err := dec.ReadToken(); err != nil { // [
		return nil, err
	}
	if _, err := dec.ReadToken(); err != nil { // "REQ"
		return nil, err
	}

	var subID string
	if err := json.UnmarshalDecode(dec, &subID); err != nil {
		return nil, fmt.Errorf("failed to parse subscription id: %w", err)
	}

	var filters []*ReqFilter
	for dec.PeekKind() != ']' {
		var f ReqFilter
		if err := json.UnmarshalDecode(dec, &f); err != nil {
			return nil, fmt.Errorf("failed to parse filter: %w", err)
		}
		filters = append(filters, &f)
	}

	if _, err := dec.ReadToken(); err != nil { // ]
		return nil, err
	}

	if len(filters) == 0 {
		return nil, fmt.Errorf("REQ must have at least one filter")
	}

	return &ClientMsg{
		Type:           MsgTypeReq,
		SubscriptionID: subID,
		Filters:        filters,
	}, nil
}

func parseClientCloseMsg(data []byte) (*ClientMsg, error) {
	// ["CLOSE", <subscription_id>]
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, err
	}
	if len(arr) != 2 {
		return nil, fmt.Errorf("CLOSE must have exactly 2 elements")
	}

	return &ClientMsg{
		Type:           MsgTypeClose,
		SubscriptionID: arr[1],
	}, nil
}

func parseClientAuthMsg(data []byte) (*ClientMsg, error) {
	// ["AUTH", <event>]
	dec := jsontext.NewDecoder(bytes.NewReader(data))

	if _, err := dec.ReadToken(); err != nil { // [
		return nil, err
	}
	if _, err := dec.ReadToken(); err != nil { // "AUTH"
		return nil, err
	}

	var ev Event
	if err := json.UnmarshalDecode(dec, &ev); err != nil {
		return nil, fmt.Errorf("failed to parse auth event: %w", err)
	}

	if _, err := dec.ReadToken(); err != nil { // ]
		return nil, err
	}

	return &ClientMsg{
		Type:  MsgTypeAuth,
		Event: &ev,
	}, nil
}

func parseClientCountMsg(data []byte) (*ClientMsg, error) {
	// ["COUNT", <subscription_id>, <filter>, ...]
	dec := jsontext.NewDecoder(bytes.NewReader(data))

	if _, err := dec.ReadToken(); err != nil { // [
		return nil, err
	}
	if _, err := dec.ReadToken(); err != nil { // "COUNT"
		return nil, err
	}

	var subID string
	if err := json.UnmarshalDecode(dec, &subID); err != nil {
		return nil, fmt.Errorf("failed to parse subscription id: %w", err)
	}

	var filters []*ReqFilter
	for dec.PeekKind() != ']' {
		var f ReqFilter
		if err := json.UnmarshalDecode(dec, &f); err != nil {
			return nil, fmt.Errorf("failed to parse filter: %w", err)
		}
		filters = append(filters, &f)
	}

	if _, err := dec.ReadToken(); err != nil { // ]
		return nil, err
	}

	if len(filters) == 0 {
		return nil, fmt.Errorf("COUNT must have at least one filter")
	}

	return &ClientMsg{
		Type:           MsgTypeCount,
		SubscriptionID: subID,
		Filters:        filters,
	}, nil
}

// ServerMsg represents a message from relay to client.
// This is a union type - check Type field to determine which fields are valid.
type ServerMsg struct {
	Type string

	// EVENT: subscription ID and event
	SubscriptionID string
	Event          *Event

	// OK: event ID, success flag, and message
	EventID  string
	Accepted bool
	Message  string

	// COUNT: count result
	Count       uint64
	Approximate *bool
}

// NewServerEventMsg creates an EVENT message.
func NewServerEventMsg(subID string, event *Event) *ServerMsg {
	return &ServerMsg{
		Type:           MsgTypeEvent,
		SubscriptionID: subID,
		Event:          event,
	}
}

// NewServerOKMsg creates an OK message.
func NewServerOKMsg(eventID string, accepted bool, message string) *ServerMsg {
	return &ServerMsg{
		Type:     MsgTypeOK,
		EventID:  eventID,
		Accepted: accepted,
		Message:  message,
	}
}

// NewServerEOSEMsg creates an EOSE message.
func NewServerEOSEMsg(subID string) *ServerMsg {
	return &ServerMsg{
		Type:           MsgTypeEOSE,
		SubscriptionID: subID,
	}
}

// NewServerClosedMsg creates a CLOSED message.
func NewServerClosedMsg(subID string, message string) *ServerMsg {
	return &ServerMsg{
		Type:           MsgTypeClosed,
		SubscriptionID: subID,
		Message:        message,
	}
}

// NewServerNoticeMsg creates a NOTICE message.
func NewServerNoticeMsg(message string) *ServerMsg {
	return &ServerMsg{
		Type:    MsgTypeNotice,
		Message: message,
	}
}

// NewServerCountMsg creates a COUNT message.
func NewServerCountMsg(subID string, count uint64, approximate *bool) *ServerMsg {
	return &ServerMsg{
		Type:           MsgTypeCount,
		SubscriptionID: subID,
		Count:          count,
		Approximate:    approximate,
	}
}

// NewServerAuthMsg creates an AUTH message (challenge).
func NewServerAuthMsg(challenge string) *ServerMsg {
	return &ServerMsg{
		Type:    MsgTypeAuth,
		Message: challenge,
	}
}

// MarshalJSON implements json.Marshaler for ServerMsg.
func (m *ServerMsg) MarshalJSON() ([]byte, error) {
	switch m.Type {
	case MsgTypeEvent:
		// ["EVENT", <subscription_id>, <event>]
		return json.Marshal([]any{MsgTypeEvent, m.SubscriptionID, m.Event})

	case MsgTypeOK:
		// ["OK", <event_id>, <accepted>, <message>]
		return json.Marshal([]any{MsgTypeOK, m.EventID, m.Accepted, m.Message})

	case MsgTypeEOSE:
		// ["EOSE", <subscription_id>]
		return json.Marshal([]string{MsgTypeEOSE, m.SubscriptionID})

	case MsgTypeClosed:
		// ["CLOSED", <subscription_id>, <message>]
		return json.Marshal([]string{MsgTypeClosed, m.SubscriptionID, m.Message})

	case MsgTypeNotice:
		// ["NOTICE", <message>]
		return json.Marshal([]string{MsgTypeNotice, m.Message})

	case MsgTypeCount:
		// ["COUNT", <subscription_id>, {"count": <n>, "approximate": <bool>}]
		payload := map[string]any{"count": m.Count}
		if m.Approximate != nil {
			payload["approximate"] = *m.Approximate
		}
		return json.Marshal([]any{MsgTypeCount, m.SubscriptionID, payload})

	case MsgTypeAuth:
		// ["AUTH", <challenge>]
		return json.Marshal([]string{MsgTypeAuth, m.Message})

	default:
		return nil, fmt.Errorf("unknown server message type: %s", m.Type)
	}
}
