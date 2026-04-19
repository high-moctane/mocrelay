package mocrelay

import (
	"math"
	"testing"
)

func TestKindLabels(t *testing.T) {
	tests := []struct {
		name     string
		kind     int64
		wantKind string
		wantType string
	}{
		// Known kinds -> decimal form + NIP-01 category
		{"metadata", 0, "0", "replaceable"},
		{"note", 1, "1", "regular"},
		{"follows", 3, "3", "replaceable"},
		{"legacy_dm", 4, "4", "regular"},
		{"deletion", 5, "5", "regular"},
		{"repost", 6, "6", "regular"},
		{"reaction", 7, "7", "regular"},
		{"seal", 13, "13", "regular"},
		{"chat_message", 14, "14", "regular"},
		{"channel_create", 40, "40", "regular"},
		{"channel_metadata", 41, "41", "regular"},
		{"channel_message", 42, "42", "regular"},
		{"hide_message", 43, "43", "regular"},
		{"mute_user", 44, "44", "regular"},
		{"gift_wrap", 1059, "1059", "regular"},
		{"comment", 1111, "1111", "regular"},
		{"reporting", 1984, "1984", "regular"},
		{"zap_request", 9734, "9734", "regular"},
		{"zap_receipt", 9735, "9735", "regular"},
		{"relay_list_metadata", 10002, "10002", "replaceable"},
		{"dm_relay_list", 10050, "10050", "replaceable"},
		{"long_form_content", 30023, "30023", "addressable"},

		// Unknown kind, but classifiable type
		{"regular_2", 2, "other", "regular"},
		{"regular_upper_4_44", 44, "44", "regular"},             // sanity: 44 is in allowlist
		{"regular_outside_4_44_lower", 45, "other", "unknown"},  // 45 is in NIP-01 gap
		{"regular_outside_4_44_upper", 999, "other", "unknown"}, // 999 is in NIP-01 gap
		{"regular_1000_lower", 1000, "other", "regular"},        // 1000-9999 regular range
		{"regular_9999_upper", 9999, "other", "regular"},        // upper bound of regular range
		{"replaceable_10000", 10000, "other", "replaceable"},    // 10000-19999 replaceable range
		{"replaceable_19999", 19999, "other", "replaceable"},    // upper bound
		{"ephemeral_lower_bound", 20000, "other", "ephemeral"},  // 20000-29999 ephemeral range
		{"ephemeral_mid", 25000, "other", "ephemeral"},
		{"ephemeral_upper_bound", 29999, "other", "ephemeral"},
		{"addressable_lower_bound", 30000, "other", "addressable"}, // 30000-39999 addressable range
		{"addressable_upper_bound", 39999, "other", "addressable"},

		// Unknown kind AND unknown type
		{"negative_one", -1, "other", "unknown"},
		{"int64_min", math.MinInt64, "other", "unknown"},
		{"above_addressable", 40000, "other", "unknown"},
		{"large", 1_000_000, "other", "unknown"},
		{"int64_max", math.MaxInt64, "other", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotType := kindLabels(tt.kind)
			if gotKind != tt.wantKind || gotType != tt.wantType {
				t.Errorf("kindLabels(%d) = (%q, %q), want (%q, %q)",
					tt.kind, gotKind, gotType, tt.wantKind, tt.wantType)
			}
		})
	}
}

// TestKindLabelsCardinalityBound asserts the total set of observable
// (kind, type) pairs is bounded and matches the policy. If this needs to
// grow, update CLAUDE.md Known concerns #1 at the same time.
func TestKindLabelsCardinalityBound(t *testing.T) {
	seen := make(map[[2]string]struct{})

	// Every known kind contributes exactly one (kind, type) pair.
	for k := range knownKinds {
		kind, typ := kindLabels(k)
		seen[[2]string{kind, typ}] = struct{}{}
	}

	// Sample every type bucket (as an unknown kind) to surface the
	// "other" pairings.
	unknownSamples := []int64{
		2,                  // regular (gap, not allowlisted)
		5000,               // regular (1000..9999)
		15000,              // replaceable (10000..19999)
		25000,              // ephemeral (20000..29999)
		35000,              // addressable (30000..39999)
		-1, 40000, 1 << 40, // unknown
	}
	for _, k := range unknownSamples {
		kind, typ := kindLabels(k)
		seen[[2]string{kind, typ}] = struct{}{}
	}

	// Expected cardinality:
	//   - len(knownKinds) distinct (known_kind, its_type) pairs
	//   - plus 5 pairs: ("other", regular|replaceable|ephemeral|addressable|unknown)
	want := len(knownKinds) + 5
	if got := len(seen); got != want {
		t.Errorf("kindLabels observed cardinality = %d, want %d; seen=%v", got, want, seen)
	}
}

func TestKindType(t *testing.T) {
	tests := []struct {
		kind int64
		want string
	}{
		// regular: 1, 2, 4..44, 1000..9999
		{1, "regular"},
		{2, "regular"},
		{4, "regular"},
		{44, "regular"},
		{1000, "regular"},
		{9999, "regular"},
		// replaceable: 0, 3, 10000..19999
		{0, "replaceable"},
		{3, "replaceable"},
		{10000, "replaceable"},
		{19999, "replaceable"},
		// ephemeral: 20000..29999
		{20000, "ephemeral"},
		{29999, "ephemeral"},
		// addressable: 30000..39999
		{30000, "addressable"},
		{39999, "addressable"},
		// unknown (NIP-01 gaps / out of range)
		{-1, "unknown"},
		{45, "unknown"},
		{999, "unknown"},
		{40000, "unknown"},
		{math.MaxInt64, "unknown"},
	}
	for _, tt := range tests {
		if got := kindType(tt.kind); got != tt.want {
			t.Errorf("kindType(%d) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
