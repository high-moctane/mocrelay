package mocrelay

import (
	"math"
	"testing"
)

func TestKindLabel(t *testing.T) {
	tests := []struct {
		name string
		kind int64
		want string
	}{
		// Known kinds -> decimal form
		{"metadata", 0, "0"},
		{"note", 1, "1"},
		{"follows", 3, "3"},
		{"legacy_dm", 4, "4"},
		{"deletion", 5, "5"},
		{"repost", 6, "6"},
		{"reaction", 7, "7"},
		{"seal", 13, "13"},
		{"chat_message", 14, "14"},
		{"channel_create", 40, "40"},
		{"channel_metadata", 41, "41"},
		{"channel_message", 42, "42"},
		{"hide_message", 43, "43"},
		{"mute_user", 44, "44"},
		{"gift_wrap", 1059, "1059"},
		{"comment", 1111, "1111"},
		{"reporting", 1984, "1984"},
		{"zap_request", 9734, "9734"},
		{"zap_receipt", 9735, "9735"},
		{"mute_list_not_in_allowlist_yet", 10000, "replaceable_other"},
		{"relay_list_metadata", 10002, "10002"},
		{"dm_relay_list", 10050, "10050"},
		{"long_form_content", 30023, "30023"},

		// regular range (1..9999) non-known
		{"regular_2", 2, "regular_other"},
		{"regular_45", 45, "regular_other"},
		{"regular_1000", 1000, "regular_other"},
		{"regular_9999", 9999, "regular_other"},

		// replaceable range (10000..19999) non-known
		{"replaceable_lower_bound", 10000, "replaceable_other"},
		{"replaceable_mid", 10001, "replaceable_other"},
		{"replaceable_upper_bound", 19999, "replaceable_other"},

		// ephemeral range (20000..29999) -- no known kinds in this range
		{"ephemeral_lower_bound", 20000, "ephemeral"},
		{"ephemeral_mid", 25000, "ephemeral"},
		{"ephemeral_upper_bound", 29999, "ephemeral"},

		// addressable range (30000..39999) non-known
		{"addressable_lower_bound", 30000, "addressable_other"},
		{"addressable_mid", 35000, "addressable_other"},
		{"addressable_upper_bound", 39999, "addressable_other"},

		// out of any defined range -> "other"
		{"negative", -1, "other"},
		{"int64_min", math.MinInt64, "other"},
		{"zero_is_metadata_not_other", 0, "0"}, // sanity: 0 is known
		{"above_addressable", 40000, "other"},
		{"large", 1_000_000, "other"},
		{"int64_max", math.MaxInt64, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kindLabel(tt.kind); got != tt.want {
				t.Errorf("kindLabel(%d) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

// TestKindLabelCardinalityBound asserts the total set of possible return
// values is bounded. If this test ever needs to grow, update the policy
// in CLAUDE.md at the same time.
func TestKindLabelCardinalityBound(t *testing.T) {
	seen := make(map[string]struct{})

	// Every known kind contributes one label value.
	for k := range knownKinds {
		seen[kindLabel(k)] = struct{}{}
	}

	// Sample every bucket to surface their labels.
	samples := []int64{
		-1, 0, 1, 2, 45, 1000, 9999,
		10000, 10001, 19999,
		20000, 25000, 29999,
		30000, 35000, 39999,
		40000, 1_000_000,
	}
	for _, k := range samples {
		seen[kindLabel(k)] = struct{}{}
	}

	// Expected: len(knownKinds) individually + 5 bucket labels
	// ("regular_other", "replaceable_other", "ephemeral",
	// "addressable_other", "other").
	want := len(knownKinds) + 5
	if got := len(seen); got != want {
		t.Errorf("kindLabel cardinality = %d, want %d; seen=%v", got, want, seen)
	}
}
