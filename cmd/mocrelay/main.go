package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/high-moctane/mocrelay"
)

func main() {
	ctx := context.Background()
	h := mocrelay.NewMergeHandler(mocrelay.NewCacheHandler(ctx, 100), mocrelay.NewRouter(nil))
	h = mocrelay.NewRecvEventUniquefyMiddleware(100)(h)
	h = mocrelay.NewSendEventUniquefyMiddleware(100)(h)
	h = mocrelay.NewEventCreatedAtReqFilterMiddleware(-5*time.Minute, 1*time.Minute)(h)

	r := mocrelay.NewRelay(h, &mocrelay.RelayOption{
		Logger:     slog.Default(),
		RecvLogger: slog.Default(),
		SendLogger: slog.Default(),
	})

	slog.ErrorContext(ctx, "mocrelay terminated", http.ListenAndServe("localhost:8234", r))
}
