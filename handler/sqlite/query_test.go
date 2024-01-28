package sqlite

import (
	"context"
	"database/sql"
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
		f       *mocrelay.ReqFilter
		want    []*mocrelay.Event
		wantErr bool
	}{
		{
			name: "query event",
			input: []*mocrelay.Event{
				{
					ID:        "id",
					Pubkey:    "pubkey",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{},
			want: []*mocrelay.Event{
				{
					ID:        "id",
					Pubkey:    "pubkey",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				IDs: []string{"id2", "id3"},
			},
			want: []*mocrelay.Event{
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey2",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id4",
					Pubkey:    "pubkey3",
					CreatedAt: 1237,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				Authors: []string{"pubkey1", "pubkey2"},
			},
			want: []*mocrelay.Event{
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey2",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id1",
					Pubkey:    "pubkey1",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      10000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id4",
					Pubkey:    "pubkey1",
					CreatedAt: 1237,
					Kind:      10001,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				Kinds: []int64{1, 10000},
			},
			want: []*mocrelay.Event{
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      10000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id1",
					Pubkey:    "pubkey1",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      10000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id4",
					Pubkey:    "pubkey1",
					CreatedAt: 1237,
					Kind:      10001,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				Kinds: []int64{1, 10000},
			},
			want: []*mocrelay.Event{
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      10000,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id1",
					Pubkey:    "pubkey1",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 2,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 3,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value2"},
						{"p"},
					},
				},
				{
					ID:        "id4",
					Pubkey:    "pubkey1",
					CreatedAt: 4,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value2"},
						{"p", "value"},
					},
				},
				{
					ID:        "id5",
					Pubkey:    "pubkey1",
					CreatedAt: 5,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p"},
					},
				},
				{
					ID:        "id6",
					Pubkey:    "pubkey1",
					CreatedAt: 6,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value3"},
						{"p"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				Tags: map[string][]string{
					"e": {"value1", "value3"},
					"p": {""},
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "id6",
					Pubkey:    "pubkey1",
					CreatedAt: 6,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value3"},
						{"p"},
					},
				},
				{
					ID:        "id5",
					Pubkey:    "pubkey1",
					CreatedAt: 5,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p"},
					},
				},
				{
					ID:        "id1",
					Pubkey:    "pubkey1",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				Since: toPtr[int64](1235),
			},
			want: []*mocrelay.Event{
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				Until: toPtr[int64](1235),
			},
			want: []*mocrelay.Event{
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id1",
					Pubkey:    "pubkey1",
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
					ID:        "id1",
					Pubkey:    "pubkey1",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
					CreatedAt: 1235,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
			},
			f: &mocrelay.ReqFilter{
				Limit: toPtr[int64](2),
			},
			want: []*mocrelay.Event{
				{
					ID:        "id3",
					Pubkey:    "pubkey1",
					CreatedAt: 1236,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
				},
				{
					ID:        "id2",
					Pubkey:    "pubkey1",
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
				for _, pubkey := range []string{"pubkey1", "pubkey2", "pubkey3"} {
					for _, kind := range []int64{1, 10000, 10001} {
						for _, evalue := range []string{"", "value1", "value2"} {
							for _, pvalue := range []string{"", "value1", "value2"} {
								events = append(events, &mocrelay.Event{
									ID: fmt.Sprintf(
										"%s-%d-%s-%s",
										pubkey,
										kind,
										evalue,
										pvalue,
									),
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
			f: &mocrelay.ReqFilter{
				Authors: []string{"pubkey1", "pubkey3"},
				Kinds:   []int64{1, 10000},
				Tags: map[string][]string{
					"e": {"value1", "value2"},
					"p": {""},
				},
				Since: toPtr[int64](50),
				Until: toPtr[int64](67),
				Limit: toPtr[int64](2),
			},
			want: []*mocrelay.Event{
				{
					ID:        "pubkey3-10000-value1-",
					Pubkey:    "pubkey3",
					CreatedAt: 67,
					Kind:      10000,
					Tags: []mocrelay.Tag{
						{"e", "value1"},
						{"p", ""},
					},
				},
				{
					ID:        "pubkey3-1-value2-",
					Pubkey:    "pubkey3",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite3", ":memory:?cache=shared")
			if err != nil {
				t.Fatalf("failed to open db: %v", err)
			}

			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("failed to migrate: %v", err)
			}

			for _, event := range tt.input {
				_, err := insertEvent(ctx, db, event)
				if err != nil {
					t.Fatalf("failed to insert event: %v", err)
				}
			}

			got, err := queryEvent(ctx, db, tt.f)
			if (err != nil) != tt.wantErr {
				t.Errorf("queryEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
