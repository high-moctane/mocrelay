package sqlite

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/doug-martin/goqu/v9/exp"

	"github.com/high-moctane/mocrelay"
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
	eEventKey := e.Col("event_key")
	eID := e.Col("id")
	ePubkey := e.Col("pubkey")
	eCreatedAt := e.Col("created_at")
	eKind := e.Col("kind")

	p := goqu.T("event_payloads")
	pEventKey := p.Col("event_key")
	pTags := p.Col("tags")
	pContent := p.Col("content")
	pSig := p.Col("sig")

	t := goqu.T("event_tags")

	builder := sqlite3.
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
		Join(p, goqu.On(eEventKey.Eq(pEventKey))).
		Order(eCreatedAt.Desc())
	builder = appendLimitQuery(builder, toPtr(int64(maxLimit)), maxLimit)

	var subs []exp.Expression

	for i, f := range fs {
		esub := e.As(fmt.Sprintf("esub%d", i))

		sub := sqlite3.
			Select(
				esub.Col("event_key"),
			).
			From(esub).
			Order(esub.Col("created_at").Desc())

		sub = appendDeletedEventsQuery(
			sub,
			esub.Col("event_key"),
			esub.Col("id"),
			esub.Col("pubkey"),
		)
		sub = appendSinceQuery(sub, esub.Col("created_at"), f.Since)
		sub = appendUntilQuery(sub, esub.Col("created_at"), f.Until)
		sub = appendLimitQuery(sub, f.Limit, maxLimit)

		if f.IDs != nil {
			idBins := make([][]byte, len(f.IDs))
			for i, id := range f.IDs {
				var err error
				idBins[i], err = hex.DecodeString(id)
				if err != nil {
					return "", nil, fmt.Errorf("failed to decode id: %w", err)
				}
			}

			eid := goqu.T("events").As("eid")

			sub = sub.
				Join(eid, goqu.On(
					esub.Col("event_key").Eq(eid.Col("event_key")),
					esub.Col("created_at").Eq(eid.Col("created_at")),
				)).
				Where(eid.Col("id").In(idBins))
		}

		if f.Authors != nil {
			authorBins := make([][]byte, len(f.Authors))
			for i, pubkey := range f.Authors {
				var err error
				authorBins[i], err = hex.DecodeString(pubkey)
				if err != nil {
					return "", nil, fmt.Errorf("failed to decode pubkey: %w", err)
				}
			}

			epubkey := goqu.T("events").As("epubkey")

			sub = sub.
				Join(epubkey, goqu.On(
					esub.Col("event_key").Eq(epubkey.Col("event_key")),
					esub.Col("created_at").Eq(epubkey.Col("created_at")),
				)).
				Where(epubkey.Col("pubkey").In(authorBins))
		}

		if f.Kinds != nil {
			ekind := goqu.T("events").As("ekind")

			sub = sub.
				Join(ekind, goqu.On(
					esub.Col("event_key").Eq(ekind.Col("event_key")),
					esub.Col("created_at").Eq(ekind.Col("created_at")),
				)).
				Where(ekind.Col("kind").In(f.Kinds))
		}

		if f.Tags != nil {
			sub = sub.Distinct()

			for key, values := range f.Tags {
				tagHashes := make([][]byte, len(values))
				for i, value := range values {
					b := md5.Sum([]byte(key + value))
					tagHashes[i] = b[:]
				}

				etag := t.As("etag" + key)

				sub = sub.
					Join(etag, goqu.On(
						esub.Col("event_key").Eq(etag.Col("event_key")),
						esub.Col("created_at").Eq(etag.Col("created_at")),
					)).
					Where(etag.Col("tag_hash").In(tagHashes))
			}
		}

		subs = append(subs, sub)
	}

	var ors []exp.Expression
	for _, sub := range subs {
		or := eEventKey.In(sub)
		ors = append(ors, or)
	}

	builder = builder.Where(goqu.Or(ors...))

	return builder.Prepared(true).ToSQL()
}

func appendDeletedEventsQuery(
	b *goqu.SelectDataset,
	eventKeyCol, eventIDCol, pubkeyCol exp.IdentifierExpression,
) *goqu.SelectDataset {
	dKey := goqu.T("deleted_event_keys")
	dKeyEventKey := dKey.Col("event_key")
	dKeyPubkey := dKey.Col("pubkey")

	dID := goqu.T("deleted_event_ids")
	dIDID := dID.Col("id")
	dIDPubkey := dID.Col("pubkey")

	return b.
		Where(goqu.L("not exists ?",
			goqu.
				Select(goqu.L("1")).
				From(dKey).
				Where(dKeyEventKey.Eq(eventKeyCol)).
				Where(dKeyPubkey.Eq(pubkeyCol)),
		)).
		Where(goqu.L("not exists ?",
			goqu.
				Select(goqu.L("1")).
				From(dID).
				Where(dIDID.Eq(eventIDCol)).
				Where(dIDPubkey.Eq(pubkeyCol)),
		))
}

func appendSinceQuery(
	b *goqu.SelectDataset,
	createdAtCol exp.IdentifierExpression,
	since *int64,
) *goqu.SelectDataset {
	if since != nil {
		b = b.Where(createdAtCol.Gte(*since))
	}
	return b
}

func appendUntilQuery(
	b *goqu.SelectDataset,
	createdAtCol exp.IdentifierExpression,
	until *int64,
) *goqu.SelectDataset {
	if until != nil {
		b = b.Where(createdAtCol.Lte(*until))
	}
	return b
}

func appendLimitQuery(b *goqu.SelectDataset, limit *int64, maxLimit uint) *goqu.SelectDataset {
	l := maxLimit
	if limit != nil {
		l = min(l, uint(*limit))
	}
	if l != NoLimit {
		b = b.Limit(l)
	}
	return b
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
		var raw rawEvent
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
	ID        []byte
	Pubkey    []byte
	CreatedAt int64
	Kind      int64
	Tags      []byte
	Content   string
	Sig       []byte
}

func (r *rawEvent) toEvent() (*mocrelay.Event, error) {
	var tags []mocrelay.Tag
	if err := json.Unmarshal(r.Tags, &tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	return &mocrelay.Event{
		ID:        hex.EncodeToString(r.ID),
		Pubkey:    hex.EncodeToString(r.Pubkey),
		CreatedAt: r.CreatedAt,
		Kind:      r.Kind,
		Tags:      tags,
		Content:   r.Content,
		Sig:       hex.EncodeToString(r.Sig),
	}, nil
}
