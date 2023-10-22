package mocrelay

import (
	"bufio"
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func helperTestHandler(t *testing.T, h Handler, in []ClientMsg, out []ServerMsg) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, "", "/", new(bufio.Reader))
	recv := make(chan ClientMsg, len(in))
	send := make(chan ServerMsg, len(out)*2)

	errCh := make(chan error, 1)

	go func() {
		defer cancel()
		errCh <- h.Handle(r, recv, send)
	}()

	for _, msg := range in {
		time.Sleep(1 * time.Millisecond)
		recv <- msg
	}

	var gots []ServerMsg
	var gotjsons, wantjsons []string
	for _, v := range out {
		j, err := v.MarshalJSON()
		if err != nil {
			t.Errorf("unexpect error: %s", err)
			return
		}
		wantjsons = append(wantjsons, string(j)+"\n")
	}

Loop:
	for i := 0; i < len(out); i++ {
		select {
		case <-ctx.Done():
			break Loop

		case got := <-send:
			gots = append(gots, got)
		}
	}

	for _, v := range gots {
		j, err := v.MarshalJSON()
		if err != nil {
			t.Errorf("unexpect error: %s", err)
			return
		}
		gotjsons = append(gotjsons, string(j)+"\n")
	}

	t.Log(gotjsons, "\n\n", wantjsons)
	t.Log(cmp.Diff(wantjsons, gotjsons))

	slices.Sort(gotjsons)
	slices.Sort(wantjsons)

	assert.EqualValues(t, wantjsons, gotjsons)

	select {
	case msg := <-send:
		t.Errorf("too much server msg: %#+v", msg)
	default:
	}

	cancel()
	t.Logf("error: %v", <-errCh)
}

