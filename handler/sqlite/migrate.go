package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	// table events
	if _, err := db.ExecContext(ctx, `
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
	`); err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	// index events_id
	if _, err := db.ExecContext(ctx, `
		create unique index if not exists idx_events_id on events (id);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_id: %w", err)
	}

	// index events_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_created_at on events (created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_created_at: %w", err)
	}

	// table deleted_keys
	if _, err := db.ExecContext(ctx, `
		create table if not exists deleted_keys (
			key    text not null,
			pubkey text not null,

			primary key (key, pubkey)
		) strict, without rowid;
	`); err != nil {
		return fmt.Errorf("failed to create deleted_keys table: %w", err)
	}

	return nil
}
