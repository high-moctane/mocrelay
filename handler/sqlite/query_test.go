package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/high-moctane/mocrelay"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func Test_queryEvent(t *testing.T) {
	tests := []struct {
		name    string
		input   []*mocrelay.Event
		fs      []*mocrelay.ReqFilter
		want    []*mocrelay.Event
		wantErr bool
	}{
		{
			name: "query event",
			input: []*mocrelay.Event{
				{
					ID:        "aa",
					Pubkey:    "bb",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{{}},
			want: []*mocrelay.Event{
				{
					ID:        "aa",
					Pubkey:    "bb",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query event with two filters",
			input: []*mocrelay.Event{
				{
					ID:        "aa",
					Pubkey:    "bb",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{{}, {}},
			want: []*mocrelay.Event{
				{
					ID:        "aa",
					Pubkey:    "bb",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query ids",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					IDs: []string{"aa22", "aa33"},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query authors",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb33",
					CreatedAt: 1237,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Authors: []string{"bb11", "bb22"},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query kinds",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb11",
					CreatedAt: 1237,
					Kind:      1001,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Kinds: []int64{1, 1000},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query kinds",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb11",
					CreatedAt: 1237,
					Kind:      1001,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Kinds: []int64{1, 1000},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query tags",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 2,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 3,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value2"},
						{"p"},
					},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb11",
					CreatedAt: 4,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value2"},
						{"p", "value"},
					},
				},
				{
					ID:        "aa55",
					Pubkey:    "bb11",
					CreatedAt: 5,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p"},
					},
				},
				{
					ID:        "aa66",
					Pubkey:    "bb11",
					CreatedAt: 6,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value3"},
						{"p"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Tags: map[string][]string{
						"#e": {"value1", "value3"},
						"#p": {""},
					},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa66",
					Pubkey:    "bb11",
					CreatedAt: 6,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value3"},
						{"p"},
					},
				},
				{
					ID:        "aa55",
					Pubkey:    "bb11",
					CreatedAt: 5,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p"},
					},
				},
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query since",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Since: toPtr[int64](1235),
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query until",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Until: toPtr[int64](1235),
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query limit",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Limit: toPtr[int64](2),
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "query all",
			input: func() []*mocrelay.Event {
				var events []*mocrelay.Event

				var createdAt int64 = 1
				for _, pubkey := range []string{"bb11", "bb22", "bb33"} {
					for _, kind := range []int64{1, 1000, 1001} {
						for _, evalue := range []string{"", "value1", "value2"} {
							for _, pvalue := range []string{"", "value1", "value2"} {
								id := fmt.Sprintf(
									"%s-%d-%s-%s",
									pubkey,
									kind,
									evalue,
									pvalue,
								)

								events = append(events, &mocrelay.Event{
									ID:        hex.EncodeToString([]byte(id)),
									Pubkey:    pubkey,
									CreatedAt: createdAt,
									Kind:      kind,
									Tags: []mocrelay.Tag{
										{"e", evalue},
										{"p", pvalue},
									},
								})
								createdAt++
							}
						}
					}
				}

				return events
			}(),
			fs: []*mocrelay.ReqFilter{
				{
					Authors: []string{"bb11", "bb33"},
					Kinds:   []int64{1, 1000},
					Tags: map[string][]string{
						"#e": {"value1", "value2"},
						"#p": {""},
					},
					Since: toPtr[int64](50),
					Until: toPtr[int64](67),
					Limit: toPtr[int64](2),
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "626233332d313030302d76616c7565312d",
					Pubkey:    "bb33",
					CreatedAt: 67,
					Kind:      1000,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p", ""},
					},
				},
				{
					ID:        "626233332d312d76616c7565322d",
					Pubkey:    "bb33",
					CreatedAt: 61,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value2"},
						{"p", ""},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple filters",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 2,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 3,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb11",
					CreatedAt: 4,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa55",
					Pubkey:    "bb22",
					CreatedAt: 5,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa66",
					Pubkey:    "bb22",
					CreatedAt: 6,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					Authors: []string{"bb22"},
				},
				{
					IDs: []string{"aa11", "aa55"},
				},
				{
					IDs: []string{"aa22", "aa55"},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "aa66",
					Pubkey:    "bb22",
					CreatedAt: 6,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa55",
					Pubkey:    "bb22",
					CreatedAt: 5,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb11",
					CreatedAt: 2,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
			},
			wantErr: false,
		},
		{
			name: "kind5",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 2,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 3,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value"},
					},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb22",
					CreatedAt: 4,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value"},
					},
				},
				{
					ID:        "aa55",
					Pubkey:    "bb11",
					CreatedAt: 5,
					Kind:      10000,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa66",
					Pubkey:    "bb22",
					CreatedAt: 6,
					Kind:      10000,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa77",
					Pubkey:    "bb11",
					CreatedAt: 7,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value1"},
					},
				},
				{
					ID:        "cc55",
					Pubkey:    "bb11",
					CreatedAt: 100,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa11"},
						{"e", "aa22"},
						{"e", "aa77"},
						{"a", "10000:bb11"},
						{"a", "30000:bb11:value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{{}},
			want: []*mocrelay.Event{
				{
					ID:        "cc55",
					Pubkey:    "bb11",
					CreatedAt: 100,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa11"},
						{"e", "aa22"},
						{"e", "aa77"},
						{"a", "10000:bb11"},
						{"a", "30000:bb11:value"},
					},
				},
				{
					ID:        "aa66",
					Pubkey:    "bb22",
					CreatedAt: 6,
					Kind:      10000,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb22",
					CreatedAt: 4,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 2,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
			},
		},
		{
			name: "kind5 with ids",
			input: []*mocrelay.Event{
				{
					ID:        "aa11",
					Pubkey:    "bb11",
					CreatedAt: 1,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 2,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa33",
					Pubkey:    "bb11",
					CreatedAt: 3,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value"},
					},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb22",
					CreatedAt: 4,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value"},
					},
				},
				{
					ID:        "aa55",
					Pubkey:    "bb11",
					CreatedAt: 5,
					Kind:      10000,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa66",
					Pubkey:    "bb22",
					CreatedAt: 6,
					Kind:      10000,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa77",
					Pubkey:    "bb11",
					CreatedAt: 7,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value1"},
					},
				},
				{
					ID:        "cc55",
					Pubkey:    "bb11",
					CreatedAt: 100,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa11"},
						{"e", "aa22"},
						{"e", "aa77"},
						{"a", "10000:bb11"},
						{"a", "30000:bb11:value"},
					},
				},
			},
			fs: []*mocrelay.ReqFilter{
				{
					IDs: []string{"aa11", "aa22", "aa33", "aa44", "aa55", "aa66", "aa77", "cc55"},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "cc55",
					Pubkey:    "bb11",
					CreatedAt: 100,
					Kind:      5,
					Tags: []mocrelay.Tag{
						{"e", "aa11"},
						{"e", "aa22"},
						{"e", "aa77"},
						{"a", "10000:bb11"},
						{"a", "30000:bb11:value"},
					},
				},
				{
					ID:        "aa66",
					Pubkey:    "bb22",
					CreatedAt: 6,
					Kind:      10000,
					Tags:      []mocrelay.Tag{},
				},
				{
					ID:        "aa44",
					Pubkey:    "bb22",
					CreatedAt: 4,
					Kind:      30000,
					Tags: []mocrelay.Tag{
						{"d", "value"},
					},
				},
				{
					ID:        "aa22",
					Pubkey:    "bb22",
					CreatedAt: 2,
					Kind:      1,
					Tags:      []mocrelay.Tag{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite3", ":memory:?cache=shared")
			if err != nil {
				t.Fatalf("failed to open db: %v", err)
			}
			defer db.Close()

			if _, err := db.ExecContext(ctx, `pragma foreign_keys = on`); err != nil {
				t.Fatalf("failed to enable foreign keys: %v", err)
			}

			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("failed to migrate: %v", err)
			}

			if err := insertEvents(ctx, db, 0, tt.input); err != nil {
				t.Fatalf("failed to insert event: %v", err)
			}

			got, err := queryEvent(ctx, db, 0, tt.fs, NoLimit)
			if (err != nil) != tt.wantErr {
				t.Errorf("queryEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)

			q, p, _ := buildEventQuery(tt.fs, 0, NoLimit)
			t.Logf("query: %s, param: %v", q, p)
		})
	}
}
