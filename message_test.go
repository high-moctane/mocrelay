package mocrelay

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseClientMsg(t *testing.T) {
	type Expect struct {
		MsgType ClientMsg
		IsErr   bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ng: empty string",
			Input: []byte(""),
			Expect: Expect{
				MsgType: new(ClientUnknownMsg),
				IsErr:   true,
			},
		},
		{
			Name:  "ok: unknown client message",
			Input: []byte(`["POWA","value"]`),
			Expect: Expect{
				MsgType: new(ClientUnknownMsg),
				IsErr:   false,
			},
		},
		{
			Name:  "ok: client close message",
			Input: []byte(`["CLOSE","sub_id"]`),
			Expect: Expect{
				MsgType: new(ClientCloseMsg),
				IsErr:   false,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				MsgType: new(ClientCloseMsg),
				IsErr:   false,
			},
		},
		{
			Name: "ok: client event message",
			Input: []byte(`["EVENT",` +
				`{` +
				`  "kind": 1,` +
				`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
				`  "created_at": 1693157791,` +
				`  "tags": [` +
				`    [` +
				`      "e",` +
				`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
				`      "",` +
				`      "root"` +
				`    ],` +
				`    [` +
				`      "p",` +
				`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
				`    ]` +
				`  ],` +
				`  "content": "powa",` +
				`  "id": "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
				`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
				`}]`),
			Expect: Expect{
				MsgType: new(ClientEventMsg),
				IsErr:   false,
			},
		},
		{
			Name: "ok: client req message",
			Input: []byte(
				`["REQ","cf9ee89f-a07d-4ed6-9cc9-66ff6ef319f4",{"ids":["powa"],"authors":["meu"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}]`,
			),
			Expect: Expect{
				MsgType: new(ClientReqMsg),
				IsErr:   false,
			},
		},
		{
			Name:  "ok: client auth message",
			Input: []byte(`["AUTH","cf9ee89f-a07d-4ed6-9cc9-66ff6ef319f4"]`),
			Expect: Expect{
				MsgType: new(ClientAuthMsg),
				IsErr:   false,
			},
		},
		{
			Name: "ok: client count message",
			Input: []byte(
				`["COUNT","cf9ee89f-a07d-4ed6-9cc9-66ff6ef319f4",{"ids":["powa"],"authors":["meu"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}]`,
			),
			Expect: Expect{
				MsgType: new(ClientCountMsg),
				IsErr:   false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientMsg(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err != nil {
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.IsType(t, tt.Expect.MsgType, msg)
		})
	}
}

func BenchmarkParseClientMsg_All(b *testing.B) {
	eventJSON := []byte(`["EVENT",` +
		`{` +
		`  "kind": 1,` +
		`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
		`  "created_at": 1693157791,` +
		`  "tags": [` +
		`    [` +
		`      "e",` +
		`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
		`      "",` +
		`      "root"` +
		`    ],` +
		`    [` +
		`      "p",` +
		`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
		`    ]` +
		`  ],` +
		`  "content": "powa",` +
		`  "id": "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
		`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
		`}]`)
	reqJSON := []byte(
		`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`,
	)
	closeJSON := []byte(`["CLOSE","sub_id"]`)
	authJSON := []byte(`["AUTH","challenge"]`)
	countJSON := []byte(
		`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i += 5 {
		ParseClientMsg(eventJSON)
		ParseClientMsg(reqJSON)
		ParseClientMsg(closeJSON)
		ParseClientMsg(authJSON)
		ParseClientMsg(countJSON)
	}
}

func BenchmarkParseClientMsg_Event(b *testing.B) {
	eventJSON := []byte(`["EVENT",` +
		`{` +
		`  "kind": 1,` +
		`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
		`  "created_at": 1693157791,` +
		`  "tags": [` +
		`    [` +
		`      "e",` +
		`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
		`      "",` +
		`      "root"` +
		`    ],` +
		`    [` +
		`      "p",` +
		`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
		`    ]` +
		`  ],` +
		`  "content": "powa",` +
		`  "id": "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
		`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
		`}]`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseClientMsg(eventJSON)
	}
}

func BenchmarkParseClientMsg_Req(b *testing.B) {
	reqJSON := []byte(
		`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseClientMsg(reqJSON)
	}
}

func BenchmarkParseClientMsg_Close(b *testing.B) {
	closeJSON := []byte(`["CLOSE","sub_id"]`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseClientMsg(closeJSON)
	}
}

func BenchmarkParseClientMsg_Auth(b *testing.B) {
	authJSON := []byte(`["AUTH","challenge"]`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseClientMsg(authJSON)
	}
}

func BenchmarkParseClientMsg_Count(b *testing.B) {
	countJSON := []byte(
		`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseClientMsg(countJSON)
	}
}

func TestClientUnknownMsg_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		Msg   ClientUnknownMsg
		IsErr bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: json array",
			Input: []byte(`["POWA","meu",{"moyasu":29}]`),
			Expect: Expect{
				Msg: ClientUnknownMsg{
					Label: "POWA",
					Msg: []interface{}{
						"meu",
						map[string]interface{}{
							"moyasu": float64(29),
						},
					},
				},
				IsErr: false,
			},
		},
		{
			Name:  "ng: not a json array",
			Input: []byte(`{"moyasu":29}`),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var msg ClientUnknownMsg
			err := msg.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected err: %v", err)
				return
			}
			if err != nil {
				return
			}
			assert.EqualExportedValues(t, tt.Expect.Msg, msg)
		})
	}
}

