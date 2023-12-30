package mocrelay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

type testSimpleHandlerInterfaceEntry struct {
	input ClientMsg
	want  []ServerMsg
}

func helperTestSimpleHandlerInterface(
	t *testing.T,
	h SimpleHandlerInterface,
	entries []testSimpleHandlerInterfaceEntry,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, "", "/", new(bufio.Reader))

	r, err := h.HandleStart(r)
	assert.NoError(t, err)

	for i, entry := range entries {
		smsgCh, err := h.HandleClientMsg(r, entry.input)
		assert.NoError(t, err)

		var smsgs []ServerMsg
		if smsgCh != nil {
		L:
			for {
				select {
				case <-ctx.Done():
					break L
				case smsg, ok := <-smsgCh:
					if !ok {
						break L
					}
					smsgs = append(smsgs, smsg)
				}
			}
		}
		assert.EqualValuesf(t, entry.want, smsgs, "at %d", i)
	}

	err = h.HandleStop(r)
	assert.NoError(t, err)
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
							IDs: []string{
								"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
							},
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
		name    string
		cap     int
		entries []testSimpleHandlerInterfaceEntry
	}{
		{
			name: "req",
			cap:  10,
			entries: []testSimpleHandlerInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
						NewServerEOSEMsg("sub_id"),
					},
				},
			},
		},
		{
			name: "req event",
			cap:  10,
			entries: []testSimpleHandlerInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id1",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
						NewServerEOSEMsg("sub_id1"),
					},
				},
				{
					input: &ClientEventMsg{
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
					want: []ServerMsg{
						NewServerOKMsg(
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientEventMsg{
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
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						},
					},
					want: []ServerMsg{
						NewServerOKMsg(
							"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id2",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
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
								{
									"p",
									"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
								},
							},
							Content: "powa",
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						}),
						NewServerEventMsg("sub_id2", &Event{
							ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							CreatedAt: 1693156107,
							Kind:      1,
							Tags:      []Tag{},
							Content:   "ぽわ〜",
							Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
						}),
						NewServerEOSEMsg("sub_id2"),
					},
				},
			},
		},
		{
			name: "req event: duplicate",
			cap:  10,
			entries: []testSimpleHandlerInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id1",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
						NewServerEOSEMsg("sub_id1"),
					},
				},
				{
					input: &ClientEventMsg{
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
					want: []ServerMsg{
						NewServerOKMsg(
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientEventMsg{
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
					want: []ServerMsg{
						NewServerOKMsg(
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							false,
							"duplicate: ",
							"already have this event",
						),
					},
				},
				{
					input: &ClientEventMsg{
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
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						},
					},
					want: []ServerMsg{
						NewServerOKMsg(
							"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id2",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
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
								{
									"p",
									"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
								},
							},
							Content: "powa",
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						}),
						NewServerEventMsg("sub_id2", &Event{
							ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							CreatedAt: 1693156107,
							Kind:      1,
							Tags:      []Tag{},
							Content:   "ぽわ〜",
							Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
						}),
						NewServerEOSEMsg("sub_id2"),
					},
				},
			},
		},
		{
			name: "req event: capacity",
			cap:  2,
			entries: []testSimpleHandlerInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id1",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
						NewServerEOSEMsg("sub_id1"),
					},
				},
				{
					input: &ClientEventMsg{
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
					want: []ServerMsg{
						NewServerOKMsg(
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientEventMsg{
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
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						},
					},
					want: []ServerMsg{
						NewServerOKMsg(
							"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientEventMsg{
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
					want: []ServerMsg{
						NewServerOKMsg(
							"5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id2",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
						NewServerEventMsg("sub_id2", &Event{
							ID:        "5b2b799aa222cdf555d46b72a868014ffe602d9842cc29d3bedca794c8b32b3e",
							Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
							CreatedAt: 1694867396,
							Kind:      1,
							Tags:      []Tag{},
							Content:   "ﾈﾑ",
							Sig:       "3aea60cff4da67e42e5216064250357f957cd44bf12bd88c337070dbd57592d11605bfe8f8d8697494559d0770958a9b19b6b18a1926db97bf1555f225e98d0c",
						}),
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
								{
									"p",
									"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
								},
							},
							Content: "powa",
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						}),
						NewServerEOSEMsg("sub_id2"),
					},
				},
			},
		},
		{
			name: "req event: kind5",
			cap:  10,
			entries: []testSimpleHandlerInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id1",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
						NewServerEOSEMsg("sub_id1"),
					},
				},
				{
					input: &ClientEventMsg{
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
					want: []ServerMsg{
						NewServerOKMsg(
							"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientEventMsg{
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
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						},
					},
					want: []ServerMsg{
						NewServerOKMsg(
							"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientEventMsg{
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
					want: []ServerMsg{
						NewServerOKMsg(
							"powa",
							true,
							"",
							"",
						),
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub_id2",
						ReqFilters:     []*ReqFilter{{}},
					},
					want: []ServerMsg{
						NewServerEventMsg("sub_id2", &Event{
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
						}),
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
								{
									"p",
									"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
								},
							},
							Content: "powa",
							Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
						}),
						NewServerEOSEMsg("sub_id2"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSimpleCacheHandler(tt.cap)
			helperTestSimpleHandlerInterface(t, h, tt.entries)
		})
	}
}

