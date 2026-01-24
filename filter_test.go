//go:build goexperiment.jsonv2

package mocrelay

import (
	"encoding/json/v2"
	"testing"
	"time"
)

func TestReqFilter_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(*ReqFilter) bool
	}{
		{
			name:    "empty filter",
			input:   `{}`,
			wantErr: false,
			check:   func(f *ReqFilter) bool { return f != nil },
		},
		{
			name:    "with ids",
			input:   `{"ids":["abc","def"]}`,
			wantErr: false,
			check:   func(f *ReqFilter) bool { return len(f.IDs) == 2 },
		},
		{
			name:    "with authors",
			input:   `{"authors":["abc"]}`,
			wantErr: false,
			check:   func(f *ReqFilter) bool { return len(f.Authors) == 1 },
		},
		{
			name:    "with kinds",
			input:   `{"kinds":[1,2,3]}`,
			wantErr: false,
			check:   func(f *ReqFilter) bool { return len(f.Kinds) == 3 },
		},
		{
			name:    "with tag filters",
			input:   `{"#e":["abc"],"#p":["def","ghi"]}`,
			wantErr: false,
			check: func(f *ReqFilter) bool {
				return len(f.Tags["e"]) == 1 && len(f.Tags["p"]) == 2
			},
		},
		{
			name:    "with since/until/limit",
			input:   `{"since":100,"until":200,"limit":10}`,
			wantErr: false,
			check: func(f *ReqFilter) bool {
				return f.Since != nil && *f.Since == 100 &&
					f.Until != nil && *f.Until == 200 &&
					f.Limit != nil && *f.Limit == 10
			},
		},
		{
			name:    "with search (NIP-50)",
			input:   `{"search":"hello world"}`,
			wantErr: false,
			check: func(f *ReqFilter) bool {
				return f.Search != nil && *f.Search == "hello world"
			},
		},
		{
			name:    "unknown field",
			input:   `{"unknown":"value"}`,
			wantErr: true,
		},
		{
			name:    "invalid tag key",
			input:   `{"#123":["abc"]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f ReqFilter
			err := json.Unmarshal([]byte(tt.input), &f)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil && !tt.check(&f) {
				t.Errorf("check failed for %s", tt.input)
			}
		})
	}
}

func TestReqFilter_MarshalJSON(t *testing.T) {
	f := &ReqFilter{
		IDs:     []string{"abc"},
		Authors: []string{"def"},
		Kinds:   []int64{1, 2},
		Tags:    map[string][]string{"e": {"xyz"}},
		Since:   ptr(int64(100)),
		Until:   ptr(int64(200)),
		Limit:   ptr(int64(10)),
	}

	got, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Unmarshal back and compare
	var f2 ReqFilter
	if err := json.Unmarshal(got, &f2); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(f2.IDs) != 1 || f2.IDs[0] != "abc" {
		t.Errorf("IDs mismatch: %v", f2.IDs)
	}
	if len(f2.Tags["e"]) != 1 || f2.Tags["e"][0] != "xyz" {
		t.Errorf("Tags mismatch: %v", f2.Tags)
	}
}

func TestReqFilter_Valid(t *testing.T) {
	tests := []struct {
		name   string
		filter *ReqFilter
		want   bool
	}{
		{
			name:   "nil filter",
			filter: nil,
			want:   false,
		},
		{
			name:   "empty filter",
			filter: &ReqFilter{Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "valid ids (64-char lowercase hex)",
			filter: &ReqFilter{IDs: []string{"abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"}, Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "invalid ids (non-hex)",
			filter: &ReqFilter{IDs: []string{"xyz!def1234567890abcdef1234567890abcdef1234567890abcdef1234567890"}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "invalid ids (uppercase)",
			filter: &ReqFilter{IDs: []string{"ABCDEF1234567890abcdef1234567890abcdef1234567890abcdef1234567890"}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "invalid ids (wrong length)",
			filter: &ReqFilter{IDs: []string{"abcdef"}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "empty ids array (NIP-01: one or more values)",
			filter: &ReqFilter{IDs: []string{}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "empty authors array (NIP-01: one or more values)",
			filter: &ReqFilter{Authors: []string{}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "empty kinds array (NIP-01: one or more values)",
			filter: &ReqFilter{Kinds: []int64{}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "empty tag values (NIP-01: one or more values)",
			filter: &ReqFilter{Tags: map[string][]string{"e": {}}},
			want:   false,
		},
		{
			name:   "negative kind",
			filter: &ReqFilter{Kinds: []int64{-1}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "since > until",
			filter: &ReqFilter{Since: ptr(int64(200)), Until: ptr(int64(100)), Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "negative limit",
			filter: &ReqFilter{Limit: ptr(int64(-1)), Tags: map[string][]string{}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReqFilter_Match(t *testing.T) {
	// Use 64-char lowercase hex for all IDs/pubkeys
	eventID := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	pubkey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	refEventID := "eeeeee1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	refPubkey := "aaaaaa1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	ev := &Event{
		ID:        eventID,
		Pubkey:    pubkey,
		CreatedAt: time.Unix(1000, 0),
		Kind:      1,
		Tags:      []Tag{{"e", refEventID}, {"p", refPubkey}},
		Content:   "hello",
	}

	tests := []struct {
		name   string
		filter *ReqFilter
		want   bool
	}{
		{
			name:   "empty filter matches all",
			filter: &ReqFilter{Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "id exact match",
			filter: &ReqFilter{IDs: []string{eventID}, Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "id no match",
			filter: &ReqFilter{IDs: []string{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff0000"}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "author exact match",
			filter: &ReqFilter{Authors: []string{pubkey}, Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "kind match",
			filter: &ReqFilter{Kinds: []int64{1, 2, 3}, Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "kind no match",
			filter: &ReqFilter{Kinds: []int64{2, 3}, Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "tag #e match",
			filter: &ReqFilter{Tags: map[string][]string{"e": {refEventID}}},
			want:   true,
		},
		{
			name:   "tag #e no match",
			filter: &ReqFilter{Tags: map[string][]string{"e": {"0000001234567890abcdef1234567890abcdef1234567890abcdef1234567890"}}},
			want:   false,
		},
		{
			name:   "since match",
			filter: &ReqFilter{Since: ptr(int64(500)), Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "since no match",
			filter: &ReqFilter{Since: ptr(int64(2000)), Tags: map[string][]string{}},
			want:   false,
		},
		{
			name:   "until match",
			filter: &ReqFilter{Until: ptr(int64(2000)), Tags: map[string][]string{}},
			want:   true,
		},
		{
			name:   "until no match",
			filter: &ReqFilter{Until: ptr(int64(500)), Tags: map[string][]string{}},
			want:   false,
		},
		{
			name: "combined filters",
			filter: &ReqFilter{
				Kinds:   []int64{1},
				Authors: []string{pubkey},
				Tags:    map[string][]string{"p": {refPubkey}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Match(ev); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidLowercaseHex(t *testing.T) {
	tests := []struct {
		s      string
		length int
		want   bool
	}{
		{"abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", 64, true},
		{"0000000000000000000000000000000000000000000000000000000000000000", 64, true},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 64, true},
		{"ABCDEF1234567890abcdef1234567890abcdef1234567890abcdef1234567890", 64, false}, // uppercase
		{"abcdef", 64, false}, // too short
		{"", 64, false},       // empty
		{"xyz!def1234567890abcdef1234567890abcdef1234567890abcdef1234567890", 64, false}, // non-hex
		{"abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcde", 64, false}, // 65 chars
	}

	for _, tt := range tests {
		if got := isValidLowercaseHex(tt.s, tt.length); got != tt.want {
			t.Errorf("isValidLowercaseHex(%q, %d) = %v, want %v", tt.s, tt.length, got, tt.want)
		}
	}
}
