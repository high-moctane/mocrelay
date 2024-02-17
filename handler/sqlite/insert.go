package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/high-moctane/mocrelay"
	"github.com/pierrec/xxHash/xxHash32"
)

func insertEvents(
	ctx context.Context,
	db *sql.DB,
	seed uint32,
	events []*mocrelay.Event,
) (err error) {
	// Delete
	eventKeysCreatedAt, err := getMayDeleteEventKeysCreatedAt(seed, events)

	needDeleteEventKeysQuery, err := buildNeedDeleteEventKeysQuery(seed, eventKeysCreatedAt)
	if err != nil {
		return fmt.Errorf("failed to build need delete event keys query: %w", err)
	}

	params := buildInsertEventsParams(seed, events)
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

	// Delete
	var needDeleteEventKeysStmt *sql.Stmt
	if needDeleteEventKeysQuery != "" {
		needDeleteEventKeysStmt, err = tx.PrepareContext(ctx, needDeleteEventKeysQuery)
		if err != nil {
			return fmt.Errorf("failed to prepare need delete event keys statement: %w", err)
		}
	}

	// Insert
	eventsStmt, err := tx.PrepareContext(ctx, insertEventsQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare events statement: %w", err)
	}
	defer eventsStmt.Close()

	eventPayloadsStmt, err := tx.PrepareContext(ctx, insertEventPayloadsQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare event payloads statement: %w", err)
	}
	defer eventPayloadsStmt.Close()

	tagsStmt, err := tx.PrepareContext(ctx, insertTagsQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare tags statement: %w", err)
	}
	defer tagsStmt.Close()

	deletedEventKeysStmt, err := tx.PrepareContext(ctx, insertDeletedEventKeysQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare deleted events statement: %w", err)
	}
	defer deletedEventKeysStmt.Close()

	deletedEventIDsStmt, err := tx.PrepareContext(ctx, insertDeletedEventIDsQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare deleted event ids statement: %w", err)
	}
	defer deletedEventIDsStmt.Close()

	lookupHashesStmt, err := tx.PrepareContext(ctx, insertLookupHashesQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare lookup hashes statement: %w", err)
	}
	defer lookupHashesStmt.Close()

	// Delete
	if needDeleteEventKeysStmt != nil {
		rows, err := needDeleteEventKeysStmt.QueryContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to query need delete event keys: %w", err)
		}
		var eventKeys []int64
		err = func() error {
			defer rows.Close()

			for rows.Next() {
				var eventKey int64
				if err := rows.Scan(&eventKey); err != nil {
					return fmt.Errorf("failed to scan event key: %w", err)
				}
				eventKeys = append(eventKeys, eventKey)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("failed to iterate need delete event keys: %w", err)
			}

			return nil
		}()
		if err != nil {
			return err
		}

		if len(eventKeys) > 0 {
			deleteEvents, err := buildDeleteEvents(seed, eventKeys)
			if err != nil {
				return fmt.Errorf("failed to build delete events: %w", err)
			}

			for _, q := range deleteEvents {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return fmt.Errorf("failed to delete events: %w", err)
				}
			}
		}
	}

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

		if _, err := eventPayloadsStmt.ExecContext(ctx, p.EventPayloads...); err != nil {
			return fmt.Errorf("failed to insert event payloads: %w", err)
		}

		for _, tag := range p.Tags {
			if _, err := tagsStmt.ExecContext(ctx, tag...); err != nil {
				return fmt.Errorf("failed to insert tags: %w", err)
			}
		}

		for _, deletedEvent := range p.DeletedEventKeys {
			if _, err := deletedEventKeysStmt.ExecContext(ctx, deletedEvent...); err != nil {
				return fmt.Errorf("failed to insert deleted events: %w", err)
			}
		}

		for _, deletedEvent := range p.DeletedEventIDs {
			if _, err := deletedEventIDsStmt.ExecContext(ctx, deletedEvent...); err != nil {
				return fmt.Errorf("failed to insert deleted event ids: %w", err)
			}
		}

		for _, lookup := range p.LookupHashes {
			if _, err := lookupHashesStmt.ExecContext(ctx, lookup...); err != nil {
				return fmt.Errorf("failed to insert lookup hashes: %w", err)
			}
		}
	}

	return
}

func getMayDeleteEventKeysCreatedAt(
	seed uint32,
	events []*mocrelay.Event,
) (map[int64]int64, error) {
	ret := make(map[int64]int64)

	for _, event := range events {
		switch event.EventType() {
		case mocrelay.EventTypeReplaceable, mocrelay.EventTypeParamReplaceable:
			eventKey, ok := getEventKey(seed, event)
			if !ok {
				continue
			}

			if createdAt, ok := ret[eventKey]; !ok || event.CreatedAt < createdAt {
				ret[eventKey] = event.CreatedAt
			}
		}
	}

	return ret, nil
}

