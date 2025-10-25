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

	// Load configuration
	config, err := mocrelay.LoadConfig()
	if err != nil {
		panic(err)
	}

	// Setup Prometheus registry
	reg := prometheus.NewRegistry()

	// Build handlers based on configuration
	handlers := make([]mocrelay.Handler, 0)

	// Add cache handler
	handlers = append(handlers, mocrelay.NewCacheHandler(config.CacheSize))

	// Add router handler
	handlers = append(handlers, mocrelay.NewRouterHandler(config.RouterSize))

	// Add SQLite handler if enabled
	if config.SQLiteEnabled {
		db, err := sql.Open("sqlite3", config.SQLitePath)
		if err != nil {
			panic(err)
		}
		defer db.Close()
		if err := db.Ping(); err != nil {
			panic(err)
		}

		sqliteOpt := &mocsqlite.SQLiteHandlerOption{
			EventBulkInsertNum: config.SQLiteBulkInsertNum,
			EventBulkInsertDur: config.SQLiteBulkInsertDuration,
			MaxLimit:           uint(config.SQLiteMaxLimit),
			Logger:             slog.Default(),
		}
		sqliteHandler, err := mocsqlite.NewSQLiteHandler(ctx, db, sqliteOpt)
		if err != nil {
			panic(err)
		}
		handlers = append(handlers, sqliteHandler)
	}

	// Merge all handlers
	h := mocrelay.NewMergeHandler(handlers...)

	// Add Prometheus middleware if enabled
	if config.PrometheusEnabled {
		h = mocprom.NewPrometheusMiddleware(reg)(h)
	}

	// Create relay with configured options
	opt := &mocrelay.RelayOption{
		Logger:             slog.Default(),
		SendTimeout:        config.ServerSendTimeout,
		RecvRateLimitRate:  config.RelayRecvRateLimitRate,
		RecvRateLimitBurst: config.RelayRecvRateLimitBurst,
		MaxMessageLength:   config.RelayMaxMessageLength,
		PingDuration:       config.ServerPingDuration,
	}
	relay := mocrelay.NewRelay(h, opt)

	// Use NIP-11 from configuration
	relayMux := &mocrelay.ServeMux{
		Relay: relay,
		NIP11: config.NIP11,
	}

	// Setup HTTP handlers
	mux := http.NewServeMux()
	mux.Handle("/", relayMux)
	if config.PrometheusEnabled {
		mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	}

	// Create HTTP server with configured address
	srv := &http.Server{
		Addr:        config.ServerAddr,
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
