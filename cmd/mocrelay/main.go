package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/high-moctane/mocrelay"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	ctx := context.Background()

	reg := prometheus.NewRegistry()

	h := mocrelay.NewMergeHandler(mocrelay.NewCacheHandler(100), mocrelay.NewRouter(nil))
	h = mocrelay.NewRecvEventUniquefyMiddleware(100)(h)
	h = mocrelay.NewSendEventUniquefyMiddleware(100)(h)
	h = mocrelay.NewEventCreatedAtFilterMiddleware(-5*time.Minute, 1*time.Minute)(h)
	h = mocrelay.NewPrometheusMiddleware(reg)(h)

	r := mocrelay.NewRelay(h, &mocrelay.RelayOption{
		Logger:     slog.Default(),
		RecvLogger: slog.Default(),
		SendLogger: slog.Default(),
	})

	http.Handle("/", r)
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	slog.ErrorContext(ctx, "mocrelay terminated", http.ListenAndServe("localhost:8234", nil))
}
