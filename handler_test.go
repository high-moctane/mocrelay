package mocrelay

import (
	"bufio"
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/high-moctane/mocrelay/nostr"
	"github.com/high-moctane/mocrelay/utils"
)

func helperTestHandler(t *testing.T, h Handler, in []nostr.ClientMsg, out []nostr.ServerMsg) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, "", "/", new(bufio.Reader))
	recv := make(chan nostr.ClientMsg, len(in))
	send := make(chan nostr.ServerMsg, len(out)*2)

	go h.Handle(r, recv, send)

	for _, msg := range in {
		recv <- msg
	}

	var gots []nostr.ServerMsg

	for i := 0; i < len(out); i++ {
		select {
		case <-ctx.Done():
			var gotsstr []string
			for i := 0; i < len(gots); i++ {
				b, err := gots[i].MarshalJSON()
				if err != nil {
					t.Errorf("marshal error: %s", err)
					return
				}
				gotsstr = append(gotsstr, string(b))
			}
			t.Errorf("timeout: gots: %#+v", gotsstr)
			return

		case got := <-send:
			gots = append(gots, got)
		}
	}

	var gotjsons, wantjsons []string

	for _, v := range gots {
		j, err := v.MarshalJSON()
		if err != nil {
			t.Errorf("unexpect error: %s", err)
			return
		}
		gotjsons = append(gotjsons, string(j))
	}
	for _, v := range out {
		j, err := v.MarshalJSON()
		if err != nil {
			t.Errorf("unexpect error: %s", err)
			return
		}
		wantjsons = append(wantjsons, string(j))
	}

	slices.Sort(gotjsons)
	slices.Sort(wantjsons)

	assert.EqualValues(t, wantjsons, gotjsons)

	select {
	case msg := <-send:
		t.Errorf("too much server msg: %#+v", msg)
	default:
	}
}

