package sqlite

import (
	"context"

	"github.com/high-moctane/mocrelay"
)

type SQLiteHandler struct{}

func (h *SQLiteHandler) Handle(
	ctx context.Context,
	recv <-chan mocrelay.ClientMsg,
	send chan<- mocrelay.ServerMsg,
) {
	panic("unimplemented")
}