func TestClientEventMsg_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		Event Event
		IsErr bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name: "ok: client event message",
			Input: []byte(`["EVENT",` +
				`{` +
				`"kind": 1,` +
				`"pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
				`"created_at": 1693156107,` +
				`"tags": [],` +
				`"content": "ぽわ〜",` +
				`"id": "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
				`"sig": "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee"` +
				`}` +
				`]`),
			Expect: Expect{
				Event: Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				IsErr: false,
			},
		},
		{
			Name:  "ng: client event message invalid type",
			Input: []byte(`["EVENT",3000]`),
			Expect: Expect{
				IsErr: true,
			},
		},
		{
			Name:  "ng: client event message invalid length",
			Input: []byte(`["EVENT"]`),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var msg ClientEventMsg
			err := msg.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err != nil {
				return
			}
			assert.EqualExportedValues(t, tt.Expect.Event, *(msg.Event))
		})
	}
}

func TestClientReqMsg_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		ReqFilters     []*ReqFilter
		IsErr          bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name: "ok: client REQ message",
			Input: []byte(
				`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`,
			),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				ReqFilters: []*ReqFilter{
					{
						IDs:     []string{"powa11", "powa12"},
						Authors: []string{"meu11", "meu12"},
						Kinds:   []int64{1, 3},
						Tags: map[string][]string{
							"#e": {"moyasu11", "moyasu12"},
						},
						Since: toPtr(int64(16)),
						Until: toPtr(int64(184838)),
						Limit: toPtr(int64(143)),
					},
					{
						IDs:     []string{"powa21", "powa22"},
						Authors: []string{"meu21", "meu22"},
						Kinds:   []int64{11, 33},
						Tags:    map[string][]string{"#e": {"moyasu21", "moyasu22"}},
						Since:   toPtr(int64(17)),
						Until:   toPtr(int64(184839)),
						Limit:   toPtr(int64(144)),
					},
				},
				IsErr: false,
			},
		},
		{
			Name:  "ok: client REQ message empty",
			Input: []byte(`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{}]`),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				ReqFilters: []*ReqFilter{
					{
						IDs:     nil,
						Authors: nil,
						Kinds:   nil,
						Tags:    nil,
						Since:   nil,
						Until:   nil,
						Limit:   nil,
					},
				},
				IsErr: false,
			},
		},
		{
			Name:  "ng: client REQ message invalid",
			Input: []byte(`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":1}]`),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var msg ClientReqMsg
			err := msg.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err != nil {
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
			assert.Len(t, msg.ReqFilters, len(tt.Expect.ReqFilters))
			for i := 0; i < len(tt.Expect.ReqFilters); i++ {
				assert.EqualExportedValues(t, *tt.Expect.ReqFilters[i], *msg.ReqFilters[i])
			}
		})
	}
}

func TestClientCloseMsg_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		IsErr          bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: client close message",
			Input: []byte(`["CLOSE","sub_id"]`),
			Expect: Expect{
				SubscriptionID: "sub_id",
				IsErr:          false,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				SubscriptionID: "sub_id",
				IsErr:          false,
			},
		},
		{
			Name:  "ng: client close message invalid type",
			Input: []byte(`["CLOSE",3000]`),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var msg ClientCloseMsg
			err := msg.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err != nil {
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
		})
	}
}

