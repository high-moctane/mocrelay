package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/high-moctane/mocrelay"
)

func queryEvent(
	ctx context.Context,
	db *sql.DB,
	f *mocrelay.ReqFilter,
) (events []*mocrelay.Event, err error) {
	rows, err := db.QueryContext(ctx, `
		select
			id, pubkey, created_at, kind, tags, content, sig
		from
			events
		order by
			created_at desc
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			event mocrelay.Event
			tags  []byte
		)
		if err := rows.Scan(
			&event.ID,
			&event.Pubkey,
			&event.CreatedAt,
			&event.Kind,
			&tags,
			&event.Content,
			&event.Sig,
		); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if err := json.Unmarshal(tags, &event.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		events = append(events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return
}
