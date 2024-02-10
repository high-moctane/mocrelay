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
			record_id      integer primary key autoincrement,
			event_key_hash integer not null,
			event_key      text    not null,
			id_hash        integer not null,
			id             text    not null,
			pubkey         text    not null,
			created_at     integer not null,
			kind           integer not null,
			tags           blob    not null,
			content        text    not null,
			sig            text    not null
		) strict;
	`); err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	// unique index events_event_key
	if _, err := db.ExecContext(ctx, `
		create unique index if not exists idx_events_event_key on events (event_key);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_event_key: %w", err)
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
			record_id            integer primary key autoincrement,
			event_key_or_id_hash integer not null,
			event_key_or_id      text    not null,
			pubkey		         text    not null
		) strict;
	`); err != nil {
		return fmt.Errorf("failed to create deleted_events table: %w", err)
	}

	// table lookups
	if _, err := db.ExecContext(ctx, `
		create table if not exists lookups (
			record_id       integer primary key autoincrement,
			what_value_hash integer not null,
			what            text    not null,
			value           text    not null,
			event_key_hash  integer not null,
			event_key       text    not null,
			created_at      integer not null
		) strict;
	`); err != nil {
		return fmt.Errorf("failed to create lookups table: %w", err)
	}

	// index lookups_what_value_hash_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_lookups_what_value_hash_created_at on lookups (what_value_hash, created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_lookups_what_value_hash_created_at: %w", err)
	}

	// index lookups_event_key_hash
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_lookups_event_key_hash on lookups (event_key_hash);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_lookups_event_key_hash: %w", err)
	}

	// trigger events_delete_cascade_lookups
	if _, err := db.ExecContext(ctx, `
		create trigger if not exists events_delete_cascade_lookups
		after delete on events
		begin
			delete from lookups
			where
				event_key_hash = old.event_key_hash
				and
				event_key = old.event_key;
		end;
	`); err != nil {
		return fmt.Errorf("failed to create trigger events_delete_cascade_lookups: %w", err)
	}

	return nil
}
