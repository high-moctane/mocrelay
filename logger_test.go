package mocrelay

import (
	"context"
	"log/slog"
	"testing"
)

func TestLoggerFromContext(t *testing.T) {
	t.Run("returns default logger when not set", func(t *testing.T) {
		ctx := context.Background()
		logger := LoggerFromContext(ctx)
		if logger != slog.Default() {
			t.Fatal("expected slog.Default()")
		}
	})

	t.Run("returns logger from context", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(nil, nil))
		ctx := ContextWithLogger(context.Background(), logger)
		got := LoggerFromContext(ctx)
		if got != logger {
			t.Fatal("expected the same logger")
		}
	})
}

func TestConnIDFromContext(t *testing.T) {
	t.Run("returns empty string when not set", func(t *testing.T) {
		ctx := context.Background()
		if got := ConnIDFromContext(ctx); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("returns conn ID from context", func(t *testing.T) {
		ctx := contextWithConnID(context.Background(), "abc-123")
		if got := ConnIDFromContext(ctx); got != "abc-123" {
			t.Fatalf("expected %q, got %q", "abc-123", got)
		}
	})
}
