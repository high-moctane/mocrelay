package main

import (
	"context"
	"io"
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

	h := mocrelay.NewMergeHandler(
		mocrelay.NewCacheHandler(100),
		mocrelay.NewSendEventUniqueFilterMiddleware(10)(mocrelay.NewRouterHandler(100)),
	)
	h = mocrelay.NewEventCreatedAtMiddleware(-5*time.Minute, 1*time.Minute)(h)
	h = mocrelay.NewRecvEventUniqueFilterMiddleware(10)(h)
	h = mocprom.NewPrometheusMiddleware(reg)(h)

	relay := mocrelay.NewRelay(h, &mocrelay.RelayOption{
		Logger:     slog.Default(),
		RecvLogger: slog.Default(),
		SendLogger: slog.Default(),
	})

	nip11 := &mocrelay.NIP11{
		Name:        "mocrelay",
		Description: "moctane's nostr relay",
		Software:    "https://github.com/high-moctane/mocrelay",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			relay.ServeHTTP(w, r)
			return
		}

		if r.Header.Get("Accept") == "application/nostr+json" {
			nip11.ServeHTTP(w, r)
			return
		}

		io.WriteString(w, "Hello Mocrelay (｀･ω･´)！")
	})
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
			relay.Wait()
			cancel()
		}()

		<-c.Done()
		srv.Shutdown(c)
	}()

	err := srv.ListenAndServe()
	slog.ErrorContext(ctx, "mocrelay terminated", "err", err)
}
