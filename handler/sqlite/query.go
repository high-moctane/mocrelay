package sqlite

import (
	"context"
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

var (
	e          = goqu.T("events")
	eEventKey  = e.Col("event_key")
	eID        = e.Col("id")
	ePubkey    = e.Col("pubkey")
	eCreatedAt = e.Col("created_at")
	eKind      = e.Col("kind")

	p         = goqu.T("event_payloads")
	pEventKey = p.Col("event_key")
	pTags     = p.Col("tags")
	pContent  = p.Col("content")
	pSig      = p.Col("sig")

	t         = goqu.T("event_tags")
	tEventKey = t.Col("event_key")
	tKey      = t.Col("key")
	tValue    = t.Col("value")

	dKey         = goqu.T("deleted_event_keys")
	dKeyEventKey = dKey.Col("event_key")
	dKeyPubkey   = dKey.Col("pubkey")

	dID       = goqu.T("deleted_event_ids")
	dIDID     = dID.Col("id")
	dIDPubkey = dID.Col("pubkey")

	l          = goqu.T("lookup_hashes")
	lEventKey  = l.Col("event_key")
	lHash      = l.Col("hash")
	lCreatedAt = l.Col("created_at")
)

func buildEventQuery(
	fs []*mocrelay.ReqFilter,
	seed uint32,
	maxLimit uint,
) (query string, param []any, err error) {
	var builder *goqu.SelectDataset

	for i, f := range fs {
		var b *goqu.SelectDataset
		if needLookupTable(f) {
			b = buildLookupsQuery(seed, f)
			b = appendSelectLookupsEventKey(b)

		} else {
			b = buildEventsQuery(f)
			if len(fs) == 1 {
				b = appendSelectEvent(b)
			} else {
				b = appendSelectEventsEventKey(b)
			}
		}

		if i == 0 {
			builder = b
		} else {
			builder = builder.Union(b)
		}
	}

	if len(fs) > 1 || needLookupTable(fs[0]) {
		builder = goqu.
			Dialect("sqlite3").
			From(e).
			Where(eEventKey.In(builder)).
			Order(eCreatedAt.Desc())

		builder = appendSelectEvent(builder)
	}

	builder = appendPayloads(builder)
	builder = appendLimit(builder, &mocrelay.ReqFilter{}, maxLimit)

	return builder.Prepared(true).ToSQL()
}

func buildEventsQuery(f *mocrelay.ReqFilter) *goqu.SelectDataset {
	b := goqu.
		Dialect("sqlite3").
		From(e).
		Order(eCreatedAt.Desc())

	b = appendIsDeleted(b)
	b = appendSince(b, eCreatedAt, f)
	b = appendUntil(b, eCreatedAt, f)
	b = appendLimit(b, f, NoLimit)

	return b
}

func buildLookupsQuery(seed uint32, f *mocrelay.ReqFilter) *goqu.SelectDataset {
	conds := createLookupCondsFromReqFilter(seed, f)

	ands := make([]exp.Expression, 0, len(conds))
	for _, cond := range conds {
		var exps []exp.Expression

		exps = append(exps, lHash.Eq(cond.hash))

		if cond.ID != nil {
			idBin, err := hex.DecodeString(*cond.ID)
			if err != nil {
				return nil
			}
			exps = append(exps, goqu.L("exists ?",
				goqu.
					Select(goqu.L("1")).
					From(e).
					Where(eEventKey.Eq(lEventKey)).
					Where(eID.Eq(idBin)),
			))
		}
		if cond.Pubkey != nil {
			pubkeyBin, err := hex.DecodeString(*cond.Pubkey)
			if err != nil {
				return nil
			}
			exps = append(exps, goqu.L("exists ?",
				goqu.
					Select(goqu.L("1")).
					From(e).
					Where(eEventKey.Eq(lEventKey)).
					Where(ePubkey.Eq(pubkeyBin)),
			))
		}
		if cond.Kind != nil {
			exps = append(exps, goqu.L("exists ?",
				goqu.
					Select(goqu.L("1")).
					From(e).
					Where(eEventKey.Eq(lEventKey)).
					Where(eKind.Eq(cond.Kind)),
			))
		}
		for _, tag := range cond.Tags {
			exps = append(exps, goqu.L("exists ?",
				goqu.
					Select(goqu.L("1")).
					From(t).
					Where(tEventKey.Eq(lEventKey)).
					Where(tKey.Eq([]byte(tag.Key))).
					Where(tValue.Eq([]byte(tag.Value))),
			))
		}

		ands = append(ands, goqu.And(exps...))
	}

	b := goqu.
		Dialect("sqlite3").
		From(l).
		Where(goqu.Or(ands...)).
		Order(lCreatedAt.Desc())

	b = appendIsDeleted(b)
	b = appendSince(b, lCreatedAt, f)
	b = appendUntil(b, lCreatedAt, f)
	b = appendLimit(b, f, NoLimit)

	return b
}

func appendSelectEvent(b *goqu.SelectDataset) *goqu.SelectDataset {
	return b.Select(
		eID,
		ePubkey,
		eCreatedAt,
		eKind,
		pTags,
		pContent,
		pSig,
	)
}

func appendSelectEventsEventKey(b *goqu.SelectDataset) *goqu.SelectDataset {
	return b.Select(eEventKey)
}

func appendSelectLookupsEventKey(b *goqu.SelectDataset) *goqu.SelectDataset {
	return b.Select(lEventKey)
}

func appendIsDeleted(b *goqu.SelectDataset) *goqu.SelectDataset {
	return b.Where(goqu.L("not exists ?",
		goqu.
			Select(goqu.L("1")).
			From(dKey).
			Where(dKeyEventKey.Eq(eEventKey)).
			Where(dKeyPubkey.Eq(ePubkey)),
	)).
		Where(goqu.L("not exists ?",
			goqu.
				Select(goqu.L("1")).
				From(dID).
				Where(dIDID.Eq(eID)).
				Where(dIDPubkey.Eq(ePubkey)),
		))
}

func appendSince(
	b *goqu.SelectDataset,
	createdAtCol exp.IdentifierExpression,
	f *mocrelay.ReqFilter,
) *goqu.SelectDataset {
	if f.Since != nil {
		b = b.Where(createdAtCol.Gte(f.Since))
	}
	return b
}

func appendUntil(
	b *goqu.SelectDataset,
	createdAtCol exp.IdentifierExpression,
	f *mocrelay.ReqFilter,
) *goqu.SelectDataset {
	if f.Until != nil {
		b = b.Where(createdAtCol.Lte(f.Until))
	}
	return b
}

func appendLimit(b *goqu.SelectDataset, f *mocrelay.ReqFilter, maxLimit uint) *goqu.SelectDataset {
	limit := maxLimit
	if f.Limit != nil {
		limit = min(limit, uint(*f.Limit))
	}
	if limit != NoLimit {
		b = b.Limit(limit)
	}
	return b
}

func appendPayloads(b *goqu.SelectDataset) *goqu.SelectDataset {
	return b.Join(p, goqu.On(eEventKey.Eq(pEventKey)))
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
