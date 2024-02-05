package sqlite

import (
	"context"
	"log/slog"
)

func infoLog(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.InfoContext(ctx, msg, args...)
}

func warnLog(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.WarnContext(ctx, msg, args...)
}

func errorLog(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.ErrorContext(ctx, msg, args...)
}
