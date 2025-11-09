package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/high-moctane/mocrelay"
)

const NoLimit = math.MaxUint

type SQLiteHandlerOption struct {
	EventBulkInsertNum int
	EventBulkInsertDur time.Duration

	MaxLimit uint

	Logger *slog.Logger
}

func NewDefaultSQLiteHandlerOption() *SQLiteHandlerOption {
	return &SQLiteHandlerOption{
		EventBulkInsertNum: 1000,
		EventBulkInsertDur: 2 * time.Minute,
		MaxLimit:           NoLimit,
	}
}

type SQLiteHandler mocrelay.Handler

func NewSQLiteHandler(
	ctx context.Context,
	db *sql.DB,
	opt *SQLiteHandlerOption,
) (SQLiteHandler, error) {
	h, err := newSimpleSQLiteHandler(ctx, db, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLiteHandler: %w", err)
	}
	return SQLiteHandler(mocrelay.NewSimpleHandler(h)), nil
}

type simpleSQLiteHandler struct {
	db      *sql.DB
	eventCh chan *mocrelay.Event
	seed    uint32

	opt SQLiteHandlerOption
}

func newSimpleSQLiteHandler(
	ctx context.Context,
	db *sql.DB,
	opt *SQLiteHandlerOption,
) (*simpleSQLiteHandler, error) {
	if err := Migrate(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	seed, err := setOrLoadXXHashSeed(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("failed to set or load xxhash seed: %w", err)
	}

	var option SQLiteHandlerOption
	if opt != nil {
		option = *opt
	} else {
		option = *NewDefaultSQLiteHandlerOption()
	}

	h := &simpleSQLiteHandler{
		db:      db,
		eventCh: make(chan *mocrelay.Event, 2*option.EventBulkInsertNum),
		seed:    seed,
		opt:     option,
	}

	go h.serveBulkInsert(ctx)

	return h, nil
}

func (h *simpleSQLiteHandler) ServeNostrStart(
	ctx context.Context,
) (context.Context, error) {
	return ctx, nil
}

func (h *simpleSQLiteHandler) ServeNostrEnd(ctx context.Context) error {
	return nil
}

func (h *simpleSQLiteHandler) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ServerMsg, error) {
	switch msg := msg.(type) {
	case *mocrelay.ClientReqMsg:
		return h.serveClientReqMsg(ctx, msg)

	case *mocrelay.ClientEventMsg:
		return h.serveClientEventMsg(ctx, msg)

	default:
		return mocrelay.DefaultSimpleHandlerBase{}.ServeNostrClientMsg(ctx, msg)
	}
}

func (h *simpleSQLiteHandler) serveClientReqMsg(
	ctx context.Context,
	msg *mocrelay.ClientReqMsg,
) (<-chan mocrelay.ServerMsg, error) {
	events, err := queryEvent(ctx, h.db, h.seed, msg.ReqFilters, h.opt.MaxLimit)
	if err != nil {
		warnLog(ctx, h.opt.Logger, "failed to query events", "err", err)

		smsgCh := make(chan mocrelay.ServerMsg, 1)
		defer close(smsgCh)

		smsgCh <- mocrelay.NewServerEOSEMsg(msg.SubscriptionID)

		return smsgCh, nil
	}

	smsgCh := make(chan mocrelay.ServerMsg, len(events)+1)
	defer close(smsgCh)

	for _, event := range events {
		smsgCh <- mocrelay.NewServerEventMsg(msg.SubscriptionID, event)
	}

	smsgCh <- mocrelay.NewServerEOSEMsg(msg.SubscriptionID)

	return smsgCh, nil
}

func (h *simpleSQLiteHandler) serveClientEventMsg(
	ctx context.Context,
	msg *mocrelay.ClientEventMsg,
) (<-chan mocrelay.ServerMsg, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case h.eventCh <- msg.Event:
		smsgCh := make(chan mocrelay.ServerMsg, 1)
		defer close(smsgCh)

		smsgCh <- mocrelay.NewServerOKMsg(msg.Event.ID, true, "", "")

		return smsgCh, nil
	}
}

func (h *simpleSQLiteHandler) serveBulkInsert(ctx context.Context) {
	events := make([]*mocrelay.Event, 0, h.opt.EventBulkInsertNum)
	seen, err := lru.New[string, struct{}](2 * h.opt.EventBulkInsertNum)
	if err != nil {
		panic(fmt.Errorf("failed to create LRU: %w", err))
	}

	var tickCh <-chan time.Time
	if h.opt.EventBulkInsertDur > 0 {
		ticker := time.NewTicker(h.opt.EventBulkInsertDur)
		defer ticker.Stop()
		tickCh = ticker.C
	}

	for {
		select {
		case <-ctx.Done():
			if len(events) > 0 {
				c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := h.bulkInsertWithRetry(c, events); err != nil {
					errorLog(ctx, h.opt.Logger, "failed to insert events", "err", err)
				}
				infoLog(ctx, h.opt.Logger, "inserted events", "num", len(events))
			}
			return

		case event := <-h.eventCh:
			if _, ok := seen.Get(event.ID); ok {
				continue
			}
			seen.Add(event.ID, struct{}{})
			events = append(events, event)
			if len(events) >= h.opt.EventBulkInsertNum {
				if err := h.bulkInsertWithRetry(ctx, events); err != nil {
					errorLog(ctx, h.opt.Logger, "failed to insert events", "err", err)
				}
				infoLog(ctx, h.opt.Logger, "inserted events", "num", len(events))
				events = events[:0]
			}

		case <-tickCh:
			if len(events) > 0 {
				if err := h.bulkInsertWithRetry(ctx, events); err != nil {
					errorLog(ctx, h.opt.Logger, "failed to insert events", "err", err)
				}
				infoLog(ctx, h.opt.Logger, "inserted events", "num", len(events))
				events = events[:0]
			}
			if _, err := h.db.ExecContext(ctx, "pragma wal_checkpoint(TRUNCATE)"); err != nil {
				errorLog(ctx, h.opt.Logger, "failed to checkpoint", "err", err)
			}
		}
	}
}

func (h *simpleSQLiteHandler) bulkInsertWithRetry(
	ctx context.Context,
	events []*mocrelay.Event,
) error {
	for i := range 3 {
		if err := insertEvents(ctx, h.db, h.seed, events); err != nil {
			errorLog(ctx, h.opt.Logger, "failed to insert events", "err", err, "retry", i+1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second << uint(i)):
			}
		} else {
			return nil
		}
	}
	return fmt.Errorf("failed to insert events")
}
