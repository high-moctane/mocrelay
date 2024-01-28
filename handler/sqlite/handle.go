package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/high-moctane/mocrelay"
)

type SQLiteHandler struct {
	db *sql.DB
}

func NewSQLiteHandler(ctx context.Context, db *sql.DB) (*SQLiteHandler, error) {
	if err := Migrate(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return &SQLiteHandler{
		db: db,
	}, nil
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
	events, err := queryEvent(ctx, h.db, msg.ReqFilters)
	if err != nil {
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
	affected, err := insertEvents(ctx, h.db, []*mocrelay.Event{msg.Event})
	if err != nil {
		smsg := mocrelay.NewServerOKMsg(
			msg.Event.ID,
			false,
			mocrelay.ServerOkMsgPrefixError,
			"failed to save the event",
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case send <- smsg:
			return nil
		}
	}
	if affected == 0 {
		smsg := mocrelay.NewServerOKMsg(
			msg.Event.ID,
			false,
			mocrelay.ServerOKMsgPrefixDuplicate,
			"the event is already saved",
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case send <- smsg:
			return nil
		}
	}

	smsg := mocrelay.NewServerOKMsg(msg.Event.ID, true, "", "")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case send <- smsg:
		return nil
	}
}
