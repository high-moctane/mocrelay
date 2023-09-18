package nostr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

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

var hexRegexp = regexp.MustCompile(`^[0-9a-f]$`)

func (ev *Event) Valid() bool {
	if ev == nil {
		return false
	}
	if len(ev.ID) != 32 || !hexRegexp.Match([]byte(ev.ID)) {
		return false
	}
	if len(ev.Pubkey) != 32 || !hexRegexp.Match([]byte(ev.Pubkey)) {
		return false
	}
	if ev.Kind < 0 || 65535 < ev.Kind {
		return false
	}
	if len(ev.Sig) != 64 || !hexRegexp.Match([]byte(ev.Sig)) {
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
