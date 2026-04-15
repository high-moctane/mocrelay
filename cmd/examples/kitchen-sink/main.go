// Kitchen-sink relay example.
//
// A full-featured Nostr relay demonstrating every mocrelay component:
// persistent storage (Pebble), full-text search (Bleve + CJK),
// all middleware, Prometheus metrics, and HTTP multiplexing.
//
// Since mocrelay.Relay implements http.Handler, it composes naturally
// with net/http.ServeMux for health checks, metrics, and other endpoints.
//
// Usage:
//
//	go run ./cmd/examples/kitchen-sink
//
// Environment variables:
//
//	ADDR     - listen address (default ":7447")
//	DATA_DIR - data directory for Pebble and Bleve (default "data")
package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/high-moctane/mocrelay"
)

func main() {
	addr := ":7447"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}
	dataDir := "data"
	if v := os.Getenv("DATA_DIR"); v != "" {
		dataDir = v
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// --- Prometheus registry ---

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	relayMetrics := mocrelay.NewRelayMetrics(reg)
	routerMetrics := mocrelay.NewRouterMetrics(reg)
	authMetrics := mocrelay.NewAuthMetrics(reg)
	storageMetrics := mocrelay.NewStorageMetrics(reg)

	// --- Storage layer ---

	// Pebble: persistent LSM-tree storage.
	pebbleStorage, err := mocrelay.NewPebbleStorage(
		filepath.Join(dataDir, "pebble"),
		&mocrelay.PebbleStorageOptions{
			CacheSize: 64 * 1024 * 1024, // 64 MB block cache
		},
	)
	if err != nil {
		log.Fatalf("pebble: %v", err)
	}
	defer pebbleStorage.Close()

	// Bleve: full-text search with CJK support (NIP-50).
	bleveIndex, err := mocrelay.NewBleveIndex(&mocrelay.BleveIndexOptions{
		Path: filepath.Join(dataDir, "bleve"),
	})
	if err != nil {
		log.Fatalf("bleve: %v", err)
	}
	defer bleveIndex.Close()

	// Composite: Pebble (source of truth) + Bleve (search).
	compositeStorage := mocrelay.NewCompositeStorage(pebbleStorage, bleveIndex)

	// Metrics: wrap storage with Prometheus instrumentation.
	storage := mocrelay.NewMetricsStorage(compositeStorage, storageMetrics)

	// --- Routing layer ---

	router := mocrelay.NewRouter()
	router.Metrics = routerMetrics

	// --- Handler ---

	handler := mocrelay.NewMergeHandler(
		mocrelay.NewStorageHandler(storage),
		mocrelay.NewRouterHandler(router),
	)

	// --- Middleware pipeline (outermost first) ---

	handler = mocrelay.NewSimpleMiddleware(
		// Auth (NIP-42): challenge/response authentication.
		mocrelay.NewAuthMiddlewareBase("wss://relay.example.com/", authMetrics),

		// Protected events (NIP-70): prevent republishing "-" tagged events.
		mocrelay.NewProtectedEventsMiddlewareBase(),

		// Proof of work (NIP-13): require minimum difficulty.
		mocrelay.NewMinPowDifficultyMiddlewareBase(0, true), // 0 = check commitment only

		// Subscription limits.
		mocrelay.NewMaxSubscriptionsMiddlewareBase(20),
		mocrelay.NewMaxSubidLengthMiddlewareBase(256),
		mocrelay.NewMaxLimitMiddlewareBase(500, 100),

		// Event limits.
		mocrelay.NewMaxEventTagsMiddlewareBase(2000),
		mocrelay.NewMaxContentLengthMiddlewareBase(100_000),

		// Temporal limits.
		mocrelay.NewCreatedAtLimitsMiddlewareBase(60*60*24*365, 60*60*24*365), // ±1 year
		mocrelay.NewExpirationMiddlewareBase(),

		// Kind blacklist: reject DM-related kinds (Japan Telecommunications Business Act).
		mocrelay.NewKindBlacklistMiddlewareBase([]int64{4, 13, 14, 1059, 10050}),
	)(handler)

	// --- Relay ---

	relay := mocrelay.NewRelay(handler)
	relay.Logger = logger
	relay.Metrics = relayMetrics
	relay.Info = &mocrelay.RelayInfo{
		Name:          "mocrelay-kitchen-sink",
		Description:   "A full-featured Nostr relay example",
		Software:      "https://github.com/high-moctane/mocrelay",
		SupportedNIPs: []int{1, 9, 11, 13, 40, 42, 45, 50, 70},
		Limitation: &mocrelay.RelayLimitation{
			MaxMessageLength: 100_000,
			MaxSubscriptions: 20,
			MaxSubidLength:   256,
			MaxLimit:         500,
			DefaultLimit:     100,
			MaxEventTags:     2000,
			MaxContentLength: 100_000,
			AuthRequired:     true,
		},
		Retention: []*mocrelay.RelayRetention{
			{Kinds: []int64{4, 13, 14, 1059, 10050}, Time: intPtr(0)},
		},
	}

	// --- HTTP multiplexing ---
	//
	// mocrelay.Relay is a standard http.Handler, so it composes with ServeMux.

	mux := http.NewServeMux()
	mux.Handle("/", relay)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "ok")
	})

	logger.Info("starting relay", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func intPtr(n int64) *int64 { return &n }
