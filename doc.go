//go:build goexperiment.jsonv2

// Package mocrelay implements a Nostr relay as a composable Go library.
//
// mocrelay provides a middleware-composable architecture for building Nostr relays.
// The core abstraction is the [Handler] interface, which processes a single WebSocket
// connection's lifetime. Handlers can be composed using [Middleware] to add features
// like authentication, rate limiting, and content filtering.
//
// # Architecture
//
// The key types form a layered architecture:
//
//   - [Relay] serves HTTP/WebSocket and manages connection lifecycles
//   - [Handler] processes messages for a single connection
//   - [Middleware] wraps a Handler to add cross-cutting concerns
//   - [Storage] persists and queries events
//   - [Router] routes events between connected clients in real-time
//
// # Handlers
//
// mocrelay provides several built-in handlers:
//
//   - [StorageHandler] wraps a [Storage] to handle EVENT and REQ messages
//   - [RouterHandler] wraps a [Router] to route events between clients
//   - [MergeHandler] runs multiple handlers in parallel and merges responses
//   - [NopHandler] is a minimal handler for testing
//
// Most handlers can be implemented using [SimpleHandlerBase], which provides
// a simpler message-at-a-time interface instead of managing channels directly.
//
// # Middleware
//
// Middleware is built on the [SimpleMiddlewareBase] interface. Multiple middleware
// bases are composed into a single pipeline via [NewSimpleMiddleware]:
//
//	handler := NewSimpleMiddleware(
//	    NewMaxSubscriptionsMiddlewareBase(20),
//	    NewMaxLimitMiddlewareBase(500, 100),
//	    NewKindBlacklistMiddlewareBase([]int64{4, 1059}),
//	)(innerHandler)
//
// Built-in middleware corresponds to NIP-11 limitation and retention fields,
// providing a declarative way to configure relay policies.
//
// # Storage
//
// The [Storage] interface uses Go iterators ([iter.Seq]) for streaming query results:
//
//	events, errFn, closeFn := storage.Query(ctx, filters)
//	defer closeFn()
//	for event := range events {
//	    // process event
//	}
//	if err := errFn(); err != nil {
//	    // handle error
//	}
//
// [InMemoryStorage] is provided for testing. For production use, see [PebbleStorage]
// which provides persistent storage using CockroachDB's Pebble LSM-tree engine.
//
// # Typical usage
//
// A typical relay combines storage, routing, middleware, and metrics:
//
//	storage, _ := NewPebbleStorage("/path/to/db", nil)
//	defer storage.Close()
//
//	router := NewRouter()
//	handler := NewMergeHandler(
//	    NewStorageHandler(storage),
//	    NewRouterHandler(router),
//	)
//
//	handler = NewSimpleMiddleware(
//	    NewMaxSubscriptionsMiddlewareBase(20),
//	    NewMaxLimitMiddlewareBase(500, 100),
//	)(handler)
//
//	relay := NewRelay(handler)
//	http.ListenAndServe(":7447", relay)
package mocrelay