func buildNeedDeleteEventKeysQuery(seed uint32, eventKeys map[int64]int64) (string, error) {
	if len(eventKeys) == 0 {
		return "", nil
	}

	var ands []goqu.Expression
	for eventKey, createdAt := range eventKeys {
		ands = append(ands, goqu.And(
			goqu.C("event_key").Eq(eventKey),
			goqu.C("created_at").Lt(createdAt),
		))
	}

	q, _, err := goqu.
		Dialect("sqlite3").
		Select("event_key").
		From("events").
		Where(goqu.Or(ands...)).
		ToSQL()
	if err != nil {
		return "", fmt.Errorf("failed to build need delete event keys query: %w", err)
	}

	return q, nil
}

func buildDeleteEvents(seed uint32, eventKeys []int64) ([]string, error) {
	var ret []string

	q, _, err := goqu.
		Dialect("sqlite3").
		Delete("event_payloads").
		Where(goqu.C("event_key").In(eventKeys)).
		ToSQL()
	if err != nil {
		return nil, fmt.Errorf("failed to build delete event payloads query: %w", err)
	}
	ret = append(ret, q)

	q, _, err = goqu.
		Dialect("sqlite3").
		Delete("event_tags").
		Where(goqu.C("event_key").In(eventKeys)).
		ToSQL()
	if err != nil {
		return nil, fmt.Errorf("failed to build delete event tags query: %w", err)
	}
	ret = append(ret, q)

	q, _, err = goqu.
		Dialect("sqlite3").
		Delete("lookup_hashes").
		Where(goqu.C("event_key").In(eventKeys)).
		ToSQL()
	if err != nil {
		return nil, fmt.Errorf("failed to build delete lookup hashes query: %w", err)
	}
	ret = append(ret, q)

	q, _, err = goqu.
		Dialect("sqlite3").
		Delete("events").
		Where(goqu.C("event_key").In(eventKeys)).
		ToSQL()
	if err != nil {
		return nil, fmt.Errorf("failed to build delete events query: %w", err)
	}
	ret = append(ret, q)

	return ret, nil
}

type insertEventsParams struct {
	Events           []any
	EventPayloads    []any
	Tags             [][]any
	DeletedEventKeys [][]any
	DeletedEventIDs  [][]any
	LookupHashes     [][]any
}

var emptyTagsBytes = []byte("[]")

func buildInsertEventsParams(seed uint32, events []*mocrelay.Event) []insertEventsParams {
	ret := make([]insertEventsParams, 0, len(events))

	for _, event := range events {
		eventKey, ok := getEventKey(seed, event)
		if !ok {
			continue
		}

		events, err := buildInsertEventsParamsEvent(seed, event, eventKey)
		if err != nil {
			continue
		}

		eventPayloads, err := buildInsertEventsParamsEventPayloads(seed, event, eventKey)
		if err != nil {
			continue
		}

		deletedEventKeys, err := buildInsertEventsParamsDeletedEventKeys(seed, event)
		if err != nil {
			continue
		}

		deleteEventIDs, err := buildInsertEventsParamsDeletedEventIDs(seed, event)
		if err != nil {
			continue
		}

		ret = append(ret, insertEventsParams{
			Events:           events,
			EventPayloads:    eventPayloads,
			Tags:             buildInsertEventsParamsTags(seed, event, eventKey),
			DeletedEventKeys: deletedEventKeys,
			DeletedEventIDs:  deleteEventIDs,
			LookupHashes:     buildInsertLookupHashesParams(seed, event, eventKey),
		})
	}

	return ret
}

const insertEventsQuery = `
insert into events (
	event_key,
	id,
	pubkey,
	created_at,
	kind
) values
	(?, ?, ?, ?, ?)
on conflict(event_key) do nothing
`

func buildInsertEventsParamsEvent(
	seed uint32,
	event *mocrelay.Event,
	eventKey int64,
) ([]any, error) {
	idBin, err := hex.DecodeString(event.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to decode id: %w", err)
	}

	pubkeyBin, err := hex.DecodeString(event.Pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pubkey: %w", err)
	}

	return []any{
		eventKey,
		idBin,
		pubkeyBin,
		event.CreatedAt,
		event.Kind,
	}, nil
}

const insertEventPayloadsQuery = `
insert into event_payloads (
	event_key,
	tags,
	content,
	sig
) values
	(?, ?, ?, ?)
`

func buildInsertEventsParamsEventPayloads(
	seed uint32,
	event *mocrelay.Event,
	eventKey int64,
) ([]any, error) {
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

	sigBin, err := hex.DecodeString(event.Sig)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sig: %w", err)
	}

	return []any{
		eventKey,
		tagsBytes,
		event.Content,
		sigBin,
	}, nil
}

