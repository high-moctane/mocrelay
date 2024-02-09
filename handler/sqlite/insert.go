package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/high-moctane/mocrelay"
)

func insertEvents(
	ctx context.Context,
	db *sql.DB,
	events []*mocrelay.Event,
) (err error) {
	params := buildInsertEventsParams(events)
	if len(params) == 0 {
		return
	}

	// Transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
			return
		}
		err = tx.Commit()
	}()

	eventsStmt, err := tx.PrepareContext(ctx, insertEventsQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare events statement: %w", err)
	}
	defer eventsStmt.Close()

	lookupsStmt, err := tx.PrepareContext(ctx, insertLookupsQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare lookups statement: %w", err)
	}
	defer lookupsStmt.Close()

	deletedEventsStmt, err := tx.PrepareContext(ctx, insertDeletedEventsQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare deleted events statement: %w", err)
	}
	defer deletedEventsStmt.Close()

	// Insert
	for _, p := range params {
		res, err := eventsStmt.ExecContext(ctx, p.Events...)
		if err != nil {
			return fmt.Errorf("failed to insert events: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get affected rows: %w", err)
		}
		if affected == 0 {
			continue
		}

		for _, lookup := range p.Lookups {
			if _, err := lookupsStmt.ExecContext(ctx, lookup...); err != nil {
				return fmt.Errorf("failed to insert lookups: %w", err)
			}
		}

		for _, deletedEvent := range p.DeletedEvents {
			if _, err := deletedEventsStmt.ExecContext(ctx, deletedEvent...); err != nil {
				return fmt.Errorf("failed to insert deleted events: %w", err)
			}
		}
	}

	return
}

type insertEventsParams struct {
	Events        []any
	Lookups       [][]any
	DeletedEvents [][]any
}

var emptyTagsBytes = []byte("[]")

func buildInsertEventsParams(events []*mocrelay.Event) []insertEventsParams {
	ret := make([]insertEventsParams, 0, len(events))

	for _, event := range events {
		eventKey := getEventKey(event)
		if eventKey == "" {
			continue
		}

		events, err := buildInsertEventsParamsEvent(event, eventKey)
		if err != nil {
			continue
		}

		ret = append(ret, insertEventsParams{
			Events:        events,
			Lookups:       buildInsertEventsParamsLookups(event, eventKey),
			DeletedEvents: buildInsertEventsParamsDeletedEvents(event),
		})
	}

	return ret
}

const insertEventsQuery = `
insert into events (
	event_key, id, pubkey, created_at, kind, tags, content, sig
) values
	(?, ?, ?, ?, ?, ?, ?, ?)
on conflict do nothing
`

func buildInsertEventsParamsEvent(event *mocrelay.Event, eventKey string) ([]any, error) {
	var tagsBytes []byte
	if event.Tags == nil {
		tagsBytes = emptyTagsBytes
	} else {
		var err error
		tagsBytes, err = json.Marshal(event.Tags)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tags: %w", err)
		}
	}

	return []any{
		eventKey,
		event.ID,
		event.Pubkey,
		event.CreatedAt,
		event.Kind,
		tagsBytes,
		event.Content,
		event.Sig,
	}, nil
}

const (
	lookupWhatID     = "!"
	lookupWhatPubkey = "@"
	lookupWhatKind   = "#"
)

const insertLookupsQuery = `insert into lookups (value, what, created_at, event_key) values (?, ?, ?, ?) on conflict do nothing`

func buildInsertEventsParamsLookups(event *mocrelay.Event, eventKey string) [][]any {
	var ret [][]any

	ret = append(ret, []any{
		event.ID,
		lookupWhatID,
		event.CreatedAt,
		eventKey,
	})

	ret = append(ret, []any{
		event.Pubkey,
		lookupWhatPubkey,
		event.CreatedAt,
		eventKey,
	})

	ret = append(ret, []any{
		strconv.FormatInt(event.Kind, 10),
		lookupWhatKind,
		event.CreatedAt,
		eventKey,
	})

	for _, tag := range event.Tags {
		if len(tag) == 0 {
			continue
		}
		if len(tag[0]) != 1 {
			continue
		}
		if !('a' <= tag[0][0] && tag[0][0] <= 'z' || 'A' <= tag[0][0] && tag[0][0] <= 'Z') {
			continue
		}

		var value string
		if len(tag) > 1 {
			value = tag[1]
		}

		ret = append(ret, []any{
			value,
			tag[0],
			event.CreatedAt,
			eventKey,
		})
	}

	return ret
}

const insertDeletedEventsQuery = `insert into deleted_events (event_key_or_id) values (?) on conflict do nothing`

func buildInsertEventsParamsDeletedEvents(event *mocrelay.Event) [][]any {
	if event.Kind != 5 {
		return nil
	}

	var ret [][]any

	for _, tag := range event.Tags {
		if len(tag) != 2 {
			continue
		}
		if tag[0] != "a" && tag[0] != "e" {
			continue
		}

		ret = append(ret, []any{tag[1], event.Pubkey})
	}

	return ret
}

func getEventKey(event *mocrelay.Event) string {
	switch event.EventType() {
	case mocrelay.EventTypeRegular:
		return event.ID

	case mocrelay.EventTypeReplaceable:
		return fmt.Sprintf("%d:%s", event.Kind, event.Pubkey)

	case mocrelay.EventTypeParamReplaceable:
		idx := slices.IndexFunc(event.Tags, func(t mocrelay.Tag) bool {
			return len(t) >= 1 && t[0] == "d"
		})
		if idx < 0 {
			return ""
		}

		d := ""
		if len(event.Tags[idx]) > 1 {
			d = event.Tags[idx][1]
		}

		return fmt.Sprintf("%d:%s:%s", event.Kind, event.Pubkey, d)

	default:
		return ""
	}
}
