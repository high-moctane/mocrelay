package mocrelay

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// ContextWithLogger returns a new context with the given logger.
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// LoggerFromContext returns the logger from the context.
// If no logger is found, it returns slog.Default().
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

type connIDKey struct{}

// ContextWithConnID returns a new context with the given connection ID.
func ContextWithConnID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, connIDKey{}, id)
}

// ConnIDFromContext returns the connection ID from the context.
// If no connection ID is found, it returns an empty string.
func ConnIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(connIDKey{}).(string); ok {
		return id
	}
	return ""
}
