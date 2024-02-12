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
			pubkey_hash    integer not null,
			pubkey         text    not null,
			created_at     integer not null,
			kind           integer not null
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

	// index events_event_key_hash
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_event_key_hash on events (event_key_hash);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_event_key_hash: %w", err)
	}

	// index events_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_created_at on events (created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_created_at: %w", err)
	}

	// index events_id_hash_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_id_hash_created_at on events (id_hash, created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_id_hash_created_at: %w", err)
	}

	// index events_pubkey_hash_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_pubkey_hash_created_at on events (pubkey_hash, created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_pubkey_hash_created_at: %w", err)
	}

	// index events_kind_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_events_kind_created_at on events (kind, created_at desc);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_events_kind_created_at: %w", err)
	}

	// table event_payloads
	if _, err := db.ExecContext(ctx, `
		create table if not exists event_payloads (
			record_id      integer primary key autoincrement,
			event_key_hash integer not null,
			event_key      text    not null,
			tags		   blob    not null,
			content        text    not null,
			sig            text    not null
		) strict;
	`); err != nil {
		return fmt.Errorf("failed to create event_payloads table: %w", err)
	}

	// index event_payloads_event_key_hash
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_event_payloads_event_key_hash on event_payloads (event_key_hash);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_event_payloads_event_key_hash: %w", err)
	}

	// trigger event_payloads_delete_on_casecade_event_key
	if _, err := db.ExecContext(ctx, `
		create trigger if not exists tr_event_payloads_delete_on_casecade_event_key
		before delete on events
		begin
			delete from event_payloads
			where
				event_key_hash = old.event_key_hash
				and
				event_key = old.event_key;
		end;
	`); err != nil {
		return fmt.Errorf(
			"failed to create trigger tr_event_payloads_delete_on_casecade_event_key: %w",
			err,
		)
	}
	if _, err := db.ExecContext(ctx, `
		create trigger if not exists tr_event_payloads_update_on_casecade_event_key
		before update on events
		begin
			delete from event_payloads
			where
				event_key_hash = old.event_key_hash
				and
				event_key = old.event_key;
		end;
	`); err != nil {
		return fmt.Errorf(
			"failed to create trigger tr_event_payloads_update_on_casecade_event_key: %w",
			err,
		)
	}

	// table event_tags
	if _, err := db.ExecContext(ctx, `
		create table if not exists event_tags (
			record_id      integer primary key autoincrement,
			key_value_hash integer not null,
			key            text    not null,
			value          text    not null,
			created_at     integer not null,
			event_key_hash integer not null,
			event_key	   text    not null
		) strict;
	`); err != nil {
		return fmt.Errorf("failed to create event_tags table: %w", err)
	}

	// index event_tags_key_value_hash_created_at
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_event_tags_key_value_hash_created_at on event_tags (key_value_hash, created_at desc);
	`); err != nil {
		return fmt.Errorf(
			"failed to create index idx_event_tags_key_value_hash_created_at: %w",
			err,
		)
	}

	// index event_tags_event_key_hash
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_event_tags_event_key_hash on event_tags (event_key_hash);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_event_tags_event_key_hash: %w", err)
	}

	// trigger event_tags_delete_on_casecade_event_key
	if _, err := db.ExecContext(ctx, `
		create trigger if not exists tr_event_tags_delete_on_casecade_event_key
		before delete on events
		begin
			delete from event_tags
			where
				event_key_hash = old.event_key_hash
				and
				event_key = old.event_key;
		end;
	`); err != nil {
		return fmt.Errorf(
			"failed to create trigger tr_event_tags_delete_on_casecade_event_key: %w",
			err,
		)
	}
	if _, err := db.ExecContext(ctx, `
		create trigger if not exists tr_event_tags_update_on_casecade_event_key
		before update on events
		begin
			delete from event_tags
			where
				event_key_hash = old.event_key_hash
				and
				event_key = old.event_key;
		end;
	`); err != nil {
		return fmt.Errorf(
			"failed to create trigger tr_event_tags_update_on_casecade_event_key: %w",
			err,
		)
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

	// index deleted_events_event_key_or_id_hash
	if _, err := db.ExecContext(ctx, `
		create index if not exists idx_deleted_events_event_key_or_id_hash on deleted_events (event_key_or_id_hash);
	`); err != nil {
		return fmt.Errorf("failed to create index idx_deleted_events_event_key_or_id_hash: %w", err)
	}

	return nil
}
