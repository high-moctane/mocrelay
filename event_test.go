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
		{
			name: "null created_at",
			input: `{
				"id": "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				"pubkey": "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
				"created_at": null,
				"kind": 1,
				"tags": [],
				"content": "",
				"sig": "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"
			}`,
			wantErr: true,
		},
		{
			name: "float created_at",
			input: `{
				"id": "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				"pubkey": "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
				"created_at": 1723212754.5,
				"kind": 1,
				"tags": [],
				"content": "",
				"sig": "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"
			}`,
			wantErr: true,
		},
		{
			name: "null kind",
			input: `{
				"id": "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				"pubkey": "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
				"created_at": 1723212754,
				"kind": null,
				"tags": [],
				"content": "",
				"sig": "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"
			}`,
			wantErr: true,
		},
		{
			name: "null id",
			input: `{
				"id": null,
				"pubkey": "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
				"created_at": 1723212754,
				"kind": 1,
				"tags": [],
				"content": "",
				"sig": "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"
			}`,
			wantErr: true,
		},
		{
			name: "null tags",
			input: `{
				"id": "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				"pubkey": "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
				"created_at": 1723212754,
				"kind": 1,
				"tags": null,
				"content": "",
				"sig": "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"
			}`,
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
		{"uppercase ID", &Event{ID: "DC097CD6BD76F2D8816F8A2D294E8442173228E5B24FB946AA05DD89339C9168", Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: validEvent.Sig}, false},
		{"uppercase pubkey", &Event{ID: validEvent.ID, Pubkey: "79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", Tags: []Tag{}, Sig: validEvent.Sig}, false},
		{"uppercase sig", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: "5D2F49649A4F448D13757EE563FD1B8FA04E4DC1931DD34763FB7DF40A082CBDC4E136C733177D3B96A0321F8783FD6B218FEA046E039A23D99B1AB9E2D8B45F"}, false},
		{"nil tags", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: nil, Sig: validEvent.Sig}, false},
		{"empty tag", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{{}}, Sig: validEvent.Sig}, false},
		{"negative kind", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: validEvent.Sig, Kind: -1}, false},
		{"kind too large", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: validEvent.Sig, Kind: 65536}, false},
		{"kind max valid", &Event{ID: validEvent.ID, Pubkey: validEvent.Pubkey, Tags: []Tag{}, Sig: validEvent.Sig, Kind: 65535}, true},
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

func TestTag_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    Tag
	}{
		{
			name:    "valid tag",
			input:   `["e", "abc"]`,
			wantErr: false,
			want:    Tag{"e", "abc"},
		},
		{
			name:    "valid tag with empty value",
			input:   `["d", ""]`,
			wantErr: false,
			want:    Tag{"d", ""},
		},
		{
			name:    "null element",
			input:   `["d", null]`,
			wantErr: true,
		},
		{
			name:    "null in middle",
			input:   `["e", null, "relay"]`,
			wantErr: true,
		},
		{
			name:    "number element",
			input:   `["e", 123]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tag Tag
			err := json.Unmarshal([]byte(tt.input), &tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(tag) != len(tt.want) {
					t.Errorf("got %v, want %v", tag, tt.want)
					return
				}
				for i := range tag {
					if tag[i] != tt.want[i] {
						t.Errorf("got %v, want %v", tag, tt.want)
						return
					}
				}
			}
		})
	}
}
