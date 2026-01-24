//go:build goexperiment.jsonv2

package mocrelay

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
)

// ReqFilter represents a filter in REQ/COUNT messages.
type ReqFilter struct {
	IDs     []string            `json:"ids,omitempty"`
	Authors []string            `json:"authors,omitempty"`
	Kinds   []int64             `json:"kinds,omitempty"`
	Tags    map[string][]string `json:"-"` // handled manually: #e, #p, etc.
	Since   *int64              `json:"since,omitempty"`
	Until   *int64              `json:"until,omitempty"`
	Limit   *int64              `json:"limit,omitempty"`
	Search  *string             `json:"search,omitempty"` // NIP-50
}

// UnmarshalJSONFrom implements json.UnmarshalerFrom for ReqFilter.
// This handles the dynamic tag fields (#e, #p, etc.) and rejects unknown fields.
func (f *ReqFilter) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	val, err := dec.ReadValue()
	if err != nil {
		return err
	}

	// Parse as map to handle dynamic keys
	tempDec := jsontext.NewDecoder(bytes.NewReader(val))
	tempDec.ReadToken() // {

	f.Tags = make(map[string][]string)

	for tempDec.PeekKind() != '}' {
		// Read key
		keyTok, err := tempDec.ReadToken()
		if err != nil {
			return err
		}
		key := keyTok.String()

		// Read value based on key
		switch key {
		case "ids":
			if err := json.UnmarshalDecode(tempDec, &f.IDs); err != nil {
				return fmt.Errorf("invalid ids: %w", err)
			}
		case "authors":
			if err := json.UnmarshalDecode(tempDec, &f.Authors); err != nil {
				return fmt.Errorf("invalid authors: %w", err)
			}
		case "kinds":
			if err := json.UnmarshalDecode(tempDec, &f.Kinds); err != nil {
				return fmt.Errorf("invalid kinds: %w", err)
			}
		case "since":
			if err := json.UnmarshalDecode(tempDec, &f.Since); err != nil {
				return fmt.Errorf("invalid since: %w", err)
			}
		case "until":
			if err := json.UnmarshalDecode(tempDec, &f.Until); err != nil {
				return fmt.Errorf("invalid until: %w", err)
			}
		case "limit":
			if err := json.UnmarshalDecode(tempDec, &f.Limit); err != nil {
				return fmt.Errorf("invalid limit: %w", err)
			}
		case "search":
			if err := json.UnmarshalDecode(tempDec, &f.Search); err != nil {
				return fmt.Errorf("invalid search: %w", err)
			}
		default:
			// Check if it's a tag filter (#a-z, #A-Z)
			if len(key) == 2 && key[0] == '#' && isTagLetter(key[1]) {
				var values []string
				if err := json.UnmarshalDecode(tempDec, &values); err != nil {
					return fmt.Errorf("invalid tag filter %s: %w", key, err)
				}
				f.Tags[string(key[1])] = values
			} else {
				return fmt.Errorf("unknown filter field: %s", key)
			}
		}
	}

	return nil
}

// MarshalJSON implements json.Marshaler for ReqFilter.
func (f *ReqFilter) MarshalJSON() ([]byte, error) {
	// Build a map for marshaling
	obj := make(map[string]any)

	if f.IDs != nil {
		obj["ids"] = f.IDs
	}
	if f.Authors != nil {
		obj["authors"] = f.Authors
	}
	if f.Kinds != nil {
		obj["kinds"] = f.Kinds
	}
	for k, v := range f.Tags {
		obj["#"+k] = v
	}
	if f.Since != nil {
		obj["since"] = *f.Since
	}
	if f.Until != nil {
		obj["until"] = *f.Until
	}
	if f.Limit != nil {
		obj["limit"] = *f.Limit
	}
	if f.Search != nil {
		obj["search"] = *f.Search
	}

	return json.Marshal(obj)
}

// Valid checks if the filter has valid format.
// Per NIP-01: list fields must have "one or more values" - empty arrays are invalid.
func (f *ReqFilter) Valid() bool {
	if f == nil {
		return false
	}

	// IDs: must be nil or non-empty, and each must be exact 64-char lowercase hex
	// NIP-01: "The ids ... filter lists MUST contain exact 64-character lowercase hex values"
	if f.IDs != nil && len(f.IDs) == 0 {
		return false
	}
	for _, id := range f.IDs {
		if !isValidLowercaseHex(id, 64) {
			return false
		}
	}

	// Authors: must be nil or non-empty, and each must be exact 64-char lowercase hex
	if f.Authors != nil && len(f.Authors) == 0 {
		return false
	}
	for _, author := range f.Authors {
		if !isValidLowercaseHex(author, 64) {
			return false
		}
	}

	// Kinds: must be nil or non-empty
	if f.Kinds != nil && len(f.Kinds) == 0 {
		return false
	}
	for _, kind := range f.Kinds {
		if kind < 0 {
			return false
		}
	}

	// Tags: must have single-letter keys and non-empty values
	// NIP-01: "The ... #e and #p filter lists MUST contain exact 64-character lowercase hex values"
	for k, v := range f.Tags {
		if len(k) != 1 || !isTagLetter(k[0]) {
			return false
		}
		// Tag filter values must be non-empty (NIP-01: "one or more values")
		if len(v) == 0 {
			return false
		}
		// #e and #p must be exact 64-char lowercase hex
		if k == "e" || k == "p" {
			for _, val := range v {
				if !isValidLowercaseHex(val, 64) {
					return false
				}
			}
		}
	}

	// Since/Until should be non-negative if present
	if f.Since != nil && *f.Since < 0 {
		return false
	}
	if f.Until != nil && *f.Until < 0 {
		return false
	}

	// Since should be <= Until if both present
	if f.Since != nil && f.Until != nil && *f.Since > *f.Until {
		return false
	}

	// Limit should be non-negative if present
	if f.Limit != nil && *f.Limit < 0 {
		return false
	}

	return true
}

// Match checks if an event matches this filter.
func (f *ReqFilter) Match(ev *Event) bool {
	if f == nil || ev == nil {
		return false
	}

	// IDs: exact match
	if len(f.IDs) > 0 {
		matched := false
		for _, id := range f.IDs {
			if ev.ID == id {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Authors: exact match
	if len(f.Authors) > 0 {
		matched := false
		for _, author := range f.Authors {
			if ev.Pubkey == author {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Kinds: exact match
	if len(f.Kinds) > 0 {
		matched := false
		for _, kind := range f.Kinds {
			if ev.Kind == kind {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Tags: match event tags
	for tagKey, filterValues := range f.Tags {
		if len(filterValues) == 0 {
			continue
		}
		matched := false
		for _, tag := range ev.Tags {
			if len(tag) >= 2 && tag[0] == tagKey {
				for _, fv := range filterValues {
					if tag[1] == fv {
						matched = true
						break
					}
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Since: created_at >= since
	if f.Since != nil {
		if ev.CreatedAt.Unix() < *f.Since {
			return false
		}
	}

	// Until: created_at <= until
	if f.Until != nil {
		if ev.CreatedAt.Unix() > *f.Until {
			return false
		}
	}

	return true
}

// isTagLetter checks if a byte is a valid tag letter (a-z or A-Z).
func isTagLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isValidLowercaseHex checks if a string is exactly length chars of lowercase hex.
// NIP-01 requires "exact 64-character lowercase hex values" for ids, authors, #e, #p.
func isValidLowercaseHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