func TestRouter_Handle(t *testing.T) {
	tests := []struct {
		name  string
		input []nostr.ClientMsg
		want  []nostr.ServerMsg
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name: "req",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
		{
			name: "req with filter event",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id_with_filter",
					ReqFilters:     []*nostr.ReqFilter{{IDs: utils.Ptr([]string{"49"})}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
				nostr.NewServerEOSEMsg("sub_id_with_filter"),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEventMsg("sub_id_with_filter", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(nil)
			helperTestHandler(t, router, tt.input, tt.want)
		})
	}
}

func TestCacheHandler(t *testing.T) {
	tests := []struct {
		name  string
		cap   int
		input []nostr.ClientMsg
		want  []nostr.ServerMsg
	}{
		{
			name:  "empty",
			cap:   10,
			input: nil,
			want:  nil,
		},
		{
			name: "req",
			cap:  10,
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			cap:  10,
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id1",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						},
						Content: "powa",
						Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id2",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id1"),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				nostr.NewServerEventMsg("sub_id2", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id2", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEOSEMsg("sub_id2"),
			},
		},
		{
			name: "req event: duplicate",
			cap:  10,
			input: []nostr.ClientMsg{
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						},
						Content: "powa",
						Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					false,
					nostr.ServerOKMsgPrefixDuplicate,
					"already have this event",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event: capacity",
			cap:  2,
			input: []nostr.ClientMsg{
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						},
						Content: "powa",
						Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1694867396,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ﾈﾑ",
						Sig:       "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
					},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				nostr.NewServerOKMsg(
					"5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
					true,
					"",
					"",
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1694867396,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ﾈﾑ",
					Sig:       "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
				}),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
				}),
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event: kind5",
			cap:  10,
			input: []nostr.ClientMsg{
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						},
						Content: "powa",
						Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "powa",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1694867396,
						Kind:      5,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							},
						},
						Content: "",
						Sig:     "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
					},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				nostr.NewServerOKMsg("powa", true, "", ""),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
				}),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "powa",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1694867396,
					Kind:      5,
					Tags: []nostr.Tag{
						{"e", "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c"},
					},
					Content: "",
					Sig:     "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
				}),
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewCacheHandler(tt.cap)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestEventCache(t *testing.T) {
	reg := []*nostr.Event{
		{ID: "reg0", Pubkey: "reg0", Kind: 1, CreatedAt: 0},
		{ID: "reg1", Pubkey: "reg1", Kind: 1, CreatedAt: 1},
		{ID: "reg2", Pubkey: "reg2", Kind: 1, CreatedAt: 2},
		{ID: "reg3", Pubkey: "reg3", Kind: 1, CreatedAt: 3},
		{ID: "reg4", Pubkey: "reg4", Kind: 1, CreatedAt: 4},
		{ID: "reg5", Pubkey: "reg5", Kind: 1, CreatedAt: 5},
	}
	rep := []*nostr.Event{
		{ID: "rep0", Pubkey: "rep0", Kind: 0, CreatedAt: 0},
		{ID: "rep1", Pubkey: "rep0", Kind: 0, CreatedAt: 1},
		{ID: "rep2", Pubkey: "rep0", Kind: 10000, CreatedAt: 2},
		{ID: "rep3", Pubkey: "rep0", Kind: 10000, CreatedAt: 3},
		{ID: "rep4", Pubkey: "rep1", Kind: 0, CreatedAt: 4},
		{ID: "rep5", Pubkey: "rep1", Kind: 0, CreatedAt: 5},
		{ID: "rep6", Pubkey: "rep0", Kind: 0, CreatedAt: 6},
		{ID: "rep7", Pubkey: "rep0", Kind: 0, CreatedAt: 7},
	}
	prep := []*nostr.Event{
		{ID: "prep0", Pubkey: "prep0", Kind: 30000, CreatedAt: 0, Tags: []nostr.Tag{{"d", "tag0"}}},
		{ID: "prep1", Pubkey: "prep0", Kind: 30000, CreatedAt: 1, Tags: []nostr.Tag{{"d", "tag1"}}},
		{ID: "prep2", Pubkey: "prep0", Kind: 30000, CreatedAt: 2, Tags: []nostr.Tag{{"d", "tag0"}}},
		{ID: "prep3", Pubkey: "prep0", Kind: 30000, CreatedAt: 3, Tags: []nostr.Tag{{"d", "tag1"}}},
		{ID: "prep4", Pubkey: "prep1", Kind: 30000, CreatedAt: 4, Tags: []nostr.Tag{{"d", "tag0"}}},
		{ID: "prep5", Pubkey: "prep1", Kind: 30000, CreatedAt: 5, Tags: []nostr.Tag{{"d", "tag1"}}},
		{ID: "prep6", Pubkey: "prep1", Kind: 30000, CreatedAt: 6, Tags: []nostr.Tag{{"d", "tag0"}}},
		{ID: "prep7", Pubkey: "prep1", Kind: 30000, CreatedAt: 7, Tags: []nostr.Tag{{"d", "tag1"}}},
	}

	tests := []struct {
		name string
		cap  int
		in   []*nostr.Event
		want []*nostr.Event
	}{
		{
			"empty",
			3,
			nil,
			nil,
		},
		{
			"two",
			3,
			[]*nostr.Event{reg[0], reg[1]},
			[]*nostr.Event{reg[1], reg[0]},
		},
		{
			"many",
			3,
			[]*nostr.Event{reg[0], reg[1], reg[2], reg[3], reg[4]},
			[]*nostr.Event{reg[4], reg[3], reg[2]},
		},
		{
			"two: reverse",
			3,
			[]*nostr.Event{reg[1], reg[0]},
			[]*nostr.Event{reg[1], reg[0]},
		},
		{
			"random",
			3,
			[]*nostr.Event{reg[3], reg[2], reg[1], reg[4], reg[0]},
			[]*nostr.Event{reg[4], reg[3], reg[2]},
		},
		{
			"duplicate",
			3,
			[]*nostr.Event{reg[1], reg[1], reg[0], reg[1]},
			[]*nostr.Event{reg[1], reg[0]},
		},
		{
			"replaceable",
			3,
			[]*nostr.Event{rep[0], rep[1], rep[2]},
			[]*nostr.Event{rep[2], rep[1]},
		},
		{
			"replaceable: reverse",
			3,
			[]*nostr.Event{rep[1], rep[0], rep[2]},
			[]*nostr.Event{rep[2], rep[1]},
		},
		{
			"replaceable: different pubkey",
			3,
			[]*nostr.Event{rep[0], rep[4], rep[5]},
			[]*nostr.Event{rep[5], rep[0]},
		},
		{
			"param replaceable",
			5,
			[]*nostr.Event{prep[0], prep[1], prep[2], prep[3]},
			[]*nostr.Event{prep[3], prep[2]},
		},
		{
			"param replaceable: different pubkey",
			6,
			[]*nostr.Event{prep[0], prep[1], prep[2], prep[3], prep[4], prep[5], prep[6], prep[7]},
			[]*nostr.Event{prep[7], prep[6], prep[3], prep[2]},
		},
		{
			"param replaceable: different pubkey",
			6,
			[]*nostr.Event{prep[0], prep[1], prep[2], prep[3], prep[4], prep[5], prep[6], prep[7]},
			[]*nostr.Event{prep[7], prep[6], prep[3], prep[2]},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newEventCache(tt.cap)
			for _, e := range tt.in {
				c.Add(e)
			}
			got := c.Find(NewReqFilterMatcher(new(nostr.ReqFilter)))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeHandler(t *testing.T) {
	tests := []struct {
		name  string
		cap   int
		input []nostr.ClientMsg
		want  []nostr.ServerMsg
	}{
		{
			name:  "empty",
			cap:   10,
			input: nil,
			want:  nil,
		},
		{
			name: "req",
			cap:  10,
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			cap:  10,
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id1",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						},
						Content: "powa",
						Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id2",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id1"),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				nostr.NewServerEventMsg("sub_id1", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id1", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEventMsg("sub_id2", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id2", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					},
					Content: "powa",
					Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEOSEMsg("sub_id2"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h1 := NewRouter(nil)
			h2 := NewCacheHandler(tt.cap)
			h := NewMergeHandler(h1, h2)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestRecvEventUniquefyMiddleware(t *testing.T) {
	tests := []struct {
		name  string
		input []nostr.ClientMsg
		want  []nostr.ServerMsg
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name: "req",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
		{
			name: "req with filter event",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id_with_filter",
					ReqFilters:     []*nostr.ReqFilter{{IDs: utils.Ptr([]string{"49"})}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
				nostr.NewServerEOSEMsg("sub_id_with_filter"),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEventMsg("sub_id_with_filter", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handler Handler
			handler = NewRouter(nil)
			handler = NewRecvEventUniquefyMiddleware(2)(handler)

			helperTestHandler(t, handler, tt.input, tt.want)
		})
	}
}

func TestSendEventUniquefyMiddleware(t *testing.T) {
	tests := []struct {
		name  string
		input []nostr.ClientMsg
		want  []nostr.ServerMsg
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name: "req",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
		{
			name: "req with filter event",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*nostr.ReqFilter{{}},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id_with_filter",
					ReqFilters:     []*nostr.ReqFilter{{IDs: utils.Ptr([]string{"49"})}},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []nostr.Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&nostr.ClientEventMsg{
					Event: &nostr.Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []nostr.Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
								"",
								"root",
							},
							{
								"p",
								"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							},
						}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				},
			},
			want: []nostr.ServerMsg{
				nostr.NewServerEOSEMsg("sub_id"),
				nostr.NewServerEOSEMsg("sub_id_with_filter"),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []nostr.Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				nostr.NewServerEventMsg("sub_id", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEventMsg("sub_id_with_filter", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []nostr.Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
				nostr.NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					nostr.ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handler Handler
			handler = NewRouter(nil)
			handler = NewSendEventUniquefyMiddleware(5)(handler)

			helperTestHandler(t, handler, tt.input, tt.want)
		})
	}
}