func TestClientAuthMsg_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		Challenge string
		IsErr     bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: client auth message",
			Input: []byte(`["AUTH","challenge"]`),
			Expect: Expect{
				Challenge: "challenge",
				IsErr:     false,
			},
		},
		{
			Name:  "ok: client auth message with some spaces",
			Input: []byte(`[` + "\n" + `  "AUTH",` + "\n" + `  "challenge"` + "\n" + `]`),
			Expect: Expect{
				Challenge: "challenge",
				IsErr:     false,
			},
		},
		{
			Name:  "ng: client auth message invalid type",
			Input: []byte(`["AUTH",3000]`),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var msg ClientAuthMsg
			err := msg.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err != nil {
				return
			}
			assert.Equal(t, tt.Expect.Challenge, msg.Challenge)
		})
	}
}

func TestClientCountMsg_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		ReqFilters     []*ReqFilter
		IsErr          bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name: "ok: client count message",
			Input: []byte(
				`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`,
			),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				ReqFilters: []*ReqFilter{
					{
						IDs:     []string{"powa11", "powa12"},
						Authors: []string{"meu11", "meu12"},
						Kinds:   []int64{1, 3},
						Tags:    map[string][]string{"#e": {"moyasu11", "moyasu12"}},
						Since:   toPtr(int64(16)),
						Until:   toPtr(int64(184838)),
						Limit:   toPtr(int64(143)),
					},
					{
						IDs:     []string{"powa21", "powa22"},
						Authors: []string{"meu21", "meu22"},
						Kinds:   []int64{11, 33},
						Tags:    map[string][]string{"#e": {"moyasu21", "moyasu22"}},
						Since:   toPtr(int64(17)),
						Until:   toPtr(int64(184839)),
						Limit:   toPtr(int64(144)),
					},
				},
				IsErr: false,
			},
		},
		{
			Name:  "ok: client COUNT message empty",
			Input: []byte(`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{}]`),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				ReqFilters: []*ReqFilter{
					{
						IDs:     nil,
						Authors: nil,
						Kinds:   nil,
						Tags:    nil,
						Since:   nil,
						Until:   nil,
						Limit:   nil,
					},
				},
				IsErr: false,
			},
		},
		{
			Name:  "ng: client COUNT message invalid",
			Input: []byte(`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":1}]`),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var msg ClientCountMsg
			err := msg.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err == nil {
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
			assert.Len(t, msg.ReqFilters, len(tt.Expect.ReqFilters))
			for i := 0; i < len(tt.Expect.ReqFilters); i++ {
				assert.EqualExportedValues(t, *tt.Expect.ReqFilters[i], *msg.ReqFilters[i])
			}
		})
	}
}

func TestReqFilter_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		ReqFilter ReqFilter
		IsErr     bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: empty",
			Input: []byte("{}"),
			Expect: Expect{
				ReqFilter: ReqFilter{},
				IsErr:     false,
			},
		},
		{
			Name: "ok: full",
			Input: []byte(
				`{"ids":["powa"],"authors":["meu"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}`,
			),
			Expect: Expect{
				ReqFilter: ReqFilter{
					IDs:     []string{"powa"},
					Authors: []string{"meu"},
					Kinds:   []int64{1, 3},
					Tags:    map[string][]string{"#e": {"moyasu"}},
					Since:   toPtr(int64(16)),
					Until:   toPtr(int64(184838)),
					Limit:   toPtr(int64(143)),
				},
				IsErr: false,
			},
		},
		{
			Name: "ok: partial",
			Input: []byte(
				`{"ids":["powa"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}`,
			),
			Expect: Expect{
				ReqFilter: ReqFilter{
					IDs:     []string{"powa"},
					Kinds:   []int64{1, 3},
					Authors: nil,
					Tags:    map[string][]string{"#e": {"moyasu"}},
					Since:   toPtr(int64(16)),
					Until:   toPtr(int64(184838)),
					Limit:   toPtr(int64(143)),
				},
				IsErr: false,
			},
		},
		{
			Name: "ng: contains some extra fields",
			Input: []byte(
				`{"ids":["powa"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143,"powa":"meu"}`,
			),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var fil ReqFilter
			err := fil.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err != nil {
				return
			}
			assert.EqualExportedValues(t, tt.Expect.ReqFilter, fil)
		})
	}
}

