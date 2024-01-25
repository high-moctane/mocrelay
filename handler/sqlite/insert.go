package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/high-moctane/mocrelay"
)

func insertEvent(
	ctx context.Context,
	db *sql.DB,
	event *mocrelay.Event,
) (affected int64, err error) {
	tagStr, err := json.Marshal(event.Tags)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal tags: %w", err)
	}

	res, err := db.ExecContext(ctx, `
		insert into events (
			id, pubkey, created_at, kind, tags, content, sig
		) values (?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.Pubkey, event.CreatedAt, event.Kind, tagStr, event.Content, event.Sig)
	if err != nil {
		return 0, fmt.Errorf("failed to insert event: %w", err)
	}

	affected, err = res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return
}
