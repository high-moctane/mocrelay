package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		create table if not exists events (
			id         text not null primary key,
			pubkey     text not null,
			created_at integer not null,
			kind       integer not null,
			tags       blob not null,
			content    text not null,
			sig        text not null
		) strict, without rowid;
	`)
	if err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	return nil
}
