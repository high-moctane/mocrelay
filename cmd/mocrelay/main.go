package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/high-moctane/mocrelay"
	mocprom "github.com/high-moctane/mocrelay/middleware/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	ctx := context.Background()

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer cancel()

	reg := prometheus.NewRegistry()

	h := mocrelay.NewMergeHandler(mocrelay.NewCacheHandler(100), mocrelay.NewRouterHandler(100))
	h = mocrelay.NewEventCreatedAtFilterMiddleware(-5*time.Minute, 1*time.Minute)(h)
	h = mocprom.NewPrometheusMiddleware(reg)(h)

	r := mocrelay.NewRelay(h, &mocrelay.RelayOption{
		Logger:     slog.Default(),
		RecvLogger: slog.Default(),
		SendLogger: slog.Default(),
	})

	mux := http.NewServeMux()
	mux.Handle("/", r)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	srv := &http.Server{
		Addr:        "localhost:8234",
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()

		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		go func() {
			r.Wait()
			cancel()
		}()

		<-c.Done()
		srv.Shutdown(c)
	}()

	err := srv.ListenAndServe()
	slog.ErrorContext(ctx, "mocrelay terminated", "err", err)
}
