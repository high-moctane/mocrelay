package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/high-moctane/mocrelay"
	"github.com/pierrec/xxHash/xxHash32"
)

func queryEvent(
	ctx context.Context,
	db *sql.DB,
	seed uint32,
	fs []*mocrelay.ReqFilter,
	maxLimit uint,
) (events []*mocrelay.Event, err error) {
	q, param, err := buildEventQuery(fs, seed, maxLimit)
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
	seed uint32,
	maxLimit uint,
) (query string, param []any, err error) {
	sqlite3 := goqu.Dialect("sqlite3")

	e := goqu.T("events")
	eEventKeyHash := e.Col("event_key_hash")
	eEventKey := e.Col("event_key")
	eIDHash := e.Col("id_hash")
	eID := e.Col("id")
	ePubkey := e.Col("pubkey")
	ePubkeyHash := e.Col("pubkey_hash")
	eCreatedAt := e.Col("created_at")
	eKind := e.Col("kind")

	p := goqu.T("event_payloads")
	pEventKeyHash := p.Col("event_key_hash")
	pEventKey := p.Col("event_key")
	pTags := p.Col("tags")
	pContent := p.Col("content")
	pSig := p.Col("sig")

	t := goqu.T("event_tags")
	tKeyValueHash := t.Col("key_value_hash")
	tKey := t.Col("key")
	tValue := t.Col("value")
	tCreatedAt := t.Col("created_at")
	tEventKeyHash := t.Col("event_key_hash")
	tEventKey := t.Col("event_key")

	d := goqu.T("deleted_events")
	dEventKeyOrIDHash := d.Col("event_key_or_id_hash")
	dEventKeyOrID := d.Col("event_key_or_id")
	dPubkey := d.Col("pubkey")

	var builder *goqu.SelectDataset

	x := xxHash32.New(seed)

	for _, f := range fs {
		b := sqlite3.
			Select(
				eID,
				ePubkey,
				eCreatedAt,
				eKind,
				pTags,
				pContent,
				pSig,
			).
			From(e).
			Join(p, goqu.On(
				eEventKeyHash.Eq(pEventKeyHash),
				eEventKey.Eq(pEventKey),
			)).
			Order(eCreatedAt.Desc())

		b = b.Where(goqu.L("not exists ?",
			sqlite3.
				Select(goqu.L("1")).
				From(d).
				Where(goqu.Or(
					goqu.And(
						dEventKeyOrIDHash.Eq(eEventKeyHash),
						dEventKeyOrID.Eq(eEventKey),
					),
					goqu.And(
						dEventKeyOrIDHash.Eq(eIDHash),
						dEventKeyOrID.Eq(eID),
					),
				)).
				Where(dPubkey.Eq(ePubkey)),
		))

		if f.IDs != nil {
			idHashes := make([]string, len(f.IDs))
			for i, id := range f.IDs {
				x.Reset()
				io.WriteString(x, id)
				idHashes[i] = fmt.Sprint(x.Sum32())
			}

			b = b.Where(eIDHash.In(idHashes))
			b = b.Where(eID.In(f.IDs))
		}

		if f.Authors != nil {
			authorHashes := make([]string, len(f.Authors))
			for i, pubkey := range f.Authors {
				x.Reset()
				io.WriteString(x, pubkey)
				authorHashes[i] = fmt.Sprint(x.Sum32())
			}

			b = b.Where(ePubkeyHash.In(authorHashes))
			b = b.Where(ePubkey.In(f.Authors))
		}

		if f.Kinds != nil {
			b = b.Where(eKind.In(f.Kinds))
		}

		if f.Tags != nil {
			for key, values := range f.Tags {
				k := key[1:]
				keyValueHashes := make([]string, len(values))
				for i, value := range values {
					x.Reset()
					io.WriteString(x, k)
					io.WriteString(x, value)
					keyValueHashes[i] = fmt.Sprint(x.Sum32())
				}

				b = b.Where(goqu.L("exists ?",
					sqlite3.
						Select(goqu.L("1")).
						From("event_tags").
						Where(tKeyValueHash.In(keyValueHashes)).
						Where(tKey.Eq(k)).
						Where(tValue.In(values)).
						Where(tCreatedAt.Eq(eCreatedAt)).
						Where(tEventKeyHash.Eq(eEventKeyHash)).
						Where(tEventKey.Eq(eEventKey)),
				))
			}
		}

		if f.Since != nil {
			b = b.Where(eCreatedAt.Gte(f.Since))
		}

		if f.Until != nil {
			b = b.Where(eCreatedAt.Lte(f.Until))
		}

		limit := maxLimit
		if f.Limit != nil {
			limit = min(limit, uint(*f.Limit))
		}
		if limit != NoLimit {
			b = b.Limit(limit)
		}

		if builder == nil {
			builder = b
		} else {
			builder = builder.Union(b)
		}
	}

	builder = builder.
		Order(goqu.C("created_at").Desc())

	if maxLimit != NoLimit {
		builder = builder.Limit(maxLimit)
	}

	return builder.ToSQL()
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
