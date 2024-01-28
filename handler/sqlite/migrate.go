package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		create table if not exists events (
			key        text not null primary key,
			id         text not null,
			pubkey     text not null,
			created_at integer not null,
			kind       integer not null,
			tags       blob not null,
			content    text not null,
			sig        text not null
		) strict;
	`)
	if err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_id on events (id);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_id: %w", err)
	}

	return nil
}
