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
//   - [NewStorageHandler] wraps a [Storage] to handle EVENT and REQ messages
//   - [NewRouterHandler] wraps a [Router] to route events between clients
//   - [NewMergeHandler] runs multiple handlers in parallel and merges responses
//   - [NewNopHandler] is a minimal handler for testing
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
//	    NewKindDenylistMiddlewareBase([]int64{4, 1059}),
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
//	router := NewRouter(nil)
//	handler := NewMergeHandler(
//	    []Handler{
//	        NewStorageHandler(storage),
//	        NewRouterHandler(router),
//	    },
//	    nil,
//	)
//
//	handler = NewSimpleMiddleware(
//	    NewMaxSubscriptionsMiddlewareBase(20),
//	    NewMaxLimitMiddlewareBase(500, 100),
//	)(handler)
//
//	relay := NewRelay(handler, nil)
//	http.ListenAndServe(":7447", relay)
package mocrelay