const insertTagsQuery = `
insert into event_tags (
	event_key,
	key,
	value
) values
	(?, ?, ?)
`

func buildInsertEventsParamsTags(seed uint32, event *mocrelay.Event, eventKey int64) [][]any {
	var ret [][]any

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
			eventKey,
			[]byte(tag[0]),
			[]byte(value),
		})
	}

	return ret
}

const insertDeletedEventKeysQuery = `
insert into deleted_event_keys (
	event_key,
	pubkey
)
values
	(?, ?)
on conflict(event_key, pubkey) do nothing
`

func buildInsertEventsParamsDeletedEventKeys(seed uint32, event *mocrelay.Event) ([][]any, error) {
	if event.Kind != 5 {
		return nil, nil
	}

	var ret [][]any

	pubkeyBin, err := hex.DecodeString(event.Pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pubkey: %w", err)
	}

	for _, tag := range event.Tags {
		if len(tag) != 2 {
			continue
		}
		if tag[0] != "a" {
			continue
		}

		elems := strings.Split(tag[1], ":")
		if len(elems) < 2 {
			continue
		}

		x := xxHash32.New(seed)
		io.WriteString(x, elems[1])
		pubkeyHash := x.Sum32()

		x.Reset()
		io.WriteString(x, tag[1])
		aHash := x.Sum32()

		eventKey := int64(pubkeyHash)<<32 | int64(aHash)

		ret = append(ret, []any{
			eventKey,
			pubkeyBin,
		})
	}

	return ret, nil
}

const insertDeletedEventIDsQuery = `
insert into deleted_event_ids (
	id,
	pubkey
)
values
	(?, ?)
on conflict(id, pubkey) do nothing
`

func buildInsertEventsParamsDeletedEventIDs(seed uint32, event *mocrelay.Event) ([][]any, error) {
	if event.Kind != 5 {
		return nil, nil
	}

	var ret [][]any

	pubkeyBin, err := hex.DecodeString(event.Pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pubkey: %w", err)
	}

	for _, tag := range event.Tags {
		if len(tag) != 2 {
			continue
		}
		if tag[0] != "e" {
			continue
		}

		idBin, err := hex.DecodeString(tag[1])
		if err != nil {
			continue
		}

		ret = append(ret, []any{
			idBin,
			pubkeyBin,
		})
	}

	return ret, nil
}

const insertLookupHashesQuery = `
insert into lookup_hashes (
	hash,
	created_at,
	event_key
)
values
	(?, ?, ?)
`

func buildInsertLookupHashesParams(seed uint32, event *mocrelay.Event, eventKey int64) [][]any {
	hashes := createLookupHashesFromEvent(seed, event)
	if len(hashes) == 0 {
		return nil
	}

	type triplet struct {
		hash      int64
		createdAt int64
		eventKey  int64
	}

	seen := make(map[triplet]bool)

	var ret [][]any
	for _, hash := range hashes {
		if seen[triplet{hash, event.CreatedAt, eventKey}] {
			continue
		}
		seen[triplet{hash, event.CreatedAt, eventKey}] = true

		ret = append(ret, []any{
			hash,
			event.CreatedAt,
			eventKey,
		})
	}

	return ret
}

func getEventKey(seed uint32, event *mocrelay.Event) (int64, bool) {
	switch event.EventType() {
	case mocrelay.EventTypeRegular:
		ts := uint64(uint32(event.CreatedAt))
		x := xxHash32.New(seed)
		io.WriteString(x, event.ID)
		idHash := x.Sum32()
		return int64(ts<<32 | uint64(idHash)), true

	case mocrelay.EventTypeReplaceable:
		x := xxHash32.New(seed)
		io.WriteString(x, event.Pubkey)
		pubkeyHash := x.Sum32()
		a := fmt.Sprintf("%d:%s", event.Kind, event.Pubkey)
		x.Reset()
		io.WriteString(x, a)
		aHash := x.Sum32()
		return int64(pubkeyHash)<<32 | int64(aHash), true

	case mocrelay.EventTypeParamReplaceable:
		idx := slices.IndexFunc(event.Tags, func(t mocrelay.Tag) bool {
			return len(t) >= 1 && t[0] == "d"
		})
		if idx < 0 {
			return 0, false
		}

		d := ""
		if len(event.Tags[idx]) > 1 {
			d = event.Tags[idx][1]
		}

		x := xxHash32.New(seed)
		io.WriteString(x, event.Pubkey)
		pubkeyHash := x.Sum32()
		a := fmt.Sprintf("%d:%s:%s", event.Kind, event.Pubkey, d)
		x.Reset()
		io.WriteString(x, a)
		aHash := x.Sum32()
		return int64(pubkeyHash)<<32 | int64(aHash), true

	default:
		return 0, false
	}
}
