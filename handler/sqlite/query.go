package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/high-moctane/mocrelay"
)

func queryEvent(
	ctx context.Context,
	db *sql.DB,
	fs []*mocrelay.ReqFilter,
	maxLimit uint,
) (events []*mocrelay.Event, err error) {
	q, param, err := buildEventQuery(fs, maxLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	events, err = fetchEventQuery(ctx, db, q, param)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events with (%s, %v): %w", q, param, err)
	}

	return
}

func buildEventQuery(
	fs []*mocrelay.ReqFilter,
	maxLimit uint,
) (query string, param []any, err error) {
	sqlite3 := goqu.Dialect("sqlite3")

	e := goqu.T("events")
	eRecordID := e.Col("record_id")
	eEventKey := e.Col("event_key")
	eID := e.Col("id")
	ePubkey := e.Col("pubkey")
	eCreatedAt := e.Col("created_at")
	eKind := e.Col("kind")
	eTags := e.Col("tags")
	eContent := e.Col("content")
	eSig := e.Col("sig")

	l := goqu.T("lookups")
	lCreatedAt := l.Col("created_at")
	lEventKey := l.Col("event_key")

	d := goqu.T("deleted_events")
	dEventKeyOrID := d.Col("event_key_or_id")
	dPubkey := d.Col("pubkey")

	filterQueries := make([]*goqu.SelectDataset, len(fs))

	for i, f := range fs {
		var b *goqu.SelectDataset

		if needJoinLookup(f) {
			b = sqlite3.
				From(l).
				Distinct().
				Join(e, goqu.On(lEventKey.Eq(eEventKey))).
				Order(lCreatedAt.Desc())

			if f.IDs != nil {
				t := goqu.T(fmt.Sprintf("lookups_ids_%d", i))
				b = b.Join(l.As(t), goqu.On(
					lEventKey.Eq(t.Col("event_key")),
					lCreatedAt.Eq(t.Col("created_at")),
					t.Col("value").In(f.IDs),
					t.Col("what").Eq(lookupWhatID),
				))
			}

			if f.Authors != nil {
				t := goqu.T(fmt.Sprintf("lookups_authors_%d", i))
				b = b.Join(l.As(t), goqu.On(
					lEventKey.Eq(t.Col("event_key")),
					lCreatedAt.Eq(t.Col("created_at")),
					t.Col("value").In(f.Authors),
					t.Col("what").Eq(lookupWhatPubkey),
				))
			}

			if f.Kinds != nil {
				t := goqu.T(fmt.Sprintf("lookups_kinds_%d", i))

				kindsStr := make([]string, len(f.Kinds))
				for i, kind := range f.Kinds {
					kindsStr[i] = strconv.FormatInt(kind, 10)
				}

				b = b.Join(l.As(t), goqu.On(
					lEventKey.Eq(t.Col("event_key")),
					lCreatedAt.Eq(t.Col("created_at")),
					t.Col("value").In(f.Kinds),
					t.Col("what").Eq(lookupWhatKind),
				))
			}

			if f.Tags != nil {
				for k, vs := range f.Tags {
					tagName := k[1:2]
					t := goqu.T(fmt.Sprintf("lookups_tags_%d_%s", i, tagName))

					b = b.Join(l.As(t), goqu.On(
						lEventKey.Eq(t.Col("event_key")),
						lCreatedAt.Eq(t.Col("created_at")),
						t.Col("value").In(vs),
						t.Col("what").Eq(tagName),
					))
				}
			}

			if f.Since != nil {
				b = b.Where(lCreatedAt.Gte(f.Since))
			}

			if f.Until != nil {
				b = b.Where(lCreatedAt.Lte(f.Until))
			}
		} else {
			b = sqlite3.
				From(e).
				Order(eCreatedAt.Desc())

			if f.Since != nil {
				b = b.Where(eCreatedAt.Gte(f.Since))
			}

			if f.Until != nil {
				b = b.Where(eCreatedAt.Lte(f.Until))
			}
		}

		b = b.Where(goqu.L("not exists ?",
			sqlite3.
				Select(goqu.L("1")).
				From(d).
				Where(goqu.Or(
					goqu.And(
						dEventKeyOrID.Eq(eEventKey),
						dPubkey.Eq(ePubkey),
					),
					goqu.And(
						dEventKeyOrID.Eq(eID),
						dPubkey.Eq(ePubkey),
					),
				)),
		))

		limit := maxLimit
		if f.Limit != nil {
			limit = min(limit, uint(*f.Limit))
		}
		if limit != NoLimit {
			b = b.Limit(limit)
		}

		filterQueries[i] = b
	}

	var builder *goqu.SelectDataset

	if len(filterQueries) == 1 {
		builder = filterQueries[0].
			Select(
				eID,
				ePubkey,
				eCreatedAt,
				eKind,
				eTags,
				eContent,
				eSig,
			)
	} else {
		var subquery *goqu.SelectDataset
		for i := range filterQueries {
			q := filterQueries[i].Select(eRecordID)
			if i == 0 {
				subquery = q
			} else {
				subquery = subquery.UnionAll(q)
			}
		}

		builder = sqlite3.
			Select(
				eID,
				ePubkey,
				eCreatedAt,
				eKind,
				eTags,
				eContent,
				eSig,
			).
			From(e).
			Where(eRecordID.In(subquery)).
			Order(eCreatedAt.Desc())

		if maxLimit != NoLimit {
			builder = builder.Limit(maxLimit)
		}
	}

	return builder.ToSQL()
}

func needJoinLookup(f *mocrelay.ReqFilter) bool {
	return f.IDs != nil || f.Authors != nil || f.Kinds != nil || f.Tags != nil
}

func fetchEventQuery(
	ctx context.Context,
	db *sql.DB,
	query string,
	param []any,
) (events []*mocrelay.Event, err error) {
	raws, err := fetchRawEvent(ctx, db, query, param)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch raw events: %w", err)
	}

	for _, raw := range raws {
		event, err := raw.toEvent()
		if err != nil {
			return nil, fmt.Errorf("failed to convert raw event to event: %w", err)
		}
		events = append(events, event)
	}

	return
}

func fetchRawEvent(
	ctx context.Context,
	db *sql.DB,
	query string,
	param []any,
) (raws []*rawEvent, err error) {
	rows, err := db.QueryContext(ctx, query, param...)
	if err != nil {
		return nil, fmt.Errorf("failed to query raw events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			raw rawEvent
		)
		if err := rows.Scan(
			&raw.ID,
			&raw.Pubkey,
			&raw.CreatedAt,
			&raw.Kind,
			&raw.Tags,
			&raw.Content,
			&raw.Sig,
		); err != nil {
			return nil, fmt.Errorf("failed to scan raw event: %w", err)
		}

		raws = append(raws, &raw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return
}

type rawEvent struct {
	ID        string
	Pubkey    string
	CreatedAt int64
	Kind      int64
	Tags      []byte
	Content   string
	Sig       string
}

func (r *rawEvent) toEvent() (*mocrelay.Event, error) {
	var tags []mocrelay.Tag
	if err := json.Unmarshal(r.Tags, &tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	return &mocrelay.Event{
		ID:        r.ID,
		Pubkey:    r.Pubkey,
		CreatedAt: r.CreatedAt,
		Kind:      r.Kind,
		Tags:      tags,
		Content:   r.Content,
		Sig:       r.Sig,
	}, nil
}
