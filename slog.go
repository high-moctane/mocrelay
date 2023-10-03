package mocrelay

import (
	"context"
	"log/slog"
)

var _ slog.Handler = (*SlogMocrelayHandler)(nil)

type SlogMocrelayHandler struct {
	h slog.Handler
}

func WithSlogMocrelayHandler(handler slog.Handler) *SlogMocrelayHandler {
	return &SlogMocrelayHandler{handler}
}

func (h *SlogMocrelayHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

func (h *SlogMocrelayHandler) Handle(ctx context.Context, record slog.Record) error {
	id := GetRequestID(ctx)
	ip := GetRealIP(ctx)
	return h.h.WithGroup("mocrelay").
		WithAttrs([]slog.Attr{slog.String("requestID", id), slog.String("realIP", ip)}).
		Handle(ctx, record)
}

func (h *SlogMocrelayHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return WithSlogMocrelayHandler(h.h.WithAttrs(attrs))
}

func (h *SlogMocrelayHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return WithSlogMocrelayHandler(h.h.WithGroup(name))
}
