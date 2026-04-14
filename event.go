//go:build goexperiment.jsonv2

package mocrelay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"slices"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// EventType represents the category of a Nostr event based on its kind.
type EventType int

const (
	// EventTypeRegular represents standard events (kind 1, 2, 4-44, 1000-9999).
	// All regular events are stored.
	EventTypeRegular EventType = iota

	// EventTypeReplaceable represents events where only the latest is kept per (pubkey, kind).
	// Kinds: 0, 3, 10000-19999.
	EventTypeReplaceable

	// EventTypeEphemeral represents events that are not stored.
	// Kinds: 20000-29999.
	EventTypeEphemeral

	// EventTypeAddressable represents events where only the latest is kept per (pubkey, kind, d-tag).
	// Kinds: 30000-39999.
	EventTypeAddressable
)

// Event represents a Nostr event as defined in NIP-01.
type Event struct {
	ID        string    `json:"id"`
	Pubkey    string    `json:"pubkey"`
	CreatedAt time.Time `json:"created_at,format:unix"`
	Kind      int64     `json:"kind"`
	Tags      []Tag     `json:"tags"`
	Content   string    `json:"content"`
	Sig       string    `json:"sig"`
}

// Tag represents a tag in a Nostr event.
// The first element is the tag name, followed by optional values.
type Tag []string

// UnmarshalJSONFrom implements json.UnmarshalerFrom to reject null elements.
// NIP-01 requires tags to be "arrays of non-null strings".
func (t *Tag) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != '[' {
		return fmt.Errorf("expected array for tag, got %v", tok.Kind())
	}

	*t = (*t)[:0]
	for dec.PeekKind() != ']' {
		if dec.PeekKind() == 'n' {
			return fmt.Errorf("tag elements must be non-null strings")
		}
		var s string
		if err := json.UnmarshalDecode(dec, &s); err != nil {
			return fmt.Errorf("invalid tag element: %w", err)
		}
		*t = append(*t, s)
	}

	if _, err := dec.ReadToken(); err != nil { // ]
		return err
	}

	return nil
}

// Key returns the tag name (first element).
func (t Tag) Key() string {
	if len(t) < 1 {
		return ""
	}
	return t[0]
}

// Value returns the first value of the tag (second element).
func (t Tag) Value() string {
	if len(t) < 2 {
		return ""
	}
	return t[1]
}

// MarshalJSONTo implements json.MarshalerTo to ensure NIP-01 compliant output.
// Tags is always marshaled as [] (never null).
func (e Event) MarshalJSONTo(enc *jsontext.Encoder) error {
	type EventAlias Event
	alias := EventAlias(e)
	if alias.Tags == nil {
		alias.Tags = []Tag{}
	}
	return json.MarshalEncode(enc, &alias)
}

// UnmarshalJSONFrom implements json.UnmarshalerFrom to validate field count
// and field value types. Nostr events must have exactly 7 fields, all non-null.
func (e *Event) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	val, err := dec.ReadValue()
	if err != nil {
		return err
	}

	// Count fields and validate value types
	count := 0
	tempDec := jsontext.NewDecoder(bytes.NewReader(val))
	tok, err := tempDec.ReadToken()
	if err != nil {
		return fmt.Errorf("failed to read token: %w", err)
	}
	if tok.Kind() != '{' {
		return fmt.Errorf("expected object, got %v", tok.Kind())
	}
	for {
		tok, err := tempDec.ReadToken()
		if err != nil {
			return fmt.Errorf("failed to read token: %w", err)
		}
		if tok.Kind() == '}' {
			break
		}
		key := tok.String()
		count++

		raw, err := tempDec.ReadValue()
		if err != nil {
			return fmt.Errorf("failed to read value for %s: %w", key, err)
		}

		// All fields must be non-null
		if string(raw) == "null" {
			return fmt.Errorf("field %q must not be null", key)
		}

		// created_at must be a plain integer (no decimal point or exponent)
		if key == "created_at" && bytes.ContainsAny(raw, ".eE") {
			return fmt.Errorf("created_at must be an integer")
		}
	}

	if count != 7 {
		return fmt.Errorf("event must have exactly 7 fields, got %d", count)
	}

	// Unmarshal with strict options
	type EventAlias Event
	alias := (*EventAlias)(e)
	return json.Unmarshal(val, alias, json.RejectUnknownMembers(true))
}

