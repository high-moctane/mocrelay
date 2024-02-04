package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"time"

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

type SQLiteHandler struct {
	db      *sql.DB
	eventCh chan *mocrelay.Event
	seed    uint32

	opt SQLiteHandlerOption
}

func NewSQLiteHandler(
	ctx context.Context,
	db *sql.DB,
	opt *SQLiteHandlerOption,
) (*SQLiteHandler, error) {
	if err := Migrate(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	seed, err := getOrSetSeed(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("failed to get or set seed: %w", err)
	}

	var option SQLiteHandlerOption
	if opt != nil {
		option = *opt
	} else {
		option = *NewDefaultSQLiteHandlerOption()
	}

	h := &SQLiteHandler{
		db:      db,
		eventCh: make(chan *mocrelay.Event, option.EventBulkInsertNum),
		seed:    seed,
		opt:     option,
	}

	go h.serveBulkInsert(ctx)
	go h.servePragmaOptimize(ctx, db)

	return h, nil
}

func (h *SQLiteHandler) Handle(
	ctx context.Context,
	recv <-chan mocrelay.ClientMsg,
	send chan<- mocrelay.ServerMsg,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-recv:
			if !ok {
				return mocrelay.ErrRecvClosed
			}

			switch msg := msg.(type) {
			case *mocrelay.ClientReqMsg:
				if err := h.serveClientReqMsg(ctx, send, msg); err != nil {
					return err
				}

			case *mocrelay.ClientCountMsg:
				if err := h.serveClientCountMsg(ctx, send, msg); err != nil {
					return err
				}

			case *mocrelay.ClientEventMsg:
				if err := h.serveClientEventMsg(ctx, send, msg); err != nil {
					return err
				}
			}
		}
	}
}

func (h *SQLiteHandler) serveClientReqMsg(
	ctx context.Context,
	send chan<- mocrelay.ServerMsg,
	msg *mocrelay.ClientReqMsg,
) error {
	events, err := queryEvent(ctx, h.db, h.seed, msg.ReqFilters, h.opt.MaxLimit)
	if err != nil {
		errorLog(ctx, h.opt.Logger, "failed to query events", "err", err)

		smsg := mocrelay.NewServerEOSEMsg(msg.SubscriptionID)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case send <- smsg:
			return nil
		}
	}

	for _, event := range events {
		smsg := mocrelay.NewServerEventMsg(msg.SubscriptionID, event)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case send <- smsg:
		}
	}

	smsg := mocrelay.NewServerEOSEMsg(msg.SubscriptionID)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case send <- smsg:
		return nil
	}
}

func (h *SQLiteHandler) serveClientCountMsg(
	ctx context.Context,
	send chan<- mocrelay.ServerMsg,
	msg *mocrelay.ClientCountMsg,
) error {
	smsg := mocrelay.NewServerCountMsg(msg.SubscriptionID, 0, nil)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case send <- smsg:
		return nil
	}
}

func (h *SQLiteHandler) serveClientEventMsg(
	ctx context.Context,
	send chan<- mocrelay.ServerMsg,
	msg *mocrelay.ClientEventMsg,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()

	case h.eventCh <- msg.Event:
		smsg := mocrelay.NewServerOKMsg(msg.Event.ID, true, "", "")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case send <- smsg:
			return nil
		}
	}
}

func (h *SQLiteHandler) serveBulkInsert(ctx context.Context) {
	events := make([]*mocrelay.Event, 0, h.opt.EventBulkInsertNum)
	seen := make(map[string]bool, h.opt.EventBulkInsertNum)

	ticker := time.NewTicker(h.opt.EventBulkInsertDur)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if len(events) > 0 {
				c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if _, err := insertEvents(c, h.db, h.seed, events); err != nil {
					errorLog(ctx, h.opt.Logger, "failed to insert events", "err", err)
				}
			}
			return

		case event := <-h.eventCh:
			if seen[event.ID] {
				continue
			}
			seen[event.ID] = true
			events = append(events, event)
			if len(events) >= h.opt.EventBulkInsertNum {
				if _, err := insertEvents(ctx, h.db, h.seed, events); err != nil {
					errorLog(ctx, h.opt.Logger, "failed to insert events", "err", err)
				}
				events = events[:0]
				seen = make(map[string]bool, h.opt.EventBulkInsertNum)
			}

		case <-ticker.C:
			if len(events) > 0 {
				if _, err := insertEvents(ctx, h.db, h.seed, events); err != nil {
					errorLog(ctx, h.opt.Logger, "failed to insert events", "err", err)
				}
				events = events[:0]
				seen = make(map[string]bool, h.opt.EventBulkInsertNum)
			}
		}
	}
}

func (h *SQLiteHandler) servePragmaOptimize(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if _, err := db.ExecContext(ctx, "pragma optimize"); err != nil {
				errorLog(ctx, h.opt.Logger, "failed to pragma optimize", "err", err)
			}
		}
	}
}
