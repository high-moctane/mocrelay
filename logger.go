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

// logRejection reports a middleware-level message rejection with a uniform
// key set. It performs two side-effects:
//
//  1. Increments the unified [RejectionMetrics] counter from the context
//     (labels {middleware, reason}) if one is installed. No-op if absent.
//  2. Emits a Debug-level structured log entry ("message rejected") so
//     operators can find all rejections with a single grep.
//
// Both sinks share the middleware and reason strings, making logs and
// metrics cross-indexable by the reason label.
//
// Rejections are Debug (not Warn) because they are an expected outcome of
// normal middleware policy and would otherwise flood the log. Enable Debug
// logging when investigating why a specific client's messages are being
// dropped; rely on the metrics counter for day-to-day trends.
func logRejection(ctx context.Context, middleware, reason string, attrs ...any) {
	if m := RejectionMetricsFromContext(ctx); m != nil {
		m.Total.WithLabelValues(middleware, reason).Inc()
	}
	base := []any{"middleware", middleware, "reason", reason}
	LoggerFromContext(ctx).DebugContext(ctx, "message rejected", append(base, attrs...)...)
}

type connIDKey struct{}

// contextWithConnID returns a new context with the given connection ID.
// It is internal; callers retrieve the value through [ConnIDFromContext].
func contextWithConnID(ctx context.Context, id string) context.Context {
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