// EventType returns the type of the event based on its kind.
func (e *Event) EventType() EventType {
	kind := e.Kind
	switch {
	case kind == 0 || kind == 3 || (10000 <= kind && kind < 20000):
		return EventTypeReplaceable
	case 20000 <= kind && kind < 30000:
		return EventTypeEphemeral
	case 30000 <= kind && kind < 40000:
		return EventTypeAddressable
	default:
		return EventTypeRegular
	}
}

// Valid checks if the event has valid format (not cryptographic validity).
func (e *Event) Valid() bool {
	if e == nil {
		return false
	}

	// ID: 32-bytes lowercase hex (NIP-01)
	if !isValidLowercaseHex(e.ID, 64) {
		return false
	}

	// Pubkey: 32-bytes lowercase hex (NIP-01)
	if !isValidLowercaseHex(e.Pubkey, 64) {
		return false
	}

	// Kind: integer between 0 and 65535 (NIP-01)
	if e.Kind < 0 || e.Kind > 65535 {
		return false
	}

	// Tags: each tag must have at least one element
	if e.Tags == nil {
		return false
	}
	for _, tag := range e.Tags {
		if len(tag) < 1 || tag[0] == "" {
			return false
		}
	}

	// Sig: 64-bytes lowercase hex (NIP-01)
	if !isValidLowercaseHex(e.Sig, 128) {
		return false
	}

	return true
}

// Serialize returns the canonical JSON representation for signing/verification.
// Format: [0, pubkey, created_at, kind, tags, content]
func (e *Event) Serialize() ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("nil event")
	}

	// Build the array manually for precise control
	var buf bytes.Buffer
	buf.WriteByte('[')
	buf.WriteByte('0')
	buf.WriteByte(',')

	// pubkey
	pubkeyJSON, err := json.Marshal(e.Pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pubkey: %w", err)
	}
	buf.Write(pubkeyJSON)
	buf.WriteByte(',')

	// created_at (as unix timestamp)
	fmt.Fprintf(&buf, "%d", e.CreatedAt.Unix())
	buf.WriteByte(',')

	// kind
	fmt.Fprintf(&buf, "%d", e.Kind)
	buf.WriteByte(',')

	// tags
	tagsJSON, err := json.Marshal(e.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}
	buf.Write(tagsJSON)
	buf.WriteByte(',')

	// content
	contentJSON, err := json.Marshal(e.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}
	buf.Write(contentJSON)

	buf.WriteByte(']')

	return buf.Bytes(), nil
}

// Verify checks if the event ID and signature are cryptographically valid.
func (e *Event) Verify() (bool, error) {
	if e == nil {
		return false, fmt.Errorf("nil event")
	}

	// Verify ID
	serialized, err := e.Serialize()
	if err != nil {
		return false, fmt.Errorf("failed to serialize: %w", err)
	}

	hash := sha256.Sum256(serialized)
	expectedID := hex.EncodeToString(hash[:])
	if e.ID != expectedID {
		return false, nil
	}

	// Verify signature
	pubkeyBytes, err := hex.DecodeString(e.Pubkey)
	if err != nil {
		return false, fmt.Errorf("failed to decode pubkey: %w", err)
	}

	pubkey, err := schnorr.ParsePubKey(pubkeyBytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse pubkey: %w", err)
	}

	sigBytes, err := hex.DecodeString(e.Sig)
	if err != nil {
		return false, fmt.Errorf("failed to decode sig: %w", err)
	}

	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse sig: %w", err)
	}

	idBytes, err := hex.DecodeString(e.ID)
	if err != nil {
		return false, fmt.Errorf("failed to decode id: %w", err)
	}

	return sig.Verify(idBytes, pubkey), nil
}

// Address returns the address for replaceable/addressable events.
// Format: "kind:pubkey:" for replaceable, "kind:pubkey:d-tag" for addressable.
// Returns empty string for regular/ephemeral events.
func (e *Event) Address() string {
	if e == nil {
		return ""
	}

	switch e.EventType() {
	case EventTypeReplaceable:
		return fmt.Sprintf("%d:%s:", e.Kind, e.Pubkey)

	case EventTypeAddressable:
		idx := slices.IndexFunc(e.Tags, func(t Tag) bool {
			return len(t) >= 1 && t[0] == "d"
		})
		d := ""
		if idx >= 0 && len(e.Tags[idx]) > 1 {
			d = e.Tags[idx][1]
		}
		return fmt.Sprintf("%d:%s:%s", e.Kind, e.Pubkey, d)

	default:
		return ""
	}
}
