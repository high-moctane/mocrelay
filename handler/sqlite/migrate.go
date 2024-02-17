package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	ddls := []string{
		`create table if not exists xxhash_seed (
			seed      integer not null primary key
		) without rowid, strict;`,

		`create table if not exists events (
			event_key       integer not null primary key,
			id              blob    not null,
			pubkey          blob    not null,
			created_at      integer not null,
			kind            integer not null
		) strict;`,
		`create index if not exists idx_events_created_at
			on events (created_at desc);`,

		`create table if not exists event_payloads (
			event_key integer not null primary key,
			tags	  blob    not null,
			content   text    not null,
			sig       blob    not null
		) strict;`,

		`create table if not exists event_tags (
			record_id		   integer not null primary key,
			event_key          integer not null,
			key                blob    not null,
			value              blob    not null
		) strict;`,
		`create index if not exists idx_event_tags_event_key
			on event_tags (event_key);`,

		`create table if not exists deleted_event_keys (
			event_key integer not null,
			pubkey    blob    not null,
			constraint pk_deleted_event_keys
				primary key (event_key, pubkey)
		) without rowid, strict;`,

		`create table if not exists deleted_event_ids (
			id     blob not null,
			pubkey blob not null,
			constraint pk_deleted_event_ids
				primary key (id, pubkey)
		) without rowid, strict;`,

		`create table if not exists lookup_hashes (
			hash       integer not null,
			created_at integer not null,
			event_key  integer not null,

			constraint pk_lookups
				primary key (hash, created_at desc, event_key)
		) without rowid, strict;`,
		`create index if not exists idx_lookup_hashes_event_key
			on lookup_hashes (event_key);`,
	}

	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("failed to execute ddl: %w", err)
		}
	}

	return nil
}

func SetPragmas(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		`pragma recursive_triggers = on;`,
		`pragma foreign_keys = on;`,
	}

	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("failed to execute pragma: %w", err)
		}
	}

	return nil
}

func setOrLoadXXHashSeed(ctx context.Context, db *sql.DB) (uint32, error) {
	var seed uint32
	if err := db.QueryRowContext(ctx, "select seed from xxhash_seed").Scan(&seed); err != nil {
		if err == sql.ErrNoRows {
			seed = rand.Uint32()
			if _, err := db.ExecContext(ctx, "insert into xxhash_seed (seed) values (?)", seed); err != nil {
				return 0, fmt.Errorf("failed to insert xxhash_seed: %w", err)
			}
		} else {
			return 0, fmt.Errorf("failed to scan xxhash_seed: %w", err)
		}
	}
	return seed, nil
}
