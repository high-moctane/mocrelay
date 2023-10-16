package mocrelay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

var ErrInvalidClientMsg = errors.New("invalid client message")

type ClientMsg interface {
	ClientMsg()
}

func IsNilClientMsg(msg ClientMsg) bool {
	return msg == nil || reflect.ValueOf(msg).IsNil()
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

func (*ClientUnknownMsg) ClientMsg() {}

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

func (*ClientEventMsg) ClientMsg() {}

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

func (*ClientReqMsg) ClientMsg() {}

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

func (*ClientCloseMsg) ClientMsg() {}

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

func (*ClientAuthMsg) ClientMsg() {}

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

func (*ClientCountMsg) ClientMsg() {}

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

type ServerMsg interface {
	ServerMsg()
	MarshalJSON() ([]byte, error)
}

func IsNilServerMsg(msg ServerMsg) bool {
	return msg == nil || reflect.ValueOf(msg).IsNil()
}

type ServerEOSEMsg struct {
	SubscriptionID string
}

func NewServerEOSEMsg(subID string) *ServerEOSEMsg {
	return &ServerEOSEMsg{
		SubscriptionID: subID,
	}
}

func (*ServerEOSEMsg) ServerMsg() {}

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

func NewServerEventMsg(subID string, event *Event) *ServerEventMsg {
	ret := &ServerEventMsg{
		SubscriptionID: subID,
		Event:          event,
	}
	return ret
}

func (*ServerEventMsg) ServerMsg() {}

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

func (*ServerNoticeMsg) ServerMsg() {}

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
	EventID   string
	Accepted  bool
	Msg       string
	MsgPrefix string
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

func NewServerOKMsg(eventID string, accepted bool, prefix, msg string) *ServerOKMsg {
	return &ServerOKMsg{
		EventID:   eventID,
		Accepted:  accepted,
		MsgPrefix: prefix,
		Msg:       msg,
	}
}

func (*ServerOKMsg) ServerMsg() {}

func (msg *ServerOKMsg) Message() string {
	return msg.MsgPrefix + msg.Msg
}

var ErrMarshalServerOKMsg = errors.New("failed to marshal server ok msg")

func (msg *ServerOKMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerOKMsg
	}

	v := [4]interface{}{"OK", msg.EventID, msg.Accepted, msg.Message()}
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

func (*ServerAuthMsg) ServerMsg() {}

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

func (*ServerCountMsg) ServerMsg() {}

var ErrMarshalServerCountMsg = errors.New("failed to marshal server count msg")

func (msg *ServerCountMsg) MarshalJSON() ([]byte, error) {
	if msg == nil {
		return nil, ErrMarshalServerCountMsg
	}

	type payload struct {
		Count       uint64 `json:"count"`
		Approximate *bool  `json:"approximate,omitempty"`
	}

	v := [3]interface{}{
		"COUNT",
		msg.SubscriptionID,
		payload{Count: msg.Count, Approximate: msg.Approximate},
	}
	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrMarshalServerCountMsg)
	}

	return ret, err
}

type EventType int

const (
	EventTypeUnknown EventType = iota
	EventTypeRegular
	EventTypeReplaceable
	EventTypeEphemeral
	EventTypeParamReplaceable
)

type Event struct {
	ID        string `json:"id"`
	Pubkey    string `json:"pubkey"`
	CreatedAt int64  `json:"created_at"`
	Kind      int64  `json:"kind"`
	Tags      []Tag  `json:"tags"`
	Content   string `json:"content"`
	Sig       string `json:"sig"`
}

var (
	ErrInvalidEvent = errors.New("invalid event")
	ErrNilEvent     = errors.New("nil event")
)

func ParseEvent(b []byte) (ev *Event, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, ErrInvalidEvent)
		}
	}()

	var obj map[string]json.RawMessage
	if err = json.Unmarshal(b, &obj); err != nil {
		return
	}

	var ret Event
	var v json.RawMessage
	var ok bool

	v, ok = obj["id"]
	if !ok {
		err = errors.New("id not found")
		return
	}
	if err = json.Unmarshal(v, &ret.ID); err != nil {
		return
	}

	v, ok = obj["pubkey"]
	if !ok {
		err = errors.New("pubkey not found")
		return
	}
	if err = json.Unmarshal(v, &ret.Pubkey); err != nil {
		return
	}

	v, ok = obj["created_at"]
	if !ok {
		err = errors.New("created_at not found")
		return
	}
	if err = json.Unmarshal(v, &ret.CreatedAt); err != nil {
		return
	}

	v, ok = obj["kind"]
	if !ok {
		err = errors.New("kind not found")
		return
	}
	if err = json.Unmarshal(v, &ret.Kind); err != nil {
		return
	}

	v, ok = obj["tags"]
	if !ok {
		err = errors.New("tags not found")
		return
	}
	if err = json.Unmarshal(v, &ret.Tags); err != nil {
		return
	}

	v, ok = obj["content"]
	if !ok {
		err = errors.New("content not found")
		return
	}
	if err = json.Unmarshal(v, &ret.Content); err != nil {
		return
	}

	v, ok = obj["sig"]
	if !ok {
		err = errors.New("sig not found")
		return
	}
	if err = json.Unmarshal(v, &ret.Sig); err != nil {
		return
	}

	ev = &ret

	return
}

var ErrMarshalEvent = errors.New("failed to marshal event")

func (ev *Event) MarshalJSON() ([]byte, error) {
	if ev == nil {
		return nil, ErrMarshalEvent
	}

	type alias Event
	ret, err := json.Marshal(alias(*ev))
	if err != nil {
		err = errors.Join(err, ErrMarshalEvent)
	}
	return ret, err
}

func (ev *Event) EventType() EventType {
	if kind := ev.Kind; kind == 0 || kind == 3 || 10000 <= kind && kind < 20000 {
		return EventTypeReplaceable
	} else if 20000 <= kind && kind < 30000 {
		return EventTypeEphemeral
	} else if 30000 <= kind && kind < 40000 {
		return EventTypeParamReplaceable
	}
	return EventTypeRegular
}

func (ev *Event) Valid() bool {
	if ev == nil {
		fmt.Println("1")
		return false
	}
	if len(ev.ID) != 64 || !validHexString(ev.ID) {
		fmt.Println("2")
		return false
	}
	if len(ev.Pubkey) != 64 || !validHexString(ev.Pubkey) {
		fmt.Println("3")
		return false
	}
	if ev.Kind < 0 || 65535 < ev.Kind {
		fmt.Println("4")
		return false
	}
	if len(ev.Sig) != 128 || !validHexString(ev.Sig) {
		fmt.Println("5")
		return false
	}
	return true
}

var ErrEventSerialize = errors.New("failed to serialize event")

func (ev *Event) Serialize() ([]byte, error) {
	if ev == nil {
		return nil, fmt.Errorf("empty event: %w", ErrEventSerialize)
	}

	v := [6]interface{}{
		0,
		ev.Pubkey,
		ev.CreatedAt,
		ev.Kind,
		ev.Tags,
		ev.Content,
	}

	ret, err := json.Marshal(&v)
	if err != nil {
		err = errors.Join(err, ErrEventSerialize)
	}
	return ret, err
}

func (ev *Event) Verify() (bool, error) {
	if ev == nil {
		return false, ErrNilEvent
	}

	// Verify ID
	serialized, err := ev.Serialize()
	if err != nil {
		return false, err
	}

	idBin, err := hex.DecodeString(ev.ID)
	if err != nil {
		return false, fmt.Errorf("failed to decode id: %w", err)
	}

	hash := sha256.Sum256(serialized)

	if !bytes.Equal(idBin, hash[:]) {
		return false, nil
	}

	// Verify Sig
	pubkeyBin, err := hex.DecodeString(ev.Pubkey)
	if err != nil {
		return false, fmt.Errorf("failed to decode pubkey: %w", err)
	}

	pubkey, err := schnorr.ParsePubKey(pubkeyBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse pubkey: %w", err)
	}

	sigBin, err := hex.DecodeString(ev.Sig)
	if err != nil {
		return false, fmt.Errorf("failed to decode sig: %w", err)
	}

	sig, err := schnorr.ParseSignature(sigBin)
	if err != nil {
		return false, fmt.Errorf("failed to parse sig: %w", err)
	}

	return sig.Verify(idBin, pubkey), nil
}

func (ev *Event) CreatedAtTime() time.Time {
	if ev == nil {
		return time.Unix(0, 0)
	}
	return time.Unix(ev.CreatedAt, 0)
}

type Tag []string

type EventInvalidIDError struct {
	Correct, Actual string
}

func (e *EventInvalidIDError) Error() string {
	return fmt.Sprintf("correct event id is %q but %q", e.Correct, e.Actual)
}

type EventInvalidSigError struct {
	Correct, Actual string
}

func (e *EventInvalidSigError) Error() string {
	return fmt.Sprintf("correct event sig is %q but %q", e.Correct, e.Actual)
}