func TestCacheHandlerDumpRestore(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		events []*Event
	}{
		{
			name:   "null",
			size:   10,
			events: nil,
		},
		{
			name: "one",
			size: 10,
			events: []*Event{
				{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
			},
		},
		{
			name: "many",
			size: 10,
			events: []*Event{
				{
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
					}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
				},
				{
					ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693156107,
					Kind:      1,
					Tags:      []Tag{},
					Content:   "ぽわ〜",
					Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewCacheHandler(tt.size)

			b, err := json.Marshal(tt.events)
			assert.NoError(t, err)

			r := bytes.NewReader(b)
			err = h.Restore(r)
			assert.NoError(t, err)

			buf := new(bytes.Buffer)
			err = h.Dump(buf)
			assert.NoError(t, err)

			assert.Equal(t, b, buf.Bytes())
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

type testSimpleMiddlewareInterfaceEntry struct {
	input     any
	wantCmsgs []ClientMsg
	wantSmsgs []ServerMsg
}

func helperTestSimpleMiddlewareInterface(
	t *testing.T,
	m SimpleMiddlewareInterface,
	entries []testSimpleMiddlewareInterfaceEntry,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	r, _ := http.NewRequestWithContext(ctx, "", "/", new(bufio.Reader))

	r, err := m.HandleStart(r)
	assert.NoError(t, err)

	for i, entry := range entries {
		switch msg := entry.input.(type) {
		case ClientMsg:
			cmsgCh, sMsgCh, err := m.HandleClientMsg(r, msg)
			assert.NoError(t, err)

			var smsgs []ServerMsg
			if sMsgCh != nil {
			L1:
				for {
					select {
					case <-ctx.Done():
						break L1
					case smsg, ok := <-sMsgCh:
						if !ok {
							break L1
						}
						smsgs = append(smsgs, smsg)
					}
				}
			}
			assert.EqualValuesf(t, entry.wantSmsgs, smsgs, "at %d", i)

			var cmsgs []ClientMsg
			if cmsgCh != nil {
			L2:
				for {
					select {
					case <-ctx.Done():
						break L2
					case cmsg, ok := <-cmsgCh:
						if !ok {
							break L2
						}
						cmsgs = append(cmsgs, cmsg)
					}
				}
			}
			assert.EqualValuesf(t, entry.wantCmsgs, cmsgs, "at %d", i)

		case ServerMsg:
			sMsgCh, err := m.HandleServerMsg(r, msg)
			assert.NoError(t, err)

			var smsgs []ServerMsg
		L3:
			for {
				select {
				case <-ctx.Done():
					break L3
				case smsg, ok := <-sMsgCh:
					if !ok {
						break L3
					}
					smsgs = append(smsgs, smsg)
				}
			}

			assert.EqualValuesf(t, entry.wantSmsgs, smsgs, "at %d", i)

		default:
			panicf("[%d]: invalid msg type", i)
		}
	}

	err = m.HandleStop(r)
	assert.NoError(t, err)
}

func TestMaxSubscriptionsMiddleware(t *testing.T) {
	tests := []struct {
		name    string
		maxSubs int
		entries []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:    "req only",
			maxSubs: 3,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub1",
						ReqFilters:     []*ReqFilter{{}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub1",
							ReqFilters:     []*ReqFilter{{}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub2",
						ReqFilters:     []*ReqFilter{{}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub2",
							ReqFilters:     []*ReqFilter{{}, {}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub3",
						ReqFilters:     []*ReqFilter{{}, {}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub3",
							ReqFilters:     []*ReqFilter{{}, {}, {}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub4",
						ReqFilters:     []*ReqFilter{{}, {}, {}, {}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("sub4", "", "too many req: max subscriptions is 3"),
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub5",
						ReqFilters:     []*ReqFilter{{}, {}, {}, {}, {}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("sub5", "", "too many req: max subscriptions is 3"),
					},
				},
			},
		},
		{
			name:    "with close",
			maxSubs: 3,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub1",
						ReqFilters:     []*ReqFilter{{}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub1",
							ReqFilters:     []*ReqFilter{{}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub2",
						ReqFilters:     []*ReqFilter{{}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub2",
							ReqFilters:     []*ReqFilter{{}, {}},
						},
					},
				},
				{
					input: &ClientCloseMsg{
						SubscriptionID: "sub1",
					},
					wantCmsgs: []ClientMsg{
						&ClientCloseMsg{
							SubscriptionID: "sub1",
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub3",
						ReqFilters:     []*ReqFilter{{}, {}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub3",
							ReqFilters:     []*ReqFilter{{}, {}, {}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub4",
						ReqFilters:     []*ReqFilter{{}, {}, {}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub4",
							ReqFilters:     []*ReqFilter{{}, {}, {}, {}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub5",
						ReqFilters:     []*ReqFilter{{}, {}, {}, {}, {}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("sub5", "", "too many req: max subscriptions is 3"),
					},
				},
			},
		},
		{
			name:    "with duplication",
			maxSubs: 3,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub1",
						ReqFilters:     []*ReqFilter{{}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub1",
							ReqFilters:     []*ReqFilter{{}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub2",
						ReqFilters:     []*ReqFilter{{}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub2",
							ReqFilters:     []*ReqFilter{{}, {}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub1",
						ReqFilters:     []*ReqFilter{{}, {}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub1",
							ReqFilters:     []*ReqFilter{{}, {}, {}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub3",
						ReqFilters:     []*ReqFilter{{}, {}, {}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "sub3",
							ReqFilters:     []*ReqFilter{{}, {}, {}, {}},
						},
					},
				},
				{
					input: &ClientReqMsg{
						SubscriptionID: "sub4",
						ReqFilters:     []*ReqFilter{{}, {}, {}, {}, {}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("sub4", "", "too many req: max subscriptions is 3"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleMaxSubscriptionsMiddleware(tt.maxSubs)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestMaxReqFiltersMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		maxFilters int
		entries    []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:       "req: ok",
			maxFilters: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "req1",
						ReqFilters:     []*ReqFilter{{}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "req1",
							ReqFilters:     []*ReqFilter{{}, {}},
						},
					},
				},
			},
		},
		{
			name:       "req: ng",
			maxFilters: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "req1",
						ReqFilters:     []*ReqFilter{{}, {}, {}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("req1", "", "too many req filters: max filters is 2"),
					},
				},
			},
		},
		{
			name:       "count: ok",
			maxFilters: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientCountMsg{
						SubscriptionID: "count1",
						ReqFilters:     []*ReqFilter{{}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientCountMsg{
							SubscriptionID: "count1",
							ReqFilters:     []*ReqFilter{{}, {}},
						},
					},
				},
			},
		},
		{
			name:       "count: ng",
			maxFilters: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientCountMsg{
						SubscriptionID: "count1",
						ReqFilters:     []*ReqFilter{{}, {}, {}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg(
							"count1",
							"",
							"too many count filters: max filters is 2",
						),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleMaxReqFiltersMiddleware(tt.maxFilters)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestMaxLimitMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		maxLimit int
		entries  []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:     "req: ok: no limit",
			maxLimit: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "req1",
						ReqFilters:     []*ReqFilter{{}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "req1",
							ReqFilters:     []*ReqFilter{{}, {}},
						},
					},
				},
			},
		},
		{
			name:     "req: ok: with limit",
			maxLimit: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "req1",
						ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(2))}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "req1",
							ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(2))}},
						},
					},
				},
			},
		},
		{
			name:     "req: ng",
			maxLimit: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "req1",
						ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(3))}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("req1", "", "too large limit: max limit is 2"),
					},
				},
			},
		},
		{
			name:     "count: ok: no limit",
			maxLimit: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientCountMsg{
						SubscriptionID: "count",
						ReqFilters:     []*ReqFilter{{}, {}},
					},
					wantCmsgs: []ClientMsg{
						&ClientCountMsg{
							SubscriptionID: "count",
							ReqFilters:     []*ReqFilter{{}, {}},
						},
					},
				},
			},
		},
		{
			name:     "count: ok: with limit",
			maxLimit: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientCountMsg{
						SubscriptionID: "count",
						ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(2))}},
					},
					wantCmsgs: []ClientMsg{
						&ClientCountMsg{
							SubscriptionID: "count",
							ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(2))}},
						},
					},
				},
			},
		},
		{
			name:     "count: ng",
			maxLimit: 2,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientCountMsg{
						SubscriptionID: "count",
						ReqFilters:     []*ReqFilter{{}, {Limit: toPtr(int64(3))}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("count", "", "too large limit: max limit is 2"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleMaxLimitMiddleware(tt.maxLimit)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestMaxSubIDLengthMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		maxLimit int
		entries  []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:     "req: ok",
			maxLimit: 5,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "12345",
						ReqFilters:     []*ReqFilter{{}},
					},
					wantCmsgs: []ClientMsg{
						&ClientReqMsg{
							SubscriptionID: "12345",
							ReqFilters:     []*ReqFilter{{}},
						},
					},
				},
			},
		},
		{
			name:     "req: ng",
			maxLimit: 5,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientReqMsg{
						SubscriptionID: "123456",
						ReqFilters:     []*ReqFilter{{}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("123456", "", "too long subid: max subid length is 5"),
					},
				},
			},
		},
		{
			name:     "count: ok",
			maxLimit: 5,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientCountMsg{
						SubscriptionID: "12345",
						ReqFilters:     []*ReqFilter{{}},
					},
					wantCmsgs: []ClientMsg{
						&ClientCountMsg{
							SubscriptionID: "12345",
							ReqFilters:     []*ReqFilter{{}},
						},
					},
				},
			},
		},
		{
			name:     "count: ng",
			maxLimit: 5,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientCountMsg{
						SubscriptionID: "123456",
						ReqFilters:     []*ReqFilter{{}},
					},
					wantSmsgs: []ServerMsg{
						NewServerClosedMsg("123456", "", "too long subid: max subid length is 5"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleMaxSubIDLengthMiddleware(tt.maxLimit)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestMaxEventTagsMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		maxEventTags int
		entries      []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:         "ok",
			maxEventTags: 3,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:   "id1",
							Tags: []Tag{{"e", "powa"}, {"e", "powa"}, {"e", "powa"}},
						},
					},
					wantCmsgs: []ClientMsg{
						&ClientEventMsg{
							&Event{
								ID:   "id1",
								Tags: []Tag{{"e", "powa"}, {"e", "powa"}, {"e", "powa"}},
							},
						},
					},
				},
			},
		},
		{
			name:         "ng",
			maxEventTags: 3,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:   "id1",
							Tags: []Tag{{"e", "powa"}, {"e", "powa"}, {"e", "powa"}, {"e", "powa"}},
						},
					},
					wantSmsgs: []ServerMsg{
						NewServerOKMsg(
							"id1",
							false,
							"",
							"too many event tags: max event tags is 3",
						),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleMaxEventTagsMiddleware(tt.maxEventTags)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestMaxContentLengthMiddleware(t *testing.T) {
	tests := []struct {
		name             string
		maxContentLength int
		entries          []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:             "ok",
			maxContentLength: 5,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:      "id1",
							Content: "12345",
						},
					},
					wantCmsgs: []ClientMsg{
						&ClientEventMsg{
							&Event{
								ID:      "id1",
								Content: "12345",
							},
						},
					},
				},
			},
		},
		{
			name:             "ng",
			maxContentLength: 5,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:      "id1",
							Content: "123456",
						},
					},
					wantSmsgs: []ServerMsg{
						NewServerOKMsg(
							"id1",
							false,
							"",
							"too long content: max content length is 5",
						),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleMaxContentLengthMiddleware(tt.maxContentLength)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestCreatedAtLowerLimitMiddleware(t *testing.T) {
	tests := []struct {
		name    string
		lower   int64
		entries []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:  "ok: past",
			lower: 10,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:        "id1",
							CreatedAt: time.Now().Unix() - 9,
						},
					},
					wantCmsgs: []ClientMsg{
						&ClientEventMsg{
							&Event{
								ID:        "id1",
								CreatedAt: time.Now().Unix() - 9,
							},
						},
					},
				},
			},
		},
		{
			name:  "ok: future",
			lower: 10,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:        "id1",
							CreatedAt: time.Now().Unix() + 20,
						},
					},
					wantCmsgs: []ClientMsg{
						&ClientEventMsg{
							&Event{
								ID:        "id1",
								CreatedAt: time.Now().Unix() + 20,
							},
						},
					},
				},
			},
		},
		{
			name:  "ng",
			lower: 10,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:        "id1",
							CreatedAt: time.Now().Unix() - 11,
						},
					},
					wantSmsgs: []ServerMsg{
						NewServerOKMsg("id1", false, "", "too old created_at"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleCreatedAtLowerLimitMiddleware(tt.lower)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestCreatedAtUpperLimitMiddleware(t *testing.T) {
	tests := []struct {
		name    string
		upper   int64
		entries []testSimpleMiddlewareInterfaceEntry
	}{
		{
			name:  "ok: past",
			upper: 10,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:        "id1",
							CreatedAt: time.Now().Unix() - 20,
						},
					},
					wantCmsgs: []ClientMsg{
						&ClientEventMsg{
							&Event{
								ID:        "id1",
								CreatedAt: time.Now().Unix() - 20,
							},
						},
					},
				},
			},
		},
		{
			name:  "ok: future",
			upper: 10,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:        "id1",
							CreatedAt: time.Now().Unix() + 9,
						},
					},
					wantCmsgs: []ClientMsg{
						&ClientEventMsg{
							&Event{
								ID:        "id1",
								CreatedAt: time.Now().Unix() + 9,
							},
						},
					},
				},
			},
		},
		{
			name:  "ng",
			upper: 10,
			entries: []testSimpleMiddlewareInterfaceEntry{
				{
					input: &ClientEventMsg{
						&Event{
							ID:        "id1",
							CreatedAt: time.Now().Unix() + 11,
						},
					},
					wantSmsgs: []ServerMsg{
						NewServerOKMsg("id1", false, "", "too far off created_at"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSimpleCreatedAtUpperLimitMiddleware(tt.upper)
			helperTestSimpleMiddlewareInterface(t, m, tt.entries)
		})
	}
}

func TestEventCreatedAtMiddleware(t *testing.T) {
	tests := []struct {
		name  string
		from  time.Duration
		to    time.Duration
		input []ClientMsg
		want  []ServerMsg
	}{
		{
			name: "ok: past",
			from: -10 * time.Second,
			to:   10 * time.Second,
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
			name: "ok: future",
			from: -10 * time.Second,
			to:   10 * time.Second,
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
			name: "ng: past",
			from: -10 * time.Second,
			to:   10 * time.Second,
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
		{
			name: "ng: future",
			from: -10 * time.Second,
			to:   10 * time.Second,
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
			h = NewEventCreatedAtMiddleware(tt.from, tt.to)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}

func TestEventAllowFilterMiddleware(t *testing.T) {
	tests := []struct {
		name   string
		filter []*ReqFilter
		input  []ClientMsg
		want   []ServerMsg
	}{
		{
			name:   "no filterling",
			filter: []*ReqFilter{{}},
			input: []ClientMsg{
				&ClientEventMsg{&Event{ID: "id1"}},
				&ClientEventMsg{&Event{ID: "id2"}},
				&ClientEventMsg{&Event{ID: "id3"}},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
				NewServerOKMsg("id2", true, "", ""),
				NewServerOKMsg("id3", true, "", ""),
			},
		},
		{
			name:   "filterling",
			filter: []*ReqFilter{{IDs: []string{"id1", "id3"}}},
			input: []ClientMsg{
				&ClientEventMsg{&Event{ID: "id1"}},
				&ClientEventMsg{&Event{ID: "id2"}},
				&ClientEventMsg{&Event{ID: "id3"}},
			},
			want: []ServerMsg{
				NewServerOKMsg("id1", true, "", ""),
				NewServerOKMsg("id2", false, "blocked: ", "the event is not allowed"),
				NewServerOKMsg("id3", true, "", ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewReqFiltersEventMatchers(tt.filter)

			var h Handler
			h = NewRouterHandler(100)
			h = NewRecvEventAllowFilterMiddleware(m)(h)
			helperTestHandler(t, h, tt.input, tt.want)
		})
	}
}
