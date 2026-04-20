package mocrelay

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

func TestLogRejection(t *testing.T) {
	t.Run("emits debug log with uniform keys", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		ctx := ContextWithLogger(context.Background(), logger)

		logRejection(ctx, "max_event_tags", "too_many_tags",
			"event_id", "abc123",
			"count", 100,
		)

		out := buf.String()
		for _, want := range []string{
			`level=DEBUG`,
			`msg="message rejected"`,
			`middleware=max_event_tags`,
			`reason=too_many_tags`,
			`event_id=abc123`,
			`count=100`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\nfull output: %s", want, out)
			}
		}
	})

	t.Run("default logger skips debug by default", func(t *testing.T) {
		// logRejection is Debug level, so it should be silent under the
		// default logger (Info level). This guards against accidentally
		// promoting it to a higher level.
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil)) // default: Info
		ctx := ContextWithLogger(context.Background(), logger)

		logRejection(ctx, "auth", "event_unauthenticated", "event_id", "x")

		if buf.Len() != 0 {
			t.Errorf("expected no output at Info level, got: %s", buf.String())
		}
	})

	t.Run("increments rejection counter from ctx", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		metrics := newRejectionMetrics(reg)
		ctx := contextWithRejectionMetrics(context.Background(), metrics)

		logRejection(ctx, "max_content_length", "content_too_long",
			"event_id", "abc", "length", 9999,
		)
		logRejection(ctx, "max_content_length", "content_too_long",
			"event_id", "def", "length", 9999,
		)
		logRejection(ctx, "auth", "event_unauthenticated",
			"event_id", "ghi",
		)

		if got := testutil.ToFloat64(metrics.Total.WithLabelValues(
			"max_content_length", "content_too_long",
		)); got != 2 {
			t.Errorf("max_content_length/content_too_long: want 2, got %v", got)
		}
		if got := testutil.ToFloat64(metrics.Total.WithLabelValues(
			"auth", "event_unauthenticated",
		)); got != 1 {
			t.Errorf("auth/event_unauthenticated: want 1, got %v", got)
		}
	})

	t.Run("no-op when no metrics in ctx", func(t *testing.T) {
		// Plain context.Background() — no RejectionMetrics installed.
		// Should not panic and should still log.
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		ctx := ContextWithLogger(context.Background(), logger)

		logRejection(ctx, "auth", "event_unauthenticated", "event_id", "x")

		if buf.Len() == 0 {
			t.Error("expected log output, got none")
		}
	})
}

func TestRejectionMetricsFromContext(t *testing.T) {
	t.Run("returns nil when not set", func(t *testing.T) {
		if got := rejectionMetricsFromContext(context.Background()); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("returns metrics from context", func(t *testing.T) {
		metrics := newRejectionMetrics(prometheus.NewRegistry())
		ctx := contextWithRejectionMetrics(context.Background(), metrics)
		if got := rejectionMetricsFromContext(ctx); got != metrics {
			t.Fatalf("expected same metrics, got %v", got)
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
