package nostr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEvent(t *testing.T) {
	type Expect struct {
		Event *Event
		Err   error
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
				Err: nil,
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
				Err: nil,
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
				Err:   ErrInvalidEvent,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			event, err := ParseEvent(tt.Input)
			if tt.Expect.Err != nil || err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			assert.EqualExportedValues(t, *tt.Expect.Event, *event)
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseEvent(input)
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
			want: false,
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
			want: []byte(`[0,"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",1693157791,1,[["e","d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c","","root"],["p","dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"]],"powa"]`),
			err:  nil,
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
		name string
		in   *Event
		want bool
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
			want: true,
			err:  nil,
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
			want: false,
			err:  nil,
		},
		{
			name: "ng: nil",
			in:   nil,
			want: false,
			err:  ErrNilEvent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Verify()
			if err != nil || tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
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
