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
			sig        text not null,
			hashed_id  integer not null
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

	// index events_hashed_id
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_hashed_id on events (hashed_id);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_hashed_id: %w", err)
	}

	// table deleted_keys
	if _, err := db.ExecContext(ctx, `
		create table if not exists deleted_keys (
			value  text not null,
			tag	   text not null,
			pubkey text not null,

			primary key (value, tag, pubkey)
		) strict, without rowid;
	`); err != nil {
		return fmt.Errorf("failed to create deleted_keys table: %w", err)
	}

	// table hash_seed
	if _, err := db.ExecContext(ctx, `
		create table if not exists hash_seed (
			seed integer not null primary key
		) strict, without rowid;
	`); err != nil {
		return fmt.Errorf("failed to create hash_seed table: %w", err)
	}

	// table hashes
	if _, err := db.ExecContext(ctx, `
		create table if not exists hashes (
			hashed_value integer not null,
			created_at   integer not null,
			hashed_id    integer not null,

			primary key (hashed_value, created_at, hashed_id)
		) strict, without rowid;
	`); err != nil {
		return fmt.Errorf("failed to create hashes table: %w", err)
	}

	return nil
}