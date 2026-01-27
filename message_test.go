//go:build goexperiment.jsonv2

package mocrelay

import (
	"encoding/json/v2"
	"testing"
)

func TestParseClientMsg_Event(t *testing.T) {
	input := `["EVENT",{"id":"dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1723212754,"kind":1,"tags":[],"content":"hello","sig":"5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f"}]`

	msg, err := ParseClientMsg([]byte(input))
	if err != nil {
		t.Fatalf("ParseClientMsg() error = %v", err)
	}

	if msg.Type != MsgTypeEvent {
		t.Errorf("Type = %q, want %q", msg.Type, MsgTypeEvent)
	}
	if msg.Event == nil {
		t.Fatal("Event is nil")
	}
	if msg.Event.Content != "hello" {
		t.Errorf("Event.Content = %q, want %q", msg.Event.Content, "hello")
	}
}

func TestParseClientMsg_Req(t *testing.T) {
	input := `["REQ","sub1",{"kinds":[1],"limit":10},{"authors":["abc"]}]`

	msg, err := ParseClientMsg([]byte(input))
	if err != nil {
		t.Fatalf("ParseClientMsg() error = %v", err)
	}

	if msg.Type != MsgTypeReq {
		t.Errorf("Type = %q, want %q", msg.Type, MsgTypeReq)
	}
	if msg.SubscriptionID != "sub1" {
		t.Errorf("SubscriptionID = %q, want %q", msg.SubscriptionID, "sub1")
	}
	if len(msg.Filters) != 2 {
		t.Fatalf("len(Filters) = %d, want 2", len(msg.Filters))
	}
	if len(msg.Filters[0].Kinds) != 1 || msg.Filters[0].Kinds[0] != 1 {
		t.Errorf("Filters[0].Kinds = %v, want [1]", msg.Filters[0].Kinds)
	}
}

func TestParseClientMsg_ReqWithTags(t *testing.T) {
	input := `["REQ","sub1",{"#e":["abc","def"],"#p":["xyz"]}]`

	msg, err := ParseClientMsg([]byte(input))
	if err != nil {
		t.Fatalf("ParseClientMsg() error = %v", err)
	}

	if msg.Type != MsgTypeReq {
		t.Errorf("Type = %q, want %q", msg.Type, MsgTypeReq)
	}
	if len(msg.Filters) != 1 {
		t.Fatalf("len(Filters) = %d, want 1", len(msg.Filters))
	}

	f := msg.Filters[0]
	if len(f.Tags["e"]) != 2 {
		t.Errorf("Tags[e] = %v, want 2 elements", f.Tags["e"])
	}
	if len(f.Tags["p"]) != 1 {
		t.Errorf("Tags[p] = %v, want 1 element", f.Tags["p"])
	}
}

func TestParseClientMsg_Close(t *testing.T) {
	input := `["CLOSE","sub1"]`

	msg, err := ParseClientMsg([]byte(input))
	if err != nil {
		t.Fatalf("ParseClientMsg() error = %v", err)
	}

	if msg.Type != MsgTypeClose {
		t.Errorf("Type = %q, want %q", msg.Type, MsgTypeClose)
	}
	if msg.SubscriptionID != "sub1" {
		t.Errorf("SubscriptionID = %q, want %q", msg.SubscriptionID, "sub1")
	}
}

func TestParseClientMsg_Count(t *testing.T) {
	input := `["COUNT","sub1",{"kinds":[1]}]`

	msg, err := ParseClientMsg([]byte(input))
	if err != nil {
		t.Fatalf("ParseClientMsg() error = %v", err)
	}

	if msg.Type != MsgTypeCount {
		t.Errorf("Type = %q, want %q", msg.Type, MsgTypeCount)
	}
	if msg.SubscriptionID != "sub1" {
		t.Errorf("SubscriptionID = %q, want %q", msg.SubscriptionID, "sub1")
	}
	if len(msg.Filters) != 1 {
		t.Fatalf("len(Filters) = %d, want 1", len(msg.Filters))
	}
}

func TestParseClientMsg_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"not array", `{"type":"EVENT"}`},
		{"unknown type", `["UNKNOWN","data"]`},
		{"REQ no filters", `["REQ","sub1"]`},
		{"COUNT no filters", `["COUNT","sub1"]`},
		{"CLOSE wrong length", `["CLOSE"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseClientMsg([]byte(tt.input))
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestServerMsg_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		msg  *ServerMsg
		want string
	}{
		{
			name: "EOSE",
			msg:  NewServerEOSEMsg("sub1"),
			want: `["EOSE","sub1"]`,
		},
		{
			name: "OK accepted",
			msg:  NewServerOKMsg("abc123", true, ""),
			want: `["OK","abc123",true,""]`,
		},
		{
			name: "OK rejected",
			msg:  NewServerOKMsg("abc123", false, "blocked: spam"),
			want: `["OK","abc123",false,"blocked: spam"]`,
		},
		{
			name: "NOTICE",
			msg:  NewServerNoticeMsg("hello"),
			want: `["NOTICE","hello"]`,
		},
		{
			name: "CLOSED",
			msg:  NewServerClosedMsg("sub1", "error: too many subs"),
			want: `["CLOSED","sub1","error: too many subs"]`,
		},
		{
			name: "AUTH",
			msg:  NewServerAuthMsg("challenge123"),
			want: `["AUTH","challenge123"]`,
		},
		{
			name: "COUNT without approximate",
			msg:  NewServerCountMsg("sub1", 42, nil),
			want: `["COUNT","sub1",{"count":42}]`,
		},
		{
			name: "COUNT with approximate",
			msg:  NewServerCountMsg("sub1", 100, ptr(true)),
			want: `["COUNT","sub1",{"approximate":true,"count":100}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			// Compare as JSON to avoid field ordering issues
			var gotJSON, wantJSON any
			if err := json.Unmarshal(got, &gotJSON); err != nil {
				t.Fatalf("failed to unmarshal got: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantJSON); err != nil {
				t.Fatalf("failed to unmarshal want: %v", err)
			}
			if !jsonEqual(gotJSON, wantJSON) {
				t.Errorf("MarshalJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}

// jsonEqual compares two JSON values for equality, ignoring map key order.
func jsonEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !jsonEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func TestServerMsg_Event(t *testing.T) {
	ev := &Event{
		ID:      "abc",
		Pubkey:  "def",
		Kind:    1,
		Tags:    []Tag{},
		Content: "hello",
		Sig:     "xyz",
	}
	msg := NewServerEventMsg("sub1", ev)

	got, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Check it starts with ["EVENT","sub1",
	if len(got) < 20 {
		t.Fatalf("output too short: %s", got)
	}
	prefix := `["EVENT","sub1",{`
	if string(got[:len(prefix)]) != prefix {
		t.Errorf("output doesn't start with %s: %s", prefix, got)
	}
}

func ptr[T any](v T) *T {
	return &v
}