func TestRouterHandler_Handle(t *testing.T) {
	tests := []struct {
		name  string
		input []ClientMsg
		want  []ServerMsg
	}{
		{
			name:  "empty",
			input: nil,
			want:  nil,
		},
		{
			name: "req",
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []Tag{
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
			want: []ServerMsg{
				NewServerEOSEMsg("sub_id"),
				NewServerEventMsg("sub_id", &Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				NewServerEventMsg("sub_id", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					ServerOKMsgPrefixNoPrefix,
					"",
				),
				NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
		{
			name: "req with filter event",
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "sub_id_with_filter",
					ReqFilters: []*ReqFilter{
						{
							IDs: toPtr(
								[]string{
									"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
								},
							),
						},
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []Tag{
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
			want: []ServerMsg{
				NewServerEOSEMsg("sub_id"),
				NewServerEOSEMsg("sub_id_with_filter"),
				NewServerEventMsg("sub_id", &Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				NewServerEventMsg("sub_id", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				NewServerEventMsg("sub_id_with_filter", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
						{
							"e",
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							"",
							"root",
						},
						{"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					ServerOKMsgPrefixNoPrefix,
					"",
				),
				NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					ServerOKMsgPrefixNoPrefix,
					"",
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouterHandler(100)
			helperTestHandler(t, router, tt.input, tt.want)
		})
	}
}

func TestCacheHandler(t *testing.T) {
	tests := []struct {
		name  string
		cap   int
		input []ClientMsg
		want  []ServerMsg
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
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			cap:  10,
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub_id1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []Tag{
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
				&ClientReqMsg{
					SubscriptionID: "sub_id2",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("sub_id1"),
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				NewServerEventMsg("sub_id2", &Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				NewServerEventMsg("sub_id2", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
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
				NewServerEOSEMsg("sub_id2"),
			},
		},
		{
			name: "req event: duplicate",
			cap:  10,
			input: []ClientMsg{
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []Tag{
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
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					false,
					ServerOKMsgPrefixDuplicate,
					"already have this event",
				),
				NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				NewServerEventMsg("sub_id", &Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				NewServerEventMsg("sub_id", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
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
				NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event: capacity",
			cap:  2,
			input: []ClientMsg{
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []Tag{
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
				&ClientEventMsg{
					Event: &Event{
						ID:        "5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1694867396,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ﾈﾑ",
						Sig:       "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
					},
				},
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				NewServerOKMsg(
					"5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
					true,
					"",
					"",
				),
				NewServerEventMsg("sub_id", &Event{
					ID:        "5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1694867396,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ﾈﾑ",
					Sig:       "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
				}),
				NewServerEventMsg("sub_id", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
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
				NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event: kind5",
			cap:  10,
			input: []ClientMsg{
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []Tag{
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
				&ClientEventMsg{
					Event: &Event{
						ID:        "powa",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1694867396,
						Kind:      5,
						Tags: []Tag{
							{
								"e",
								"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							},
						},
						Content: "",
						Sig:     "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
					},
				},
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				NewServerOKMsg("powa", true, "", ""),
				NewServerEventMsg("sub_id", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
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
				NewServerEventMsg("sub_id", &Event{
					ID:        "powa",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1694867396,
					Kind:      5,
					Tags: []Tag{
						{"e", "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c"},
					},
					Content: "",
					Sig:     "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
				}),
				NewServerEOSEMsg("sub_id"),
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

func TestMergeHandler(t *testing.T) {
	tests := []struct {
		name  string
		cap   int
		input []ClientMsg
		want  []ServerMsg
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
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub_id",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("sub_id"),
			},
		},
		{
			name: "req event",
			cap:  10,
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub_id1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693156107,
						Kind:      1,
						Tags:      []Tag{},
						Content:   "ぽわ〜",
						Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
					},
				},
				&ClientEventMsg{
					Event: &Event{
						ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
						CreatedAt: 1693157791,
						Kind:      1,
						Tags: []Tag{
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
				&ClientReqMsg{
					SubscriptionID: "sub_id2",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("sub_id1"),
				NewServerOKMsg(
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					true,
					"",
					"",
				),
				NewServerOKMsg(
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					true,
					"",
					"",
				),
				NewServerEventMsg("sub_id1", &Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				NewServerEventMsg("sub_id1", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
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
				NewServerEventMsg("sub_id2", &Event{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
				),
				NewServerEventMsg("sub_id2", &Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags: []Tag{
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
				NewServerEOSEMsg("sub_id2"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h1 := NewRouterHandler(100)
			h2 := NewCacheHandler(tt.cap)
			h3 := NewCacheHandler(tt.cap)
			h := NewMergeHandler(h1, h2, h3)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestMaxSubscriptionsMiddleware(t *testing.T) {
	tests := []struct {
		name    string
		maxSubs int
		input   []ClientMsg
		want    []ServerMsg
	}{
		{
			name:    "empty",
			maxSubs: 2,
			input:   nil,
			want:    nil,
		},
		{
			name:    "less",
			maxSubs: 3,
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCloseMsg{
					SubscriptionID: "sub1",
				},
				&ClientReqMsg{
					SubscriptionID: "sub1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "sub2",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCloseMsg{
					SubscriptionID: "sub1",
				},
				&ClientReqMsg{
					SubscriptionID: "sub3",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCloseMsg{
					SubscriptionID: "sub2",
				},
				&ClientCloseMsg{
					SubscriptionID: "sub3",
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("sub1"),
				NewServerEOSEMsg("sub1"),
				NewServerEOSEMsg("sub2"),
				NewServerEOSEMsg("sub3"),
			},
		},
		{
			name:    "more",
			maxSubs: 3,
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "sub1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCloseMsg{
					SubscriptionID: "sub1",
				},
				&ClientReqMsg{
					SubscriptionID: "sub1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "sub2",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "sub3",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "sub4",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCloseMsg{
					SubscriptionID: "sub2",
				},
				&ClientCloseMsg{
					SubscriptionID: "sub3",
				},
				&ClientReqMsg{
					SubscriptionID: "sub5",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("sub1"),
				NewServerEOSEMsg("sub1"),
				NewServerEOSEMsg("sub2"),
				NewServerEOSEMsg("sub3"),
				NewServerNoticeMsg("too many req: sub4: max subscriptions is 3"),
				NewServerEOSEMsg("sub5"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewMaxSubscriptionsMiddleware(tt.maxSubs)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestMaxReqFiltersMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		maxFilters int
		input      []ClientMsg
		want       []ServerMsg
	}{
		{
			name:       "empty",
			maxFilters: 2,
			input:      nil,
			want:       nil,
		},
		{
			name:       "req",
			maxFilters: 2,
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "req1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "req2",
					ReqFilters:     []*ReqFilter{{}, {}},
				},
				&ClientReqMsg{
					SubscriptionID: "req3",
					ReqFilters:     []*ReqFilter{{}, {}, {}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("req1"),
				NewServerEOSEMsg("req2"),
				NewServerNoticeMsg("too many req filters: req3: max filters is 2"),
			},
		},
		{
			name:       "count",
			maxFilters: 2,
			input: []ClientMsg{
				&ClientCountMsg{
					SubscriptionID: "count1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCountMsg{
					SubscriptionID: "count2",
					ReqFilters:     []*ReqFilter{{}, {}},
				},
				&ClientCountMsg{
					SubscriptionID: "count3",
					ReqFilters:     []*ReqFilter{{}, {}, {}},
				},
			},
			want: []ServerMsg{
				NewServerCountMsg("count1", 0, nil),
				NewServerCountMsg("count2", 0, nil),
				NewServerNoticeMsg("too many count filters: count3: max filters is 2"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewMaxReqFiltersMiddleware(tt.maxFilters)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestMaxLimitMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		maxLimit int
		input    []ClientMsg
		want     []ServerMsg
	}{
		{
			name:     "empty",
			maxLimit: 2,
			input:    nil,
			want:     nil,
		},
		{
			name:     "req",
			maxLimit: 2,
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "req1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "req2",
					ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(2))}},
				},
				&ClientReqMsg{
					SubscriptionID: "req3",
					ReqFilters:     []*ReqFilter{{}, {}, {Limit: toPtr(int64(3))}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("req1"),
				NewServerEOSEMsg("req2"),
				NewServerNoticeMsg("too large limit: req3: max limit is 2"),
			},
		},
		{
			name:     "count",
			maxLimit: 2,
			input: []ClientMsg{
				&ClientCountMsg{
					SubscriptionID: "count1",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCountMsg{
					SubscriptionID: "count2",
					ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(2))}},
				},
				&ClientCountMsg{
					SubscriptionID: "count3",
					ReqFilters:     []*ReqFilter{{}, {}, {Limit: toPtr(int64(3))}},
				},
			},
			want: []ServerMsg{
				NewServerCountMsg("count1", 0, nil),
				NewServerCountMsg("count2", 0, nil),
				NewServerNoticeMsg("too large limit: count3: max limit is 2"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewMaxLimitMiddleware(tt.maxLimit)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestMaxSubIDLengthMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		maxLimit int
		input    []ClientMsg
		want     []ServerMsg
	}{
		{
			name:     "req",
			maxLimit: 5,
			input: []ClientMsg{
				&ClientReqMsg{
					SubscriptionID: "12345",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientReqMsg{
					SubscriptionID: "123456",
					ReqFilters:     []*ReqFilter{{}},
				},
			},
			want: []ServerMsg{
				NewServerEOSEMsg("12345"),
				NewServerNoticeMsg("too long subid: 123456: max subid length is 5"),
			},
		},
		{
			name:     "count",
			maxLimit: 4,
			input: []ClientMsg{
				&ClientCountMsg{
					SubscriptionID: "1234",
					ReqFilters:     []*ReqFilter{{}},
				},
				&ClientCountMsg{
					SubscriptionID: "12345",
					ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(2))}},
				},
			},
			want: []ServerMsg{
				NewServerCountMsg("1234", 0, nil),
				NewServerNoticeMsg("too long subid: 12345: max subid length is 4"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewMaxSubIDLengthMiddleware(tt.maxLimit)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestMaxEventTagsMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		maxEventTags int
		input        []ClientMsg
		want         []ServerMsg
	}{
		{
			name:         "test",
			maxEventTags: 3,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:   "id1",
						Tags: []Tag{{"e", "powa"}, {"e", "powa"}, {"e", "powa"}},
					},
				},
				&ClientEventMsg{
					&Event{
						ID:   "id2",
						Tags: []Tag{{"e", "powa"}, {"e", "powa"}, {"e", "powa"}, {"e", "powa"}},
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
				NewServerOKMsg("id2", false, "", "too many event tags: max event tags is 3"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewMaxEventTagsMiddleware(tt.maxEventTags)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestMaxContentLengthMiddleware(t *testing.T) {
	tests := []struct {
		name             string
		maxContentLength int
		input            []ClientMsg
		want             []ServerMsg
	}{
		{
			name:             "test",
			maxContentLength: 5,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:      "id1",
						Content: "12345",
					},
				},
				&ClientEventMsg{
					&Event{
						ID:      "id2",
						Content: "123456",
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
				NewServerOKMsg("id2", false, "", "too long content: max content length is 5"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewMaxContentLengthMiddleware(tt.maxContentLength)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestCreatedAtLowerLimitMiddleware(t *testing.T) {
	tests := []struct {
		name  string
		lower int64
		input []ClientMsg
		want  []ServerMsg
	}{
		{
			name:  "ok: past",
			lower: 10,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:        "id1",
						CreatedAt: time.Now().Unix() - 9,
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
			},
		},
		{
			name:  "ok: future",
			lower: 10,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:        "id1",
						CreatedAt: time.Now().Unix() + 9,
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
			},
		},
		{
			name:  "ng",
			lower: 10,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:        "id1",
						CreatedAt: time.Now().Unix() - 11,
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", false, "", "too old created_at"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewCreatedAtLowerLimitMiddleware(tt.lower)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestCreatedAtUpperLimitMiddleware(t *testing.T) {
	tests := []struct {
		name  string
		upper int64
		input []ClientMsg
		want  []ServerMsg
	}{
		{
			name:  "ok: past",
			upper: 10,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:        "id1",
						CreatedAt: time.Now().Unix() - 9,
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
			},
		},
		{
			name:  "ok: future",
			upper: 10,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:        "id1",
						CreatedAt: time.Now().Unix() + 9,
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
			},
		},
		{
			name:  "ng",
			upper: 10,
			input: []ClientMsg{
				&ClientEventMsg{
					&Event{
						ID:        "id1",
						CreatedAt: time.Now().Unix() + 11,
					},
				},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", false, "", "too far off created_at"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Handler
			h = NewRouterHandler(100)
			h = NewCreatedAtUpperLimitMiddleware(tt.upper)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}
