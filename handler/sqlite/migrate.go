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
			event_key  text    not null primary key,
			id         text    not null,
			pubkey     text    not null,
			created_at integer not null,
			kind       integer not null,
			tags       blob    not null,
			content    text    not null,
			sig        text    not null
		) strict;
	`); err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	// index events_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_created_at on events (created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_created_at: %w", err)
	}

	// table deleted_events
	if _, err := db.ExecContext(ctx, `
		create table if not exists deleted_events (
			event_key_or_id text not null,
			pubkey		    text not null,

			constraint deleted_events_primary_key primary key (event_key_or_id, pubkey)
		) strict, without rowid;
	`); err != nil {
		return fmt.Errorf("failed to create deleted_events table: %w", err)
	}

	// table lookups
	if _, err := db.ExecContext(ctx, `
		create table if not exists lookups (
			id         integer primary key autoincrement,
			value      text    not null,
			what       text    not null,
			created_at integer not null,
			event_key  text    not null,

			foreign key (event_key) references events (event_key) on delete cascade
		) strict;
	`); err != nil {
		return fmt.Errorf("failed to create lookups table: %w", err)
	}

	// index lookups_value_what_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_lookups_value_what_created_at on lookups (value, what, created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_lookups_value_what_created_at: %w", err)
	}

	// index lookups_event_key
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_lookups_event_key on lookups (event_key);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_lookups_event_key: %w", err)
	}

	return nil
}
