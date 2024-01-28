package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

func TestMigrate(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite3 database: %v", err)
	}
	defer db.Close()

	err = Migrate(ctx, db)
	assert.NoError(t, err)
}