func TestServerEOSEMsg_MarshalJSON(t *testing.T) {
	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerEOSEMsg
		Expect Expect
	}{
		{
			Name: "ok: server eose message",
			Input: &ServerEOSEMsg{
				SubscriptionID: "sub_id",
			},
			Expect: Expect{
				Json: []byte(`["EOSE","sub_id"]`),
				Err:  nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerEOSEMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}

func TestServerEventMsg_MarshalJSON(t *testing.T) {
	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerEventMsg
		Expect Expect
	}{
		{
			Name: "ok: server event message",
			Input: &ServerEventMsg{
				SubscriptionID: "sub_id",
				Event: &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{{
						"e",
						"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						"",
						"root",
					}, {
						"p",
						"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
				},
			},
			Expect: Expect{
				Json: []byte(`["EVENT","sub_id",` +
					`{` +
					`"id":"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
					`"pubkey":"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
					`"created_at":1693157791,` +
					`"kind":1,` +
					`"tags":[` +
					`[` +
					`"e",` +
					`"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
					`"",` +
					`"root"` +
					`],` +
					`[` +
					`"p",` +
					`"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
					`]` +
					`],` +
					`"content":"powa",` +
					`"sig":"795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
					`}]`),
				Err: nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerEventMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}

func TestServerNoticeMsg_MarshalJSON(t *testing.T) {
	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerNoticeMsg
		Expect Expect
	}{
		{
			Name: "ok: server notice message",
			Input: &ServerNoticeMsg{
				Message: "msg",
			},
			Expect: Expect{
				Json: []byte(`["NOTICE","msg"]`),
				Err:  nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerNoticeMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}

func TestServerOKMsg_MarshalJSON(t *testing.T) {
	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerOKMsg
		Expect Expect
	}{
		{
			Name: "ok: server ok message",
			Input: &ServerOKMsg{
				EventID:   "event_id",
				Accepted:  true,
				MsgPrefix: ServerOKMsgPrefixNoPrefix,
				Msg:       "msg",
			},
			Expect: Expect{
				Json: []byte(`["OK","event_id",true,"msg"]`),
				Err:  nil,
			},
		},
		{
			Name: "ok: server ok message with prefix",
			Input: &ServerOKMsg{
				EventID:   "event_id",
				Accepted:  false,
				MsgPrefix: ServerOkMsgPrefixError,
				Msg:       "msg",
			},
			Expect: Expect{
				Json: []byte(`["OK","event_id",false,"error: msg"]`),
				Err:  nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerOKMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}

func TestServerAuthMsg_MarshalJSON(t *testing.T) {
	// TODO(high-moctane) use auth event

	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerAuthMsg
		Expect Expect
	}{
		{
			Name: "ok: server auth message",
			Input: &ServerAuthMsg{
				Event: &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{{
						"e",
						"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						"",
						"root",
					}, {
						"p",
						"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
				},
			},
			Expect: Expect{
				Json: []byte(`["AUTH",` +
					`{` +
					`"id":"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
					`"pubkey":"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
					`"created_at":1693157791,` +
					`"kind":1,` +
					`"tags":[` +
					`[` +
					`"e",` +
					`"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
					`"",` +
					`"root"` +
					`],` +
					`[` +
					`"p",` +
					`"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
					`]` +
					`],` +
					`"content":"powa",` +
					`"sig":"795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
					`}]`),
				Err: nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerAuthMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}

func TestServerCountMsg_MarshalJSON(t *testing.T) {
	type Expect struct {
		Json []byte
		Err  error
	}

	tests := []struct {
		Name   string
		Input  *ServerCountMsg
		Expect Expect
	}{
		{
			Name: "ok: server count message",
			Input: &ServerCountMsg{
				SubscriptionID: "sub_id",
				Count:          192,
				Approximate:    nil,
			},
			Expect: Expect{
				Json: []byte(`["COUNT","sub_id",{"count":192}]`),
				Err:  nil,
			},
		},
		{
			Name: "ok: server count message",
			Input: &ServerCountMsg{
				SubscriptionID: "sub_id",
				Count:          192,
				Approximate:    toPtr(false),
			},
			Expect: Expect{
				Json: []byte(`["COUNT","sub_id",{"count":192,"approximate":false}]`),
				Err:  nil,
			},
		},
		{
			Name:  "ng: nil",
			Input: nil,
			Expect: Expect{
				Err: ErrMarshalServerCountMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got, err := tt.Input.MarshalJSON()
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.Equal(t, tt.Expect.Json, got)
		})
	}
}

func BenchmarkServerMsg_Marshal_All(b *testing.B) {
	var eose ServerMsg = &ServerEOSEMsg{
		SubscriptionID: "sub_id",
	}
	var event ServerMsg = &ServerEventMsg{
		SubscriptionID: "sub_id",
		Event: &Event{
			ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
			Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			CreatedAt: 1693157791,
			Kind:      1,
			Tags: []Tag{{
				"e",
				"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
				"",
				"root",
			}, {
				"p",
				"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			},
			},
			Content: "powa",
			Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
		},
	}
	var notice ServerMsg = &ServerNoticeMsg{
		Message: "msg",
	}
	var ok ServerMsg = &ServerOKMsg{
		EventID:   "event_id",
		Accepted:  false,
		MsgPrefix: ServerOkMsgPrefixError,
		Msg:       "msg",
	}

	// TODO(high-moctane) use auth event
	var auth ServerMsg = &ServerAuthMsg{
		Event: &Event{
			ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
			Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			CreatedAt: 1693157791,
			Kind:      1,
			Tags: []Tag{{
				"e",
				"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
				"",
				"root",
			}, {
				"p",
				"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			},
			},
			Content: "powa",
			Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
		},
	}
	var count ServerMsg = &ServerCountMsg{
		SubscriptionID: "sub_id",
		Count:          192,
		Approximate:    toPtr(false),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(eose)
		json.Marshal(event)
		json.Marshal(notice)
		json.Marshal(ok)
		json.Marshal(auth)
		json.Marshal(count)
	}
}

func BenchmarkServerMsg_Marshal_EOSE(b *testing.B) {
	var eose ServerMsg = &ServerEOSEMsg{
		SubscriptionID: "sub_id",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(eose)
	}
}

func BenchmarkServerMsg_Marshal_Event(b *testing.B) {
	var event ServerMsg = &ServerEventMsg{
		SubscriptionID: "sub_id",
		Event: &Event{
			ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
			Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			CreatedAt: 1693157791,
			Kind:      1,
			Tags: []Tag{{
				"e",
				"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
				"",
				"root",
			}, {
				"p",
				"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			},
			},
			Content: "powa",
			Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(event)
	}
}

func BenchmarkServerMsg_Marshal_Notice(b *testing.B) {
	var notice ServerMsg = &ServerNoticeMsg{
		Message: "msg",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(notice)
	}
}

func BenchmarkServerMsg_Marshal_OK(b *testing.B) {
	var ok ServerMsg = &ServerOKMsg{
		EventID:   "event_id",
		Accepted:  false,
		MsgPrefix: ServerOkMsgPrefixError,
		Msg:       "msg",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(ok)
	}
}

func BenchmarkServerMsg_Marshal_Auth(b *testing.B) {
	// TODO(high-moctane) use auth event
	var auth ServerMsg = &ServerAuthMsg{
		Event: &Event{
			ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
			Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			CreatedAt: 1693157791,
			Kind:      1,
			Tags: []Tag{{
				"e",
				"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
				"",
				"root",
			}, {
				"p",
				"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
			},
			},
			Content: "powa",
			Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(auth)
	}
}
func BenchmarkServerMsg_Marshal_Count(b *testing.B) {
	var count ServerMsg = &ServerCountMsg{
		SubscriptionID: "sub_id",
		Count:          192,
		Approximate:    toPtr(false),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(count)
	}
}

func TestEvent_UnmarshalJSON(t *testing.T) {
	type Expect struct {
		Event *Event
		IsErr bool
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name: "ok",
			Input: []byte(`{` +
				`"kind": 1,` +
				`"pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
				`"created_at": 1693156107,` +
				`"tags": [],` +
				`"content": "ぽわ〜",` +
				`"id": "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
				`"sig": "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee"` +
				`}`),
			Expect: Expect{
				Event: &Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				IsErr: false,
			},
		},
		{
			Name: "ok: with tags",
			Input: []byte(`{` +
				`  "kind": 1,` +
				`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
				`  "created_at": 1693157791,` +
				`  "tags": [` +
				`    [` +
				`      "e",` +
				`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
				`      "",` +
				`      "root"` +
				`    ],` +
				`    [` +
				`      "p",` +
				`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
				`    ]` +
				`  ],` +
				`  "content": "powa",` +
				`  "id": "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
				`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
				`}`),
			Expect: Expect{
				Event: &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{{
						"e",
						"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						"",
						"root",
					}, {
						"p",
						"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
				},
				IsErr: false,
			},
		},
		{
			Name: "ng: without id",
			Input: []byte(`{` +
				`  "kind": 1,` +
				`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
				`  "created_at": 1693157791,` +
				`  "tags": [` +
				`    [` +
				`      "e",` +
				`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
				`      "",` +
				`      "root"` +
				`    ],` +
				`    [` +
				`      "p",` +
				`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
				`    ]` +
				`  ],` +
				`  "content": "powa",` +
				`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
				`}`),
			Expect: Expect{
				Event: nil,
				IsErr: true,
			},
		},
		{
			Name: "ng: contains some extra fields",
			Input: []byte(`{` +
				`  "powa":"meu",` +
				`  "kind": 1,` +
				`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
				`  "created_at": 1693157791,` +
				`  "tags": [` +
				`    [` +
				`      "e",` +
				`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
				`      "",` +
				`      "root"` +
				`    ],` +
				`    [` +
				`      "p",` +
				`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
				`    ]` +
				`  ],` +
				`  "content": "powa",` +
				`  "id": "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
				`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
				`}`),
			Expect: Expect{
				IsErr: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var event Event
			err := event.UnmarshalJSON(tt.Input)
			if (err != nil) != tt.Expect.IsErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if err != nil {
				return
			}
			assert.EqualExportedValues(t, *tt.Expect.Event, event)
		})
	}
}

func BenchmarkParseEvent(b *testing.B) {
	input := []byte(`{` +
		`  "kind": 1,` +
		`  "pubkey": "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",` +
		`  "created_at": 1693157791,` +
		`  "tags": [` +
		`    [` +
		`      "e",` +
		`      "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",` +
		`      "",` +
		`      "root"` +
		`    ],` +
		`    [` +
		`      "p",` +
		`      "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"` +
		`    ]` +
		`  ],` +
		`  "content": "powa",` +
		`  "id": "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",` +
		`  "sig": "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"` +
		`}`)

	var event Event
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event.UnmarshalJSON(input)
	}
}

func TestEvent_Valid(t *testing.T) {
	tests := []struct {
		name string
		in   *Event
		want bool
	}{
		{
			name: "ok",
			in: &Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			want: true,
		},
		{
			name: "ng: big",
			in: &Event{
				ID:        "49D58222BD85DDABFC19B8052D35BCCE2BAD8F1F3030C0BC7DC9F10DBA82A8A2",
				Pubkey:    "DBF0BECF24BF8DD7D779D7FB547E6112964FF042B77A42CC2D8488636EED9F5E",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"D2EA747B6E3A35D2A8B759857B73FCABA5E9F3CFB6F38D317E034BDDC0BF0D1C",
					"",
					"root",
				}, {
					"p",
					"DBF0BECF24BF8DD7D779D7FB547E6112964FF042B77A42CC2D8488636EED9F5E",
				},
				},
				Content: "powa",
				Sig:     "795E51656E8B863805C41B3A6E1195ED63BF8C5DF1FC3A4078CD45AAF0D8838F2DC57B802819443364E8E38C0F35C97E409181680BFFF83E58949500F5A8F0C8",
			},
			want: false,
		},
		{
			name: "short",
			in: &Event{
				ID:        "49d58222bd85ddab",
				Pubkey:    "dbf0becf24bf8dd7",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a351c",
					"",
					"root",
				}, {
					"p",
					"dbf0bec36eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e55aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			want: false,
		},
		{
			name: "ng: nil",
			in:   nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Valid()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvent_Serialize(t *testing.T) {
	tests := []struct {
		name string
		in   *Event
		want []byte
		err  error
	}{
		{
			name: "ok",
			in: &Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			want: []byte(
				`[0,"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",1693157791,1,[["e","d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c","","root"],["p","dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"]],"powa"]`,
			),
			err: nil,
		},
		{
			name: "ng: nil",
			in:   nil,
			want: nil,
			err:  ErrEventSerialize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Serialize()
			if err != nil || tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvent_VerifyID(t *testing.T) {
	tests := []struct {
		name  string
		in    *Event
		want  bool
		isErr bool
	}{
		{
			name: "ok",
			in: &Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			want:  true,
			isErr: false,
		},
		{
			name: "ng",
			in: &Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "695e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			want:  false,
			isErr: false,
		},
		{
			name:  "ng: nil",
			in:    nil,
			want:  false,
			isErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Verify()
			if (err != nil) != tt.isErr {
				t.Errorf("unexpected error: %v", err)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func BenchmarkEvent_Verify(b *testing.B) {
	event := &Event{
		ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
		Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
		CreatedAt: 1693157791,
		Kind:      1,
		Tags: []Tag{{
			"e",
			"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
			"",
			"root",
		}, {
			"p",
			"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
		},
		},
		Content: "powa",
		Sig:     "695e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		event.Verify()
	}
}
