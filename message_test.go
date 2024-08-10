package mocrelay

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed testdata/clienteventmsgs_valid.jsonl
var clientEventMsgsValidJSONL []byte

//go:embed testdata/clienteventmsgs_invalid.jsonl
var clientEventMsgsInvalidJSONL []byte

//go:embed testdata/clientreqmsgs_valid.jsonl
var clientReqMsgsValidJSONL []byte

//go:embed testdata/clientreqmsgs_invalid.jsonl
var clientReqMsgsInvalidJSONL []byte

//go:embed testdata/clientclosemsgs_valid.jsonl
var clientCloseMsgsValidJSONL []byte

//go:embed testdata/clientclosemsgs_invalid.jsonl
var clientCloseMsgsInvalidJSONL []byte

//go:embed testdata/clientauthmsgs_valid.jsonl
var clientAuthMsgsValidJSONL []byte

//go:embed testdata/clientauthmsgs_invalid.jsonl
var clientAuthMsgsInvalidJSONL []byte

//go:embed testdata/clientcountmsgs_valid.jsonl
var clientCountMsgsValidJSONL []byte

//go:embed testdata/clientcountmsgs_invalid.jsonl
var clientCountMsgsInvalidJSONL []byte

//go:embed testdata/servereosemsgs_valid.jsonl
var serverEOSEMsgsValidJSONL []byte

//go:embed testdata/servereosemsgs_invalid.jsonl
var serverEOSEMsgsInvalidJSONL []byte

//go:embed testdata/servereventmsgs_valid.jsonl
var serverEventMsgsValidJSONL []byte

//go:embed testdata/servereventmsgs_invalid.jsonl
var serverEventMsgsInvalidJSONL []byte

//go:embed testdata/servernoticemsg_valid.jsonl
var serverNoticeMsgsValidJSONL []byte

//go:embed testdata/servernoticemsg_invalid.jsonl
var serverNoticeMsgsInvalidJSONL []byte

//go:embed testdata/serverokmsgs_valid.jsonl
var serverOKMsgsValidJSONL []byte

//go:embed testdata/serverokmsgs_invalid.jsonl
var serverOKMsgsInvalidJSONL []byte

//go:embed testdata/serverauthmsgs_valid.jsonl
var serverAuthMsgsValidJSONL []byte

//go:embed testdata/serverauthmsgs_invalid.jsonl
var serverAuthMsgsInvalidJSONL []byte

//go:embed testdata/servercountmsgs_valid.jsonl
var serverCountMsgsValidJSONL []byte

//go:embed testdata/servercountmsgs_invalid.jsonl
var serverCountMsgsInvalidJSONL []byte

//go:embed testdata/serverclosedmsgs_valid.jsonl
var serverClosedMsgsValidJSONL []byte

//go:embed testdata/serverclosedmsgs_invalid.jsonl
var serverClosedMsgsInvalidJSONL []byte

//go:embed testdata/events_valid.jsonl
var eventsValidJSONL []byte

//go:embed testdata/events_invalid.jsonl
var eventsInvalidJSONL []byte

//go:embed testdata/reqfilter_valid.jsonl
var reqFilterValidJSONL []byte

