package mocrelay

import (
	"context"
	"log/slog"
)

func mocrelaySlog(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return nil
	}

	id := GetRequestID(ctx)
	ip := GetRealIP(ctx)

	return logger.WithGroup("mocrelay").With(slog.String("RequestID", id), slog.String("ip", ip))
}
