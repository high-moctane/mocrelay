package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/high-moctane/mocrelay"
)

func queryEvent(
	ctx context.Context,
	db *sql.DB,
	f *mocrelay.ReqFilter,
) (events []*mocrelay.Event, err error) {
	q, param, err := buildEventQuery(f)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	events, err = fetchEventQuery(ctx, db, q, param)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events: %w", err)
	}

	return
}

func buildEventQuery(f *mocrelay.ReqFilter) (query string, param []any, err error) {
	e := goqu.T("events")
	b := goqu.Dialect("sqlite3").Select(
		e.Col("id"),
		e.Col("pubkey"),
		e.Col("created_at"),
		e.Col("kind"),
		e.Col("tags"),
		e.Col("content"),
		e.Col("sig"),
	).From("events").Order(goqu.I("events.created_at").Desc())

	if f != nil {
		if f.IDs != nil {
			b = b.Where(e.Col("id").In(f.IDs))
		}

		if f.Authors != nil {
			b = b.Where(e.Col("pubkey").In(f.Authors))
		}

		if f.Kinds != nil {
			b = b.Where(e.Col("kind").In(f.Kinds))
		}

		if f.Tags != nil {
			for tag, values := range f.Tags {
				tname := fmt.Sprintf("tag%s", tag)
				b = b.Join(goqu.L(fmt.Sprintf("json_each(events.tags) as %s", tname)), goqu.On(
					goqu.And(
						goqu.L(fmt.Sprintf("%s.value->>0", tname)).Eq(tag),
						goqu.L(fmt.Sprintf("ifnull(%s.value->>1, '')", tname)).In(values),
					),
				))
			}
		}

		if f.Since != nil {
			b = b.Where(e.Col("created_at").Gte(*f.Since))
		}

		if f.Until != nil {
			b = b.Where(e.Col("created_at").Lte(*f.Until))
		}

		if f.Limit != nil {
			b = b.Limit(uint(*f.Limit))
		}
	}

	return b.ToSQL()
}

func fetchEventQuery(
	ctx context.Context,
	db *sql.DB,
	query string,
	param []any,
) (events []*mocrelay.Event, err error) {
	rows, err := db.QueryContext(ctx, query, param...)
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
