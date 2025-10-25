package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/high-moctane/mocrelay"
	mocsqlite "github.com/high-moctane/mocrelay/handler/sqlite"
	mocprom "github.com/high-moctane/mocrelay/middleware/prometheus"
)

func main() {
	ctx := context.Background()

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer cancel()

	reg := prometheus.NewRegistry()

	db, err := sql.Open("sqlite3", ":memory:?cache=shared")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		panic(err)
	}

	sqliteHandler, err := mocsqlite.NewSQLiteHandler(ctx, db, nil)
	if err != nil {
		panic(err)
	}

	h := mocrelay.NewMergeHandler(
		mocrelay.NewCacheHandler(100),
		mocrelay.NewRouterHandler(100),
		sqliteHandler,
	)
	h = mocprom.NewPrometheusMiddleware(reg)(h)

	opt := mocrelay.NewDefaultRelayOption()
	opt.Logger = slog.Default()
	relay := mocrelay.NewRelay(h, opt)

	nip11 := &mocrelay.NIP11{
		Name:        "mocrelay",
		Description: "moctane's nostr relay",
		Software:    "https://github.com/high-moctane/mocrelay",
	}

	relayMux := &mocrelay.ServeMux{
		Relay: relay,
		NIP11: nip11,
	}

	mux := http.NewServeMux()
	mux.Handle("/", relayMux)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	srv := &http.Server{
		Addr:        "localhost:8234",
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()

		// まずHTTPサーバーをシャットダウン（新規接続を受け付けなくする）
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(ctx, "server shutdown failed", "err", err)
		}

		// 既存接続が終わるのを待つ（タイムアウト付き）
		waitDone := make(chan struct{})
		go func() {
			relay.Wait()
			close(waitDone)
		}()

		waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer waitCancel()

		select {
		case <-waitDone:
			slog.InfoContext(ctx, "all connections closed gracefully")
		case <-waitCtx.Done():
			slog.WarnContext(ctx, "shutdown timeout: some connections may not have closed gracefully")
		}
	}()

	err = srv.ListenAndServe()
	slog.ErrorContext(ctx, "mocrelay terminated", "err", err)
}