//go:embed testdata/reqfilter_invalid.jsonl
var reqFilterInvalidJSONL []byte

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
				MsgType: nil,
				IsErr:   true,
			},
		},
		{
			Name:  "ng: unknown client message",
			Input: []byte(`["POWA","value"]`),
			Expect: Expect{
				MsgType: nil,
				IsErr:   true,
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

func TestClientEventMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(clientEventMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ClientEventMsg
		want []byte
	}{
		{
			in: ClientEventMsg{
				Event: &Event{
					ID:        "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723212754,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "",
					Sig:       "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f",
				},
			},
			want: jsons[0],
		},
		{
			in: ClientEventMsg{
				Event: &Event{
					ID:        "07e782ba4b5fe85b91264d03c445c339b8783e0ea2ae3bdfb0122eda513d86ac",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723212830,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "with content",
					Sig:       "a714af322ebf22bec24364319efc635e88230c099a8e08238fff1f4f5608494cddbcc77bef426cc653e4994ac23625553500d05244dd28ed2ac3096cff0387af",
				},
			},
			want: jsons[1],
		},
		{
			in: ClientEventMsg{
				Event: &Event{
					ID:        "80cfa4cff224ad441b9cb50fdce68a47f30a2d7e38fa2b06f2ddac748bbac137",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723212930,
					Kind:      1,
					Tags: []Tag{
						{"key1", "value1"},
						{"key2"},
						{"key3", "value3-1", "value3-2"},
					},
					Content: "with tags",
					Sig:     "7553bd8efb6e338e58cd9b807225dfd5d71043f94173aa234c7727aa28236009ee34aa76422e6d3912a5d104100c49148b043202df7f8b258782b1816434d1ea",
				},
			},
			want: jsons[2],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestClientEventMsg_UnarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientEventMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ClientEventMsg
		}{
			{
				in: jsons[0],
				want: ClientEventMsg{
					Event: &Event{
						ID:        "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
						Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
						CreatedAt: 1723212754,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "",
						Sig:       "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f",
					},
				},
			},
			{
				in: jsons[1],
				want: ClientEventMsg{
					Event: &Event{
						ID:        "07e782ba4b5fe85b91264d03c445c339b8783e0ea2ae3bdfb0122eda513d86ac",
						Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
						CreatedAt: 1723212830,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "with content",
						Sig:       "a714af322ebf22bec24364319efc635e88230c099a8e08238fff1f4f5608494cddbcc77bef426cc653e4994ac23625553500d05244dd28ed2ac3096cff0387af",
					},
				},
			},
			{
				in: jsons[2],
				want: ClientEventMsg{
					Event: &Event{
						ID:        "80cfa4cff224ad441b9cb50fdce68a47f30a2d7e38fa2b06f2ddac748bbac137",
						Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
						CreatedAt: 1723212930,
						Kind:      1,
						Tags: []Tag{
							{"key1", "value1"},
							{"key2"},
							{"key3", "value3-1", "value3-2"},
						},
						Content: "with tags",
						Sig:     "7553bd8efb6e338e58cd9b807225dfd5d71043f94173aa234c7727aa28236009ee34aa76422e6d3912a5d104100c49148b043202df7f8b258782b1816434d1ea",
					},
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientEventMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientEventMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientEventMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestClientReqMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(clientReqMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ClientReqMsg
		want []byte
	}{
		{
			in: ClientReqMsg{
				SubscriptionID: "subid",
				ReqFilters:     []*ReqFilter{{}},
			},
			want: jsons[0],
		},
		{
			in: ClientReqMsg{
				SubscriptionID: "subid",
				ReqFilters: []*ReqFilter{
					{
						IDs:     []string{},
						Authors: []string{},
						Kinds:   []int64{},
						Tags: map[string][]string{
							"#e": {},
							"#p": {},
						},
						Since: toPtr[int64](100),
						Until: toPtr[int64](10000),
						Limit: toPtr[int64](200),
					},
					{
						IDs: []string{
							"0000000000000000000000000000000000000000000000000000000000000000",
							"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
						},
						Authors: []string{
							"0000000000000000000000000000000000000000000000000000000000000000",
							"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
						},
						Kinds: []int64{0, 1, 10000, 20000, 30000},
						Tags: map[string][]string{
							"#e": {
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
							"#p": {
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
						},
						Since: toPtr[int64](100),
						Until: toPtr[int64](10000),
						Limit: toPtr[int64](200),
					},
				},
			},
			want: jsons[1],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestClientReqMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientReqMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ClientReqMsg
		}{
			{
				in: jsons[0],
				want: ClientReqMsg{
					SubscriptionID: "subid",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			{
				in: jsons[1],
				want: ClientReqMsg{
					SubscriptionID: "subid",
					ReqFilters: []*ReqFilter{
						{
							IDs:     []string{},
							Authors: []string{},
							Kinds:   []int64{},
							Tags: map[string][]string{
								"#e": {},
								"#p": {},
							},
							Since: toPtr[int64](100),
							Until: toPtr[int64](10000),
							Limit: toPtr[int64](200),
						},
						{
							IDs: []string{
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
							Authors: []string{
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
							Kinds: []int64{0, 1, 10000, 20000, 30000},
							Tags: map[string][]string{
								"#e": {
									"0000000000000000000000000000000000000000000000000000000000000000",
									"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
								},
								"#p": {
									"0000000000000000000000000000000000000000000000000000000000000000",
									"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
								},
							},
							Since: toPtr[int64](100),
							Until: toPtr[int64](10000),
							Limit: toPtr[int64](200),
						},
					},
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientReqMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientReqMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientReqMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestClientCloseMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(clientCloseMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ClientCloseMsg
		want []byte
	}{
		{
			in:   ClientCloseMsg{SubscriptionID: "subid"},
			want: jsons[0],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestClientCloseMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientCloseMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ClientCloseMsg
		}{
			{
				in:   jsons[0],
				want: ClientCloseMsg{SubscriptionID: "subid"},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientCloseMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientCloseMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientCloseMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestClientAuthMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(clientAuthMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ClientAuthMsg
		want []byte
	}{
		{
			in:   ClientAuthMsg{Challenge: "challenge"},
			want: jsons[0],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestClientAuthMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientAuthMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ClientAuthMsg
		}{
			{
				in:   jsons[0],
				want: ClientAuthMsg{Challenge: "challenge"},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientAuthMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientAuthMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientAuthMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestClientCountMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(clientCountMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ClientCountMsg
		want []byte
	}{
		{
			in: ClientCountMsg{
				SubscriptionID: "subid",
				ReqFilters:     []*ReqFilter{{}},
			},
			want: jsons[0],
		},
		{
			in: ClientCountMsg{
				SubscriptionID: "subid",
				ReqFilters: []*ReqFilter{
					{
						IDs:     []string{},
						Authors: []string{},
						Kinds:   []int64{},
						Tags: map[string][]string{
							"#e": {},
							"#p": {},
						},
						Since: toPtr[int64](100),
						Until: toPtr[int64](10000),
						Limit: toPtr[int64](200),
					},
					{
						IDs: []string{
							"0000000000000000000000000000000000000000000000000000000000000000",
							"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
						},
						Authors: []string{
							"0000000000000000000000000000000000000000000000000000000000000000",
							"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
						},
						Kinds: []int64{0, 1, 10000, 20000, 30000},
						Tags: map[string][]string{
							"#e": {
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
							"#p": {
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
						},
						Since: toPtr[int64](100),
						Until: toPtr[int64](10000),
						Limit: toPtr[int64](200),
					},
				},
			},
			want: jsons[1],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestClientCountMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientCountMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ClientCountMsg
		}{
			{
				in: jsons[0],
				want: ClientCountMsg{
					SubscriptionID: "subid",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			{
				in: jsons[1],
				want: ClientCountMsg{
					SubscriptionID: "subid",
					ReqFilters: []*ReqFilter{
						{
							IDs:     []string{},
							Authors: []string{},
							Kinds:   []int64{},
							Tags: map[string][]string{
								"#e": {},
								"#p": {},
							},
							Since: toPtr[int64](100),
							Until: toPtr[int64](10000),
							Limit: toPtr[int64](200),
						},
						{
							IDs: []string{
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
							Authors: []string{
								"0000000000000000000000000000000000000000000000000000000000000000",
								"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
							},
							Kinds: []int64{0, 1, 10000, 20000, 30000},
							Tags: map[string][]string{
								"#e": {
									"0000000000000000000000000000000000000000000000000000000000000000",
									"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
								},
								"#p": {
									"0000000000000000000000000000000000000000000000000000000000000000",
									"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
								},
							},
							Since: toPtr[int64](100),
							Until: toPtr[int64](10000),
							Limit: toPtr[int64](200),
						},
					},
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientCountMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(clientCountMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ClientCountMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestReqFilter_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(reqFilterValidJSONL), []byte("\n"))

	tests := []struct {
		in   ReqFilter
		want []byte
	}{
		{
			in:   ReqFilter{},
			want: jsons[0],
		},
		{
			in: ReqFilter{
				IDs:     []string{},
				Authors: []string{},
				Kinds:   []int64{},
				Tags: map[string][]string{
					"#e": {},
					"#p": {},
				},
				Since: toPtr[int64](100),
				Until: toPtr[int64](10000),
				Limit: toPtr[int64](200),
			},
			want: jsons[1],
		},
		{
			in: ReqFilter{
				IDs: []string{
					"0000000000000000000000000000000000000000000000000000000000000000",
					"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				},
				Authors: []string{
					"0000000000000000000000000000000000000000000000000000000000000000",
					"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				},
				Kinds: []int64{0, 1, 10000, 20000, 30000},
				Tags: map[string][]string{
					"#e": {
						"0000000000000000000000000000000000000000000000000000000000000000",
						"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
					},
					"#p": {
						"0000000000000000000000000000000000000000000000000000000000000000",
						"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
					},
				},
				Since: toPtr[int64](100),
				Until: toPtr[int64](10000),
				Limit: toPtr[int64](200),
			},
			want: jsons[2],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestReqFilter_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(reqFilterValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ReqFilter
		}{
			{
				in:   jsons[0],
				want: ReqFilter{},
			},
			{
				in: jsons[1],
				want: ReqFilter{
					IDs:     []string{},
					Authors: []string{},
					Kinds:   []int64{},
					Tags: map[string][]string{
						"#e": {},
						"#p": {},
					},
					Since: toPtr[int64](100),
					Until: toPtr[int64](10000),
					Limit: toPtr[int64](200),
				},
			},
			{
				in: jsons[2],
				want: ReqFilter{
					IDs: []string{
						"0000000000000000000000000000000000000000000000000000000000000000",
						"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
					},
					Authors: []string{
						"0000000000000000000000000000000000000000000000000000000000000000",
						"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
					},
					Kinds: []int64{0, 1, 10000, 20000, 30000},
					Tags: map[string][]string{
						"#e": {
							"0000000000000000000000000000000000000000000000000000000000000000",
							"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
						},
						"#p": {
							"0000000000000000000000000000000000000000000000000000000000000000",
							"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
						},
					},
					Since: toPtr[int64](100),
					Until: toPtr[int64](10000),
					Limit: toPtr[int64](200),
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ReqFilter
				err := got.UnmarshalJSON(tt.in)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(reqFilterInvalidJSONL), []byte("\n"))

		for i, in := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ReqFilter
				err := got.UnmarshalJSON(in)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestServerEOSEMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(serverEOSEMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ServerEOSEMsg
		want []byte
	}{
		{
			in:   ServerEOSEMsg{SubscriptionID: "subid"},
			want: jsons[0],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestServerEOSEMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverEOSEMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ServerEOSEMsg
		}{
			{
				in:   jsons[0],
				want: ServerEOSEMsg{SubscriptionID: "subid"},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerEOSEMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverEOSEMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerEOSEMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestServerEventMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(serverEventMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ServerEventMsg
		want []byte
	}{
		{
			in: ServerEventMsg{
				SubscriptionID: "subid",
				Event: &Event{
					ID:        "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723212754,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "",
					Sig:       "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f",
				},
			},
			want: jsons[0],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestServerEventMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverEventMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ServerEventMsg
		}{
			{
				in: jsons[0],
				want: ServerEventMsg{
					SubscriptionID: "subid",
					Event: &Event{
						ID:        "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
						Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
						CreatedAt: 1723212754,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "",
						Sig:       "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f",
					},
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerEventMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverEventMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerEventMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestServerNoticeMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(serverNoticeMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ServerNoticeMsg
		want []byte
	}{
		{
			in:   ServerNoticeMsg{Message: "msg"},
			want: jsons[0],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestServerNoticeMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverNoticeMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ServerNoticeMsg
		}{
			{
				in:   jsons[0],
				want: ServerNoticeMsg{Message: "msg"},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerNoticeMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverNoticeMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerNoticeMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestServerOKMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(serverOKMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ServerOKMsg
		want []byte
	}{
		{
			in: ServerOKMsg{
				EventID:  "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted: true,
				Msg:      "",
			},
			want: jsons[0],
		},
		{
			in: ServerOKMsg{
				EventID:  "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted: false,
				Msg:      "with msg",
			},
			want: jsons[1],
		},
		{
			in: ServerOKMsg{
				EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted:  false,
				Msg:       "with prefix",
				MsgPrefix: ServerOKMsgPrefixPoW,
			},
			want: jsons[2],
		},
		{
			in: ServerOKMsg{
				EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted:  false,
				Msg:       "with prefix",
				MsgPrefix: ServerOKMsgPrefixDuplicate,
			},
			want: jsons[3],
		},
		{
			in: ServerOKMsg{
				EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted:  false,
				Msg:       "with prefix",
				MsgPrefix: ServerOkMsgPrefixBlocked,
			},
			want: jsons[4],
		},
		{
			in: ServerOKMsg{
				EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted:  false,
				Msg:       "with prefix",
				MsgPrefix: ServerOkMsgPrefixRateLimited,
			},
			want: jsons[5],
		},
		{
			in: ServerOKMsg{
				EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted:  false,
				Msg:       "with prefix",
				MsgPrefix: ServerOkMsgPrefixInvalid,
			},
			want: jsons[6],
		},
		{
			in: ServerOKMsg{
				EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
				Accepted:  false,
				Msg:       "with prefix",
				MsgPrefix: ServerOkMsgPrefixError,
			},
			want: jsons[7],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestServerOKMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverOKMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ServerOKMsg
		}{
			{
				in: jsons[0],
				want: ServerOKMsg{
					EventID:  "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted: true,
					Msg:      "",
				},
			},
			{
				in: jsons[1],
				want: ServerOKMsg{
					EventID:  "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted: false,
					Msg:      "with msg",
				},
			},
			{
				in: jsons[2],
				want: ServerOKMsg{
					EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted:  false,
					Msg:       "with prefix",
					MsgPrefix: ServerOKMsgPrefixPoW,
				},
			},
			{
				in: jsons[3],
				want: ServerOKMsg{
					EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted:  false,
					Msg:       "with prefix",
					MsgPrefix: ServerOKMsgPrefixDuplicate,
				},
			},
			{
				in: jsons[4],
				want: ServerOKMsg{
					EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted:  false,
					Msg:       "with prefix",
					MsgPrefix: ServerOkMsgPrefixBlocked,
				},
			},
			{
				in: jsons[5],
				want: ServerOKMsg{
					EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted:  false,
					Msg:       "with prefix",
					MsgPrefix: ServerOkMsgPrefixRateLimited,
				},
			},
			{
				in: jsons[6],
				want: ServerOKMsg{
					EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted:  false,
					Msg:       "with prefix",
					MsgPrefix: ServerOkMsgPrefixInvalid,
				},
			},
			{
				in: jsons[7],
				want: ServerOKMsg{
					EventID:   "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Accepted:  false,
					Msg:       "with prefix",
					MsgPrefix: ServerOkMsgPrefixError,
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerOKMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverOKMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerOKMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestServerAuthMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(serverAuthMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ServerAuthMsg
		want []byte
	}{
		{
			in: ServerAuthMsg{
				Event: &Event{
					ID:        "37b9219808f916a0ab60cf40daf836cfd73ab6051d536e5453cadf8d424df829",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723280486,
					Kind:      22242,
					Tags:      []Tag{{"relay", "example.com"}, {"challenge", "challengemsg"}},
					Content:   "",
					Sig:       "a5b7f83c7d65a5de41a81a688a450fa5c58bd86d7c106ec71721e64fb0a78d62e7413fbcbee17b0c6f3d7eeefbcf6fd482945942617f69b5a91e6d189ff62feb",
				},
			},
			want: jsons[0],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestServerAuthMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverAuthMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ServerAuthMsg
		}{
			{
				in: jsons[0],
				want: ServerAuthMsg{
					Event: &Event{
						ID:        "37b9219808f916a0ab60cf40daf836cfd73ab6051d536e5453cadf8d424df829",
						Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
						CreatedAt: 1723280486,
						Kind:      22242,
						Tags:      []Tag{{"relay", "example.com"}, {"challenge", "challengemsg"}},
						Content:   "",
						Sig:       "a5b7f83c7d65a5de41a81a688a450fa5c58bd86d7c106ec71721e64fb0a78d62e7413fbcbee17b0c6f3d7eeefbcf6fd482945942617f69b5a91e6d189ff62feb",
					},
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerAuthMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverAuthMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerAuthMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestServerCountMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(serverCountMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ServerCountMsg
		want []byte
	}{
		{
			in:   ServerCountMsg{SubscriptionID: "subid", Count: 100},
			want: jsons[0],
		},
		{
			in:   ServerCountMsg{SubscriptionID: "subid", Count: 100, Approximate: toPtr(false)},
			want: jsons[1],
		},
		{
			in:   ServerCountMsg{SubscriptionID: "subid", Count: 100, Approximate: toPtr(true)},
			want: jsons[2],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestServerCountMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverCountMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ServerCountMsg
		}{
			{
				in:   jsons[0],
				want: ServerCountMsg{SubscriptionID: "subid", Count: 100},
			},
			{
				in: jsons[1],
				want: ServerCountMsg{
					SubscriptionID: "subid",
					Count:          100,
					Approximate:    toPtr(false),
				},
			},
			{
				in:   jsons[2],
				want: ServerCountMsg{SubscriptionID: "subid", Count: 100, Approximate: toPtr(true)},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerCountMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverCountMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerCountMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
}

func TestServerClosedMsg_MarshalJSON(t *testing.T) {
	jsons := bytes.Split(bytes.TrimSpace(serverClosedMsgsValidJSONL), []byte("\n"))

	tests := []struct {
		in   ServerClosedMsg
		want []byte
	}{
		{
			in:   ServerClosedMsg{SubscriptionID: "subid", Msg: ""},
			want: jsons[0],
		},
		{
			in:   ServerClosedMsg{SubscriptionID: "subid", Msg: "with msg"},
			want: jsons[1],
		},
		{
			in: ServerClosedMsg{
				SubscriptionID: "subid",
				Msg:            "with prefix",
				MsgPrefix:      ServerClosedMsgPrefixPoW,
			},
			want: jsons[2],
		},
		{
			in: ServerClosedMsg{
				SubscriptionID: "subid",
				Msg:            "with prefix",
				MsgPrefix:      ServerClosedMsgPrefixDuplicate,
			},
			want: jsons[3],
		},
		{
			in: ServerClosedMsg{
				SubscriptionID: "subid",
				Msg:            "with prefix",
				MsgPrefix:      ServerClosedMsgPrefixBlocked,
			},
			want: jsons[4],
		},
		{
			in: ServerClosedMsg{
				SubscriptionID: "subid",
				Msg:            "with prefix",
				MsgPrefix:      ServerClosedMsgPrefixRateLimited,
			},
			want: jsons[5],
		},
		{
			in: ServerClosedMsg{
				SubscriptionID: "subid",
				Msg:            "with prefix",
				MsgPrefix:      ServerClosedMsgPrefixInvalid,
			},
			want: jsons[6],
		},
		{
			in: ServerClosedMsg{
				SubscriptionID: "subid",
				Msg:            "with prefix",
				MsgPrefix:      ServerClosedMsgPrefixError,
			},
			want: jsons[7],
		},
		{
			in:   ServerClosedMsg{SubscriptionID: "subid", Msg: "prefix: with unknown prefix"},
			want: jsons[8],
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("want: %s, got: %s", tt.want, got)
			}
		})
	}
}

func TestServerClosedMsg_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverClosedMsgsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want ServerClosedMsg
		}{
			{
				in:   jsons[0],
				want: ServerClosedMsg{SubscriptionID: "subid", Msg: ""},
			},
			{
				in:   jsons[1],
				want: ServerClosedMsg{SubscriptionID: "subid", Msg: "with msg"},
			},
			{
				in: jsons[2],
				want: ServerClosedMsg{
					SubscriptionID: "subid",
					Msg:            "with prefix",
					MsgPrefix:      ServerClosedMsgPrefixPoW,
				},
			},
			{
				in: jsons[3],
				want: ServerClosedMsg{
					SubscriptionID: "subid",
					Msg:            "with prefix",
					MsgPrefix:      ServerClosedMsgPrefixDuplicate,
				},
			},
			{
				in: jsons[4],
				want: ServerClosedMsg{
					SubscriptionID: "subid",
					Msg:            "with prefix",
					MsgPrefix:      ServerClosedMsgPrefixBlocked,
				},
			},
			{
				in: jsons[5],
				want: ServerClosedMsg{
					SubscriptionID: "subid",
					Msg:            "with prefix",
					MsgPrefix:      ServerClosedMsgPrefixRateLimited,
				},
			},
			{
				in: jsons[6],
				want: ServerClosedMsg{
					SubscriptionID: "subid",
					Msg:            "with prefix",
					MsgPrefix:      ServerClosedMsgPrefixInvalid,
				},
			},
			{
				in: jsons[7],
				want: ServerClosedMsg{
					SubscriptionID: "subid",
					Msg:            "with prefix",
					MsgPrefix:      ServerClosedMsgPrefixError,
				},
			},
			{
				in:   jsons[8],
				want: ServerClosedMsg{SubscriptionID: "subid", Msg: "prefix: with unknown prefix"},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerClosedMsg
				err := json.Unmarshal(tt.in, &got)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				assert.EqualExportedValues(t, tt.want, got)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(serverClosedMsgsInvalidJSONL), []byte("\n"))

		for i, b := range jsons {
			t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
				var got ServerClosedMsg
				err := json.Unmarshal(b, &got)
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
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
	var closed ServerMsg = &ServerClosedMsg{
		SubscriptionID: "sub_id",
		MsgPrefix:      ServerClosedMsgPrefixError,
		Msg:            "msg",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(eose)
		json.Marshal(event)
		json.Marshal(notice)
		json.Marshal(ok)
		json.Marshal(auth)
		json.Marshal(count)
		json.Marshal(closed)
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

func BenchmarkServerMsg_Marshal_Closed(b *testing.B) {
	var closed ServerMsg = &ServerClosedMsg{
		SubscriptionID: "sub_id",
		MsgPrefix:      ServerClosedMsgPrefixError,
		Msg:            "msg",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(closed)
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

func TestEvent_UnmarshalJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		jsons := bytes.Split(bytes.TrimSpace(eventsValidJSONL), []byte("\n"))

		tests := []struct {
			in   []byte
			want Event
		}{
			{
				in: jsons[0],
				want: Event{
					ID:        "dc097cd6bd76f2d8816f8a2d294e8442173228e5b24fb946aa05dd89339c9168",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723212754,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "",
					Sig:       "5d2f49649a4f448d13757ee563fd1b8fa04e4dc1931dd34763fb7df40a082cbdc4e136c733177d3b96a0321f8783fd6b218fea046e039a23d99b1ab9e2d8b45f",
				},
			},
			{
				in: jsons[1],
				want: Event{
					ID:        "07e782ba4b5fe85b91264d03c445c339b8783e0ea2ae3bdfb0122eda513d86ac",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723212830,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "with content",
					Sig:       "a714af322ebf22bec24364319efc635e88230c099a8e08238fff1f4f5608494cddbcc77bef426cc653e4994ac23625553500d05244dd28ed2ac3096cff0387af",
				},
			},
			{
				in: jsons[2],
				want: Event{
					ID:        "80cfa4cff224ad441b9cb50fdce68a47f30a2d7e38fa2b06f2ddac748bbac137",
					Pubkey:    "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
					CreatedAt: 1723212930,
					Kind:      1,
					Tags: []Tag{
						{"key1", "value1"},
						{"key2"},
						{"key3", "value3-1", "value3-2"},
					},
					Content: "with tags",
					Sig:     "7553bd8efb6e338e58cd9b807225dfd5d71043f94173aa234c7727aa28236009ee34aa76422e6d3912a5d104100c49148b043202df7f8b258782b1816434d1ea",
				},
			},
		}

		for i, tt := range tests {
			t.Run(fmt.Sprintf("event-%d", i), func(t *testing.T) {
				var ev Event
				err := ev.UnmarshalJSON(tt.in)
				if err != nil {
					t.Errorf("failed to unmarshal: %v", err)
					return
				}
				if !reflect.DeepEqual(tt.want, ev) {
					t.Errorf("want: %v, got: %v", tt.want, ev)
					return
				}
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		sc := bufio.NewScanner(bytes.NewReader(eventsInvalidJSONL))
		for i := 0; sc.Scan(); i++ {
			t.Run(fmt.Sprintf("event-%d", i), func(t *testing.T) {
				var ev Event
				err := ev.UnmarshalJSON(sc.Bytes())
				if err == nil {
					t.Errorf("unexpected success")
					return
				}
				t.Logf("expected error: %v", err)
			})
		}
	})
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
