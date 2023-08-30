package nostr

import (
	"bytes"
	"encoding/json"
	"errors"
)

type Event struct {
	ID        string `json:"id"`
	Pubkey    string `json:"pubkey"`
	CreatedAt int64  `json:"created_at"`
	Kind      int64  `json:"kind"`
	Tags      []Tag  `json:"tags"`
	Content   string `json:"content"`
	Sig       string `json:"sig"`

	raw []byte
}

var ErrInvalidEvent = errors.New("invalid event")

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

	ret.raw = b
	ev = &ret

	return
}

func (ev *Event) Raw() []byte {
	if ev == nil {
		return nil
	}
	return ev.raw
}

var ErrMarshalEvent = errors.New("failed to marshal event")

func (ev *Event) MarshalJSON() ([]byte, error) {
	if ev == nil {
		return nil, ErrMarshalEvent
	}

	if raw := ev.Raw(); raw != nil {
		buf := new(bytes.Buffer)
		err := json.Compact(buf, raw)
		if err != nil {
			err = errors.Join(err, ErrMarshalEvent)
		}
		return buf.Bytes(), err

	} else {
		type alias Event
		ret, err := json.Marshal(alias(*ev))
		if err != nil {
			err = errors.Join(err, ErrMarshalEvent)
		}
		return ret, err
	}
}

type Tag []string
