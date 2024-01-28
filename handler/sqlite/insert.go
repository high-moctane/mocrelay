package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"

	"github.com/high-moctane/mocrelay"
)

func insertEvents(
	ctx context.Context,
	db *sql.DB,
	events []*mocrelay.Event,
) (affected int64, err error) {
	query, param, err := buildInsertEvents(ctx, events)
	if err != nil {
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
	id, pubkey, created_at, kind, tags, content, sig
) values
{{- range $i, $event := .}}{{if ne $i 0}},{{end}}
	(?, ?, ?, ?, ?, ?, ?)
{{- end}}
`))

func buildInsertEvents(
	ctx context.Context,
	events []*mocrelay.Event,
) (query string, param []any, err error) {
	if len(events) == 0 {
		return "", nil, fmt.Errorf("events is empty")
	}

	var b bytes.Buffer
	if err = insertEventsTemplate.Execute(&b, events); err != nil {
		return "", nil, fmt.Errorf("failed to execute template: %w", err)
	}
	query = b.String()

	for _, event := range events {
		tagBytes, err := json.Marshal(event.Tags)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal tags: %w", err)
		}

		param = append(param, event.ID)
		param = append(param, event.Pubkey)
		param = append(param, event.CreatedAt)
		param = append(param, event.Kind)
		param = append(param, tagBytes)
		param = append(param, event.Content)
		param = append(param, event.Sig)
	}

	return
}
