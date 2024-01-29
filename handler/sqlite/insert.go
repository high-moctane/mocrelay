package sqlite

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"text/template"

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
) (affected int64, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
			return
		}
		err = tx.Commit()
	}()

	// kind5
	for _, event := range events {
		var kind5s []*mocrelay.Event
		if event.Kind == 5 {
			kind5s = append(kind5s, event)
		}
		if len(kind5s) > 0 {
			if _, err := insertDeletedKeys(ctx, tx, kind5s); err != nil {
				return 0, fmt.Errorf("failed to insert deleted key: %w", err)
			}
		}
	}

	// events
	query, param, err := buildInsertEvents(ctx, seed, events)
	if err != nil {
		if errors.Is(err, errNoEventToInsert) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	res, err := tx.ExecContext(ctx, query, param...)
	if err != nil {
		return 0, fmt.Errorf("failed to insert event: %w", err)
	}

	affected, err = res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	// hashes
	query, param, err = buildInsertHashes(seed, events)
	if err != nil {
		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	if _, err := tx.ExecContext(ctx, query, param...); err != nil {
		return 0, fmt.Errorf("failed to insert hash: %w", err)
	}

	return
}

var insertEventsTemplate = template.Must(template.New("insertEvent").Parse(`
insert into events (
	key, id, pubkey, created_at, kind, tags, content, sig, hashed_id
) values
{{- range $i, $event := .}}{{if ne $i 0}},{{end}}
	(?, ?, ?, ?, ?, ?, ?, ?, ?)
{{- end}}
on conflict(key) do update set
	id         = excluded.id,
	pubkey     = excluded.pubkey,
	created_at = excluded.created_at,
	kind       = excluded.kind,
	tags       = excluded.tags,
	content    = excluded.content,
	sig        = excluded.sig,
	hashed_id  = excluded.hashed_id
where
	events.id != excluded.id
	and
	(
		events.kind = 0
		or
		events.kind = 3
		or
		(10000 <= events.kind and events.kind < 20000)
		or
		(30000 <= events.kind and events.kind < 40000)
	)
	and
	events.created_at < excluded.created_at
`))

var errNoEventToInsert = fmt.Errorf("no event to insert")

func buildInsertEvents(
	ctx context.Context,
	seed uint32,
	events []*mocrelay.Event,
) (query string, param []any, err error) {
	type entry struct {
		Key       string
		ID        string
		Pubkey    string
		CreatedAt int64
		Kind      int64
		Tags      []byte
		Content   string
		Sig       string
		HashedID  uint32
	}

	entries := make([]entry, 0, len(events))

	for _, event := range events {
		key := getEventKey(event)
		if key == "" {
			continue
		}

		var tagBytes []byte
		if len(event.Tags) == 0 {
			tagBytes = []byte("[]")
		} else {
			tagBytes, err = json.Marshal(event.Tags)
			if err != nil {
				return "", nil, fmt.Errorf("failed to marshal tags: %w", err)
			}
		}

		x := xxHash32.New(seed)
		x.Write([]byte(event.ID))
		hashedID := x.Sum32()

		entries = append(entries, entry{
			Key:       key,
			ID:        event.ID,
			Pubkey:    event.Pubkey,
			CreatedAt: event.CreatedAt,
			Kind:      event.Kind,
			Tags:      tagBytes,
			Content:   event.Content,
			Sig:       event.Sig,
			HashedID:  hashedID,
		})
	}

	if len(entries) == 0 {
		return "", nil, errNoEventToInsert
	}

	var b bytes.Buffer
	if err = insertEventsTemplate.Execute(&b, entries); err != nil {
		return "", nil, fmt.Errorf("failed to execute template: %w", err)
	}
	query = b.String()

	for _, e := range entries {
		param = append(param, e.Key)
		param = append(param, e.ID)
		param = append(param, e.Pubkey)
		param = append(param, e.CreatedAt)
		param = append(param, e.Kind)
		param = append(param, e.Tags)
		param = append(param, e.Content)
		param = append(param, e.Sig)
		param = append(param, e.HashedID)
	}

	return
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

func insertDeletedKeys(
	ctx context.Context,
	tx *sql.Tx,
	kind5s []*mocrelay.Event,
) (affected int64, err error) {
	query, param, err := buildInsertDeletedKeys(ctx, kind5s)
	if err != nil {
		if errors.Is(err, errNoKeyToDelete) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	res, err := tx.ExecContext(ctx, query, param...)
	if err != nil {
		return 0, fmt.Errorf("failed to insert deleted key: %w", err)
	}

	affected, err = res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return
}

// map[tag][]key
func gatherDeletedKeys(kind5 *mocrelay.Event) map[string][]string {
	ret := make(map[string][]string)

	for _, tag := range kind5.Tags {
		if len(tag) == 0 {
			continue
		}
		if tag[0] == "e" || tag[0] == "a" {
			var value string
			if len(tag) > 1 {
				value = tag[1]
			}
			ret[tag[0]] = append(ret[tag[0]], value)
		}
	}

	return ret
}

var errNoKeyToDelete = fmt.Errorf("no key to delete")

func buildInsertDeletedKeys(
	ctx context.Context,
	kind5s []*mocrelay.Event,
) (query string, param []any, err error) {
	var records []goqu.Record

	for _, kind5 := range kind5s {
		for tag, key := range gatherDeletedKeys(kind5) {
			for _, k := range key {
				records = append(
					records,
					goqu.Record{"value": k, "tag": tag, "pubkey": kind5.Pubkey},
				)
			}
		}
	}

	b := goqu.Dialect("sqlite3").Insert("deleted_keys").Rows(records).OnConflict(goqu.DoNothing())

	return b.ToSQL()
}

var insertHashesTemplate = template.Must(template.New("insertEvent").Parse(`
insert into hashes (
	hashed_id, hashed_value, created_at
)
{{- range $i, $record := .}}
{{ if ne $i 0}}union{{end}}
select ?, ?, ? where ? in (select hashed_id from events)
{{- end}}
on conflict do nothing
`))

func buildInsertHashes(
	seed uint32,
	events []*mocrelay.Event,
) (query string, param []any, err error) {
	records := gatherEventHashes(seed, events)

	var b bytes.Buffer
	if err = insertHashesTemplate.Execute(&b, records); err != nil {
		return "", nil, fmt.Errorf("failed to execute template: %w", err)
	}
	query = b.String()

	for _, r := range records {
		param = append(param, r["hashed_id"])
		param = append(param, r["hashed_value"])
		param = append(param, r["created_at"])
		param = append(param, r["hashed_id"])
	}

	return
}

func gatherEventHashes(seed uint32, events []*mocrelay.Event) []goqu.Record {
	var ret []goqu.Record

	x := xxHash32.New(seed)

	for _, event := range events {
		x.Reset()
		x.Write([]byte(event.ID))
		hashedID := x.Sum32()
		ret = append(
			ret,
			goqu.Record{
				"hashed_id":    hashedID,
				"hashed_value": hashedID,
				"created_at":   event.CreatedAt,
			},
		)

		x.Reset()
		x.Write([]byte(event.Pubkey))
		ret = append(
			ret,
			goqu.Record{
				"hashed_id":    hashedID,
				"hashed_value": x.Sum32(),
				"created_at":   event.CreatedAt,
			},
		)

		x.Reset()
		x.Write([]byte(strconv.FormatInt(event.Kind, 10)))
		ret = append(
			ret,
			goqu.Record{
				"hashed_id":    hashedID,
				"hashed_value": x.Sum32(),
				"created_at":   event.CreatedAt,
			},
		)

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

			x.Reset()
			x.Write([]byte(tag[0] + value))
			ret = append(
				ret,
				goqu.Record{
					"hashed_id":    hashedID,
					"hashed_value": x.Sum32(),
					"created_at":   event.CreatedAt,
				},
			)
		}
	}

	return ret
}

func getOrSetSeed(ctx context.Context, db *sql.DB) (seed uint32, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
			return
		}
		err = tx.Commit()
	}()

	if err := tx.QueryRowContext(ctx, "select seed from hash_seed").Scan(&seed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			n, err := rand.Int(rand.Reader, big.NewInt(math.MaxUint32))
			if err != nil {
				return 0, fmt.Errorf("failed to generate random number: %w", err)
			}
			seed = uint32(n.Int64())

			if _, err := tx.ExecContext(ctx, "insert into hash_seed (seed) values (?)", seed); err != nil {
				return 0, fmt.Errorf("failed to insert seed: %w", err)
			}

		} else {
			return 0, fmt.Errorf("failed to query seed: %w", err)
		}
	}

	return
}
