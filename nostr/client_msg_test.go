package nostr

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/high-moctane/mocrelay/utils"
)

func TestParseClientMsg(t *testing.T) {
	type Expect struct {
		MsgType ClientMsgType
		Err     error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ng: invalid utf8",
			Input: []byte{'[', '"', 0xf0, 0x28, 0x8c, 0xbc, '"', ']'},
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ng: empty string",
			Input: []byte(""),
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ng: not a client message",
			Input: []byte(`["INVALID","value"]`),
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ok: client close message",
			Input: []byte(`["CLOSE","sub_id"]`),
			Expect: Expect{
				MsgType: ClientMsgTypeClose,
				Err:     nil,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				MsgType: ClientMsgTypeClose,
				Err:     nil,
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
				MsgType: ClientMsgTypeEvent,
				Err:     nil,
			},
		},
		{
			Name:  "ok: client req message",
			Input: []byte(`["REQ","cf9ee89f-a07d-4ed6-9cc9-66ff6ef319f4",{"ids":["powa"],"authors":["meu"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}]`),
			Expect: Expect{
				MsgType: ClientMsgTypeReq,
				Err:     nil,
			},
		},
		{
			Name:  "ok: client auth message",
			Input: []byte(`["AUTH","cf9ee89f-a07d-4ed6-9cc9-66ff6ef319f4"]`),
			Expect: Expect{
				MsgType: ClientMsgTypeAuth,
				Err:     nil,
			},
		},
		{
			Name:  "ok: client count message",
			Input: []byte(`["COUNT","cf9ee89f-a07d-4ed6-9cc9-66ff6ef319f4",{"ids":["powa"],"authors":["meu"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}]`),
			Expect: Expect{
				MsgType: ClientMsgTypeCount,
				Err:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.MsgType, msg.MsgType())
			assert.Equal(t, tt.Input, msg.Raw())
		})
	}
}

func TestParseClientEventMsg(t *testing.T) {
	type Expect struct {
		Event Event
		Err   error
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
				Err: nil,
			},
		},
		{
			Name:  "ng: client event message invalid type",
			Input: []byte(`["EVENT",3000]`),
			Expect: Expect{
				Err: ErrInvalidClientEventMsg,
			},
		},
		{
			Name:  "ng: client event message invalid length",
			Input: []byte(`["EVENT"]`),
			Expect: Expect{
				Err: ErrInvalidClientEventMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientEventMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.EqualExportedValues(t, tt.Expect.Event, *msg.Event)
			assert.Equal(t, tt.Input, msg.Raw())
		})
	}
}

