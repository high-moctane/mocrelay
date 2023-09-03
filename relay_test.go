package mocrelay

import (
	"bufio"
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/high-moctane/mocrelay/nostr"
	"github.com/high-moctane/mocrelay/utils"
	"github.com/stretchr/testify/assert"
)

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
					Filters:        nostr.Filters{{}},
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
					Filters:        nostr.Filters{{}},
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
						Tags:      []nostr.Tag{{"e", "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c", "", "root"}, {"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"}}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
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
					Tags:      []nostr.Tag{{"e", "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c", "", "root"}, {"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"}}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg("d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c", true, nostr.ServerOKMsgPrefixNoPrefix, ""),
				nostr.NewServerOKMsg("49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2", true, nostr.ServerOKMsgPrefixNoPrefix, ""),
			},
		},
		{
			name: "req with filter event",
			input: []nostr.ClientMsg{
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id",
					Filters:        nostr.Filters{{}},
				},
				&nostr.ClientReqMsg{
					SubscriptionID: "sub_id_with_filter",
					Filters:        nostr.Filters{{IDs: utils.ToRef([]string{"49"})}},
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
						Tags:      []nostr.Tag{{"e", "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c", "", "root"}, {"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"}}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
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
					Tags:      []nostr.Tag{{"e", "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c", "", "root"}, {"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"}}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerEventMsg("sub_id_with_filter", &nostr.Event{
					ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					CreatedAt: 1693157791,
					Kind:      1,
					Tags:      []nostr.Tag{{"e", "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c", "", "root"}, {"p", "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"}}, Content: "powa", Sig: "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8"},
				),
				nostr.NewServerOKMsg("d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c", true, nostr.ServerOKMsgPrefixNoPrefix, ""),
				nostr.NewServerOKMsg("49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2", true, nostr.ServerOKMsgPrefixNoPrefix, ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(nil)

			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(3*time.Second))
			defer cancel()

			r, _ := http.NewRequestWithContext(ctx, "", "/", new(bufio.Reader))
			recv := make(chan nostr.ClientMsg, 1)
			send := make(chan nostr.ServerMsg, 1)

			go router.Handle(r, recv, send)

			for _, msg := range tt.input {
				recv <- msg
			}

			var gots []nostr.ServerMsg

			for i := 0; i < len(tt.want); i++ {
				select {
				case <-ctx.Done():
					t.Errorf("timeout")
					return

				case got := <-send:
					gots = append(gots, got)
				}
			}

			slices.SortStableFunc(gots, func(a, b nostr.ServerMsg) int {
				if a.MsgType() < b.MsgType() {
					return -1
				}
				return 0
			})
			slices.SortStableFunc(tt.want, func(a, b nostr.ServerMsg) int {
				if a.MsgType() < b.MsgType() {
					return -1
				}
				return 0
			})

			for i := 0; i < len(tt.want); i++ {
				assert.Equal(t, tt.want[i], gots[i])
			}

			select {
			case msg := <-send:
				t.Errorf("too much server msg: %#+v", msg)
			default:
			}
		})
	}
}
