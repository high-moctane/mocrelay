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

	"github.com/blevesearch/bleve/v2"
	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
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
	//
	// mocrelay exposes a single `Registerer` field on each *Options struct.
	// Setting it causes that component to register its metrics with the
	// supplied registry; leaving it nil disables instrumentation entirely
	// for that component (no collectors are created and every instrument
	// site no-ops). Pass the same registry to every component to collect
	// everything under one Prometheus registry.

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())

	// --- Storage layer ---
	//
	// mocrelay does not open or close the underlying Pebble / Bleve
	// databases: the caller owns them. This lets callers pick their own
	// pebble.Options (cache size, bloom filter, event listeners, ...),
	// expose db.Metrics() to Prometheus themselves, and pin their own
	// Pebble / Bleve versions independently of mocrelay's go.mod.

	// Pebble: persistent LSM-tree storage.
	cache := pebble.NewCache(64 << 20) // 64 MB block cache
	defer cache.Unref()
	bloomOpts := pebble.LevelOptions{
		FilterPolicy: bloom.FilterPolicy(10),
		FilterType:   pebble.TableFilter,
	}
	db, err := pebble.Open(filepath.Join(dataDir, "pebble"), &pebble.Options{
		Cache: cache,
		// Apply the bloom filter to every level (Pebble uses 7 by default).
		Levels: []pebble.LevelOptions{
			bloomOpts, bloomOpts, bloomOpts, bloomOpts,
			bloomOpts, bloomOpts, bloomOpts,
		},
	})
	if err != nil {
		log.Fatalf("pebble: %v", err)
	}
	defer db.Close()
	pebbleStorage := mocrelay.NewPebbleStorage(db, nil)

	// Bleve: full-text search with CJK support (NIP-50).
	blevePath := filepath.Join(dataDir, "bleve")
	idx, err := bleve.Open(blevePath)
	if err == bleve.ErrorIndexPathDoesNotExist {
		idx, err = bleve.New(blevePath, mocrelay.BuildIndexMapping())
	}
	if err != nil {
		log.Fatalf("bleve: %v", err)
	}
	defer idx.Close()
	bleveIndex := mocrelay.NewBleveIndex(idx, nil)

	// Composite: Pebble (source of truth) + Bleve (search).
	compositeStorage := mocrelay.NewCompositeStorage(pebbleStorage, bleveIndex, &mocrelay.CompositeStorageOptions{
		Registerer: reg,
	})

	// Metrics: wrap storage with Prometheus instrumentation.
	storage := mocrelay.NewMetricsStorage(compositeStorage, reg)

	// --- Routing layer ---

	router := mocrelay.NewRouter(&mocrelay.RouterOptions{
		Registerer: reg,
	})

	// --- Handler ---

	handler := mocrelay.NewMergeHandler(
		[]mocrelay.Handler{
			mocrelay.NewStorageHandler(storage, nil),
			mocrelay.NewRouterHandler(router),
		},
		nil, // default options
	)

	// --- Middleware pipeline (outermost first) ---

	handler = mocrelay.NewSimpleMiddleware(
		// Auth (NIP-42): challenge/response authentication.
		// Auth metrics are constructed by Relay from RelayOptions.Registerer
		// (below) and injected into the request context; the middleware
		// reads them internally.
		mocrelay.NewAuthMiddlewareBase("wss://relay.example.com/", nil),

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

		// Kind denylist: reject DM-related kinds (Japan Telecommunications Business Act).
		mocrelay.NewKindDenylistMiddlewareBase([]int64{4, 13, 14, 1059, 10050}),
	)(handler)

	// --- Relay ---

	relay := mocrelay.NewRelay(handler, &mocrelay.RelayOptions{
		Logger:     logger,
		Registerer: reg,
		Info: &mocrelay.RelayInfo{
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
		},
	})

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
