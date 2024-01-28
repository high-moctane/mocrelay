package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"text/template"

	"github.com/high-moctane/mocrelay"
)

func insertEvents(
	ctx context.Context,
	db *sql.DB,
	events []*mocrelay.Event,
) (affected int64, err error) {
	query, param, err := buildInsertEvents(ctx, events)
	if err != nil {
		if errors.Is(err, errNoEventToInsert) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to build query: %w", err)
	}

	res, err := db.ExecContext(ctx, query, param...)
	if err != nil {
		return 0, fmt.Errorf("failed to insert event: %w", err)
	}

	affected, err = res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return
}

var insertEventsTemplate = template.Must(template.New("insertEvent").Parse(`
insert into events (
	key, id, pubkey, created_at, kind, tags, content, sig
) values
{{- range $i, $event := .}}{{if ne $i 0}},{{end}}
	(?, ?, ?, ?, ?, ?, ?, ?)
{{- end}}
on conflict(key) do update set
	id = excluded.id,
	pubkey = excluded.pubkey,
	created_at = excluded.created_at,
	kind = excluded.kind,
	tags = excluded.tags,
	content = excluded.content,
	sig = excluded.sig
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
	}

	entries := make([]entry, 0, len(events))

	for _, event := range events {
		key := getEventKey(event)
		if key == "" {
			continue
		}

		var tagBytes []byte
		tagBytes, err = json.Marshal(event.Tags)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal tags: %w", err)
		}

		entries = append(entries, entry{
			Key:       key,
			ID:        event.ID,
			Pubkey:    event.Pubkey,
			CreatedAt: event.CreatedAt,
			Kind:      event.Kind,
			Tags:      tagBytes,
			Content:   event.Content,
			Sig:       event.Sig,
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
	}

	return
}

func getEventKey(event *mocrelay.Event) string {
	switch event.EventType() {
	case mocrelay.EventTypeRegular:
		return event.ID

	case mocrelay.EventTypeReplaceable:
		return fmt.Sprintf("%s:%d", event.Pubkey, event.Kind)

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

		return fmt.Sprintf("%s:%d:%s", event.Pubkey, event.Kind, d)

	default:
		return ""
	}
}
