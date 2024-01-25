package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/high-moctane/mocrelay"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func Test_insertEvent(t *testing.T) {
	tests := []struct {
		name         string
		event        *mocrelay.Event
		wantAffected int64
		wantErr      bool
	}{
		{
			name: "insert event",
			event: &mocrelay.Event{
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
			wantAffected: 1,
			wantErr:      false,
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

			affected, err := insertEvent(ctx, db, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("insertEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, affected, tt.wantAffected)
		})
	}
}
