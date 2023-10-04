package mocrelay

import (
	"context"
	"log/slog"
)

var _ slog.Handler = (*slogMocrelayHandler)(nil)

type slogMocrelayHandler struct {
	h slog.Handler
}

func WithSlogMocrelayHandler(handler slog.Handler) *slogMocrelayHandler {
	return &slogMocrelayHandler{handler}
}

func (h *slogMocrelayHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

func (h *slogMocrelayHandler) Handle(ctx context.Context, record slog.Record) error {
	id := GetRequestID(ctx)
	ip := GetRealIP(ctx)
	return h.h.WithGroup("mocrelay").
		WithAttrs([]slog.Attr{slog.String("requestID", id), slog.String("realIP", ip)}).
		Handle(ctx, record)
}

func (h *slogMocrelayHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return WithSlogMocrelayHandler(h.h.WithAttrs(attrs))
}

func (h *slogMocrelayHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return WithSlogMocrelayHandler(h.h.WithGroup(name))
}
