package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/high-moctane/mocrelay"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func Test_queryEvent(t *testing.T) {
	tests := []struct {
		name    string
		input   []*mocrelay.Event
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
					Content: "content",
					Sig:     "sig",
				},
			},
			want: []*mocrelay.Event{
				{
					ID:        "id",
					Pubkey:    "pubkey",
					CreatedAt: 1234,
					Kind:      1,
					Tags: []mocrelay.Tag{
						{"e", "value"},
					},
					Content: "content",
					Sig:     "sig",
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

			got, err := queryEvent(ctx, db, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("queryEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, got, tt.want)
		})
	}
}
