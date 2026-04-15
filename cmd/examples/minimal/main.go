// Minimal relay example.
//
// A functional Nostr relay with in-memory storage, real-time event routing,
// and basic middleware. Events are stored in memory and routed between
// connected clients.
//
// Note: InMemoryStorage has no size limit and is not persistent.
// For production use, see the kitchen-sink example which uses PebbleStorage.
//
// Usage:
//
//	go run ./cmd/examples/minimal
//
// Environment variables:
//
//	ADDR - listen address (default ":7447")
package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/high-moctane/mocrelay"
)

func main() {
	addr := ":7447"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Storage: in-memory (no persistence, no size limit).
	storage := mocrelay.NewInMemoryStorage()

	// Router: routes events between connected clients in real-time.
	router := mocrelay.NewRouter()

	// Handler: merge storage (past events) and router (real-time).
	handler := mocrelay.NewMergeHandler(
		mocrelay.NewStorageHandler(storage),
		mocrelay.NewRouterHandler(router),
	)

	// Middleware: basic limits.
	handler = mocrelay.NewSimpleMiddleware(
		mocrelay.NewMaxSubscriptionsMiddlewareBase(20),
		mocrelay.NewMaxSubidLengthMiddlewareBase(256),
		mocrelay.NewMaxLimitMiddlewareBase(500, 100),
		mocrelay.NewMaxEventTagsMiddlewareBase(2000),
		mocrelay.NewMaxContentLengthMiddlewareBase(100_000),
		mocrelay.NewCreatedAtLimitsMiddlewareBase(60*60*24*365, 60*60*24*365), // ±1 year
		mocrelay.NewExpirationMiddlewareBase(),
	)(handler)

	// Relay: serves HTTP/WebSocket.
	relay := mocrelay.NewRelay(handler)
	relay.Logger = logger
	relay.Info = &mocrelay.RelayInfo{
		Name:          "mocrelay-minimal",
		Description:   "A minimal Nostr relay example with in-memory storage",
		Software:      "https://github.com/high-moctane/mocrelay",
		SupportedNIPs: []int{1, 9, 11, 40, 45},
		Limitation: &mocrelay.RelayLimitation{
			MaxMessageLength: 100_000,
			MaxSubscriptions: 20,
			MaxSubidLength:   256,
			MaxLimit:         500,
			DefaultLimit:     100,
			MaxEventTags:     2000,
			MaxContentLength: 100_000,
		},
	}

	logger.Info("starting relay", "addr", addr)
	if err := http.ListenAndServe(addr, relay); err != nil {
		log.Fatal(err)
	}
}
