package sqlite

import (
	"context"
	"log/slog"
)

func errorLog(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.ErrorContext(ctx, msg, args...)
}