func TestParseClientReqMsg(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		Filters        Filters
		Err            error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: client REQ message",
			Input: []byte(`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				Filters: Filters{
					{
						IDs:     utils.ToRef([]string{"powa11", "powa12"}),
						Authors: utils.ToRef([]string{"meu11", "meu12"}),
						Kinds:   utils.ToRef([]int64{1, 3}),
						Tags: utils.ToRef(map[string][]string{
							"#e": {"moyasu11", "moyasu12"},
						}),
						Since: utils.ToRef(int64(16)),
						Until: utils.ToRef(int64(184838)),
						Limit: utils.ToRef(int64(143)),
					},
					{
						IDs:     utils.ToRef([]string{"powa21", "powa22"}),
						Authors: utils.ToRef([]string{"meu21", "meu22"}),
						Kinds:   utils.ToRef([]int64{11, 33}),
						Tags: utils.ToRef(map[string][]string{
							"#e": {"moyasu21", "moyasu22"},
						}),
						Since: utils.ToRef(int64(17)),
						Until: utils.ToRef(int64(184839)),
						Limit: utils.ToRef(int64(144)),
					},
				},
				Err: nil,
			},
		},
		{
			Name:  "ok: client REQ message empty",
			Input: []byte(`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{}]`),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				Filters: Filters{
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
				Err: nil,
			},
		},
		{
			Name:  "ng: client REQ message invalid",
			Input: []byte(`["REQ","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":1}]`),
			Expect: Expect{
				Err: ErrInvalidClientReqMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientReqMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
			assert.Len(t, msg.Filters, len(tt.Expect.Filters))
			for i := 0; i < len(tt.Expect.Filters); i++ {
				assert.EqualExportedValues(t, *tt.Expect.Filters[i], *msg.Filters[i])
			}
			assert.Equal(t, tt.Input, msg.Raw())
		})
	}
}

func TestParseClientCloseMsg(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		Err            error
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
				Err:            nil,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				SubscriptionID: "sub_id",
				Err:            nil,
			},
		},
		{
			Name:  "ng: client close message invalid type",
			Input: []byte(`["CLOSE",3000]`),
			Expect: Expect{
				Err: ErrInvalidClientCloseMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientCloseMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
			assert.Equal(t, tt.Input, msg.Raw())
		})
	}
}

func TestParseClientAuthMsg(t *testing.T) {
	type Expect struct {
		Challenge string
		Err       error
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
				Err:       nil,
			},
		},
		{
			Name:  "ok: client auth message with some spaces",
			Input: []byte(`[` + "\n" + `  "AUTH",` + "\n" + `  "challenge"` + "\n" + `]`),
			Expect: Expect{
				Challenge: "challenge",
				Err:       nil,
			},
		},
		{
			Name:  "ng: client auth message invalid type",
			Input: []byte(`["AUTH",3000]`),
			Expect: Expect{
				Err: ErrInvalidClientAuthMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientAuthMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.Challenge, msg.Challenge)
			assert.Equal(t, tt.Input, msg.Raw())
		})
	}
}

func TestParseClientCountMsg(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		Filters        Filters
		Err            error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: client count message",
			Input: []byte(`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":["powa11","powa12"],"authors":["meu11","meu12"],"kinds":[1,3],"#e":["moyasu11","moyasu12"],"since":16,"until":184838,"limit":143},{"ids":["powa21","powa22"],"authors":["meu21","meu22"],"kinds":[11,33],"#e":["moyasu21","moyasu22"],"since":17,"until":184839,"limit":144}]`),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				Filters: Filters{
					{
						IDs:     utils.ToRef([]string{"powa11", "powa12"}),
						Authors: utils.ToRef([]string{"meu11", "meu12"}),
						Kinds:   utils.ToRef([]int64{1, 3}),
						Tags: utils.ToRef(map[string][]string{
							"#e": {"moyasu11", "moyasu12"},
						}),
						Since: utils.ToRef(int64(16)),
						Until: utils.ToRef(int64(184838)),
						Limit: utils.ToRef(int64(143)),
					},
					{
						IDs:     utils.ToRef([]string{"powa21", "powa22"}),
						Authors: utils.ToRef([]string{"meu21", "meu22"}),
						Kinds:   utils.ToRef([]int64{11, 33}),
						Tags: utils.ToRef(map[string][]string{
							"#e": {"moyasu21", "moyasu22"},
						}),
						Since: utils.ToRef(int64(17)),
						Until: utils.ToRef(int64(184839)),
						Limit: utils.ToRef(int64(144)),
					},
				},
				Err: nil,
			},
		},
		{
			Name:  "ok: client COUNT message empty",
			Input: []byte(`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{}]`),
			Expect: Expect{
				SubscriptionID: "8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",
				Filters: Filters{
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
				Err: nil,
			},
		},
		{
			Name:  "ng: client COUNT message invalid",
			Input: []byte(`["COUNT","8d405a05-a8d7-4cc5-8bc1-53eac4f7949d",{"ids":1}]`),
			Expect: Expect{
				Err: ErrInvalidClientCountMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientCountMsg(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
			assert.Len(t, msg.Filters, len(tt.Expect.Filters))
			for i := 0; i < len(tt.Expect.Filters); i++ {
				assert.EqualExportedValues(t, *tt.Expect.Filters[i], *msg.Filters[i])
			}
			assert.Equal(t, tt.Input, msg.Raw())
		})
	}
}

func TestParseFilter(t *testing.T) {
	type Expect struct {
		Filter Filter
		Err    error
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
				Filter: Filter{},
				Err:    nil,
			},
		},
		{
			Name:  "ok: full",
			Input: []byte(`{"ids":["powa"],"authors":["meu"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}`),
			Expect: Expect{
				Filter: Filter{
					IDs:     utils.ToRef([]string{"powa"}),
					Authors: utils.ToRef([]string{"meu"}),
					Kinds:   utils.ToRef([]int64{1, 3}),
					Tags: utils.ToRef(map[string][]string{
						"#e": {"moyasu"},
					}),
					Since: utils.ToRef(int64(16)),
					Until: utils.ToRef(int64(184838)),
					Limit: utils.ToRef(int64(143)),
				},
				Err: nil,
			},
		},
		{
			Name:  "ok: partial",
			Input: []byte(`{"ids":["powa"],"kinds":[1,3],"#e":["moyasu"],"since":16,"until":184838,"limit":143}`),
			Expect: Expect{
				Filter: Filter{
					IDs:   utils.ToRef([]string{"powa"}),
					Kinds: utils.ToRef([]int64{1, 3}),
					Tags: utils.ToRef(map[string][]string{
						"#e": {"moyasu"},
					}),
					Since: utils.ToRef(int64(16)),
					Until: utils.ToRef(int64(184838)),
					Limit: utils.ToRef(int64(143)),
				},
				Err: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			fil, err := ParseFilter(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if fil == nil {
				t.Errorf("expect non-nil filter but got nil")
				return
			}
			assert.EqualExportedValues(t, tt.Expect.Filter, *fil)
			assert.Equal(t, tt.Input, fil.Raw())
		})
	}
}
