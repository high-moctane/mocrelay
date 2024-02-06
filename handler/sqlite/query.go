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
	"github.com/pierrec/xxHash/xxHash32"
)

func queryEvent(
	ctx context.Context,
	db *sql.DB,
	seed uint32,
	fs []*mocrelay.ReqFilter,
	maxLimit uint,
) (events []*mocrelay.Event, err error) {
	q, param, err := buildEventQuery(seed, fs, maxLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	events, err = fetchEventQuery(ctx, db, q, param)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events with (%s, %v): %w", q, param, err)
	}

	return
}

func gathereReqFilterHashes(seed uint32, f *mocrelay.ReqFilter) (hashes [][]uint32) {
	x := xxHash32.New(seed)

	if f.IDs != nil {
		hs := make([]uint32, 0, len(f.IDs))
		for _, id := range f.IDs {
			x.Reset()
			x.Write([]byte(id))
			hs = append(hs, x.Sum32())
		}
		hashes = append(hashes, hs)
	}

	if f.Tags != nil {
		for tag, values := range f.Tags {
			tag := string(tag[1])
			hs := make([]uint32, 0, len(values))
			for _, value := range values {
				x.Reset()
				x.Write([]byte(tag + value))
				hs = append(hs, x.Sum32())
			}
			hashes = append(hashes, hs)
		}
	}

	if f.Authors != nil {
		hs := make([]uint32, 0, len(f.Authors))
		for _, author := range f.Authors {
			x.Reset()
			x.Write([]byte(author))
			hs = append(hs, x.Sum32())
		}
		hashes = append(hashes, hs)
	}

	if f.Kinds != nil {
		hs := make([]uint32, 0, len(f.Kinds))
		for _, kind := range f.Kinds {
			x.Reset()
			x.Write([]byte(strconv.FormatInt(kind, 10)))
			hs = append(hs, x.Sum32())
		}
		hashes = append(hashes, hs)
	}

	return
}

func buildEventQuery(
	seed uint32,
	fs []*mocrelay.ReqFilter,
	maxLimit uint,
) (query string, param []any, err error) {
	e := goqu.T("events")

	var builder *goqu.SelectDataset

	for i, f := range fs {
		hashes := gathereReqFilterHashes(seed, f)

		b := goqu.Dialect("sqlite3").Select(
			e.Col("id"),
			e.Col("pubkey"),
			e.Col("created_at"),
			e.Col("kind"),
			e.Col("tags"),
			e.Col("content"),
			e.Col("sig"),
		).Distinct().From("events")

		for n, hs := range hashes {
			tname := fmt.Sprintf("hashes%d", n)
			tn := goqu.T(tname)
			b = b.Join(goqu.I("hashes").As(tname), goqu.On(
				goqu.And(
					goqu.T(tname).Col("hashed_value").In(hs),
					goqu.T(tname).Col("created_at").Eq(e.Col("created_at")),
					tn.Col("hashed_id").Eq(e.Col("hashed_id")),
				),
			))
		}
		if len(hashes) == 0 {
			b = b.Order(e.Col("created_at").Desc())
		} else {
			b = b.Order(goqu.I("hashes0.created_at").Desc())
		}

		b = b.Where(goqu.L("(events.key, 'a', events.pubkey)").NotIn(
			goqu.Dialect("sqlite3").From("deleted_keys"),
		))
		b = b.Where(goqu.L("(events.id, 'e', events.pubkey)").NotIn(
			goqu.Dialect("sqlite3").From("deleted_keys"),
		))

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
					tag := string(tag[1])
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

			limit := maxLimit
			if f.Limit != nil {
				limit = min(limit, uint(*f.Limit))
			}
			if limit != NoLimit {
				b = b.Limit(uint(limit))
			}
		}

		if i == 0 {
			builder = b
		} else {
			builder = builder.Union(b)
		}
	}

	if len(fs) > 1 {
		builder = goqu.Dialect("sqlite3").Select(
			goqu.I("id"),
			goqu.I("pubkey"),
			goqu.I("created_at"),
			goqu.I("kind"),
			goqu.I("tags"),
			goqu.I("content"),
			goqu.I("sig"),
		).
			From(builder).
			Distinct().
			Order(goqu.I("created_at").
				Desc())

		if maxLimit != NoLimit {
			builder = builder.Limit(maxLimit)
		}
	}

	return builder.Prepared(true).ToSQL()
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