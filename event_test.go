//go:build goexperiment.jsonv2

package mocrelay

import (
	"encoding/json/v2"
	"testing"
	"time"
)

func TestEvent_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "valid event with 7 fields",
			input: `{
				"id": "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				"pubkey": "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
				"created_at": 1723212754,
				"kind": 1,
				"tags": [],
				"content": "",
				"sig": "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"
			}`,
			wantErr: false,
		},
		{
			name: "missing field (6 fields)",
			input: `{
				"id": "abc",
				"pubkey": "def",
				"created_at": 123,
				"kind": 1,
				"tags": [],
				"content": "hello"
			}`,
			wantErr: true,
		},
		{
			name: "extra field (8 fields)",
			input: `{
				"id": "abc",
				"pubkey": "def",
				"created_at": 123,
				"kind": 1,
				"tags": [],
				"content": "hello",
				"sig": "xyz",
				"extra": "bad"
			}`,
			wantErr: true,
		},
		{
			name:    "not an object",
			input:   `[1,2,3]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ev Event
			err := json.Unmarshal([]byte(tt.input), &ev)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvent_CreatedAt_UnixTimestamp(t *testing.T) {
	input := `{
		"id": "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
		"pubkey": "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
		"created_at": 1704067200,
		"kind": 1,
		"tags": [],
		"content": "",
		"sig": "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"
	}`

	var ev Event
	if err := json.Unmarshal([]byte(input), &ev); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Check that CreatedAt is parsed correctly
	expected := time.Unix(1704067200, 0).UTC()
	if !ev.CreatedAt.Equal(expected) {
		t.Errorf("CreatedAt = %v, want %v", ev.CreatedAt, expected)
	}

	// Check that it marshals back correctly
	out, err := json.Marshal(&ev)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var ev2 Event
	if err := json.Unmarshal(out, &ev2); err != nil {
		t.Fatalf("Unmarshal round-trip failed: %v", err)
	}

	if ev2.CreatedAt.Unix() != 1704067200 {
		t.Errorf("Round-trip CreatedAt.Unix() = %d, want 1704067200", ev2.CreatedAt.Unix())
	}
}

func TestEvent_EventType(t *testing.T) {
	tests := []struct {
		kind int64
		want EventType
	}{
		{1, EventTypeRegular},
		{2, EventTypeRegular},
		{1000, EventTypeRegular},
		{9999, EventTypeRegular},
		{0, EventTypeReplaceable},
		{3, EventTypeReplaceable},
		{10000, EventTypeReplaceable},
		{19999, EventTypeReplaceable},
		{20000, EventTypeEphemeral},
		{29999, EventTypeEphemeral},
		{30000, EventTypeAddressable},
		{39999, EventTypeAddressable},
		{40000, EventTypeRegular},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			ev := &Event{Kind: tt.kind}
			if got := ev.EventType(); got != tt.want {
				t.Errorf("EventType() for kind %d = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestEvent_Valid(t *testing.T) {
	validEvent := &Event{
		ID:        "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
		Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
		CreatedAt: time.Unix(1723212754, 0),
		Kind:      1,
		Tags:      []Tag{},
		Content:   "",
		Sig:       "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f",
	}

	if !validEvent.Valid() {
		t.Error("Valid() = false for valid event")
	}

	tests := []struct {
		name  string
		event *Event
		want  bool
	}{
		{"nil event", nil, false},
		{"short ID", &Event{ID: "abc", Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: validEvent.Sig}, false},
		{"short pubkey", &Event{ID: validEvent.ID, Pubkey: "abc", Tags: []Tag{}, Sig: validEvent.Sig}, false},
		{"short sig", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: "abc"}, false},
		{"nil tags", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: nil, Sig: validEvent.Sig}, false},
		{"empty tag", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{{}}, Sig: validEvent.Sig}, false},
		{"negative kind", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: validEvent.Sig, Kind: -1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvent_Serialize(t *testing.T) {
	ev := &Event{
		ID:        "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
		Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
		CreatedAt: time.Unix(1723212754, 0),
		Kind:      1,
		Tags:      []Tag{{"e", "abc"}, {"p", "def"}},
		Content:   "hello",
		Sig:       "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f",
	}

	serialized, err := ev.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}

	// Check format: [0, pubkey, created_at, kind, tags, content]
	expected := `[0,"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",1723212754,1,[["e","abc"],["p","def"]],"hello"]`
	if string(serialized) != expected {
		t.Errorf("Serialize() = %s, want %s", serialized, expected)
	}
}

func TestEvent_Address(t *testing.T) {
	tests := []struct {
		name  string
		event *Event
		want  string
	}{
		{
			name:  "regular event",
			event: &Event{Kind: 1, Pubkey: "abc"},
			want:  "",
		},
		{
			name:  "ephemeral event",
			event: &Event{Kind: 20000, Pubkey: "abc"},
			want:  "",
		},
		{
			name:  "replaceable event (kind 0)",
			event: &Event{Kind: 0, Pubkey: "abc"},
			want:  "0:abc:",
		},
		{
			name:  "replaceable event (kind 10000)",
			event: &Event{Kind: 10000, Pubkey: "abc"},
			want:  "10000:abc:",
		},
		{
			name:  "addressable event with d-tag",
			event: &Event{Kind: 30000, Pubkey: "abc", Tags: []Tag{{"d", "test"}}},
			want:  "30000:abc:test",
		},
		{
			name:  "addressable event without d-tag",
			event: &Event{Kind: 30000, Pubkey: "abc", Tags: []Tag{}},
			want:  "30000:abc:",
		},
		{
			name:  "addressable event with empty d-tag",
			event: &Event{Kind: 30000, Pubkey: "abc", Tags: []Tag{{"d"}}},
			want:  "30000:abc:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.Address(); got != tt.want {
				t.Errorf("Address() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTag(t *testing.T) {
	t.Run("Key", func(t *testing.T) {
		if (Tag{}).Key() != "" {
			t.Error("empty tag Key() should return empty string")
		}
		if (Tag{"e"}).Key() != "e" {
			t.Error("Key() should return first element")
		}
	})

	t.Run("Value", func(t *testing.T) {
		if (Tag{}).Value() != "" {
			t.Error("empty tag Value() should return empty string")
		}
		if (Tag{"e"}).Value() != "" {
			t.Error("single element tag Value() should return empty string")
		}
		if (Tag{"e", "abc"}).Value() != "abc" {
			t.Error("Value() should return second element")
		}
	})
}

func TestIsValidHex(t *testing.T) {
	tests := []struct {
		s      string
		length int
		want   bool
	}{
		{"abcd", 4, true},
		{"ABCD", 4, true},
		{"1234", 4, true},
		{"abcd", 3, false},
		{"abcd", 5, false},
		{"ghij", 4, false},
		{"ab cd", 5, false},
	}

	for _, tt := range tests {
		if got := isValidHex(tt.s, tt.length); got != tt.want {
			t.Errorf("isValidHex(%q, %d) = %v, want %v", tt.s, tt.length, got, tt.want)
		}
	}
}
