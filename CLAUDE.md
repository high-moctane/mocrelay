# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

mocrelay is a Nostr relay implementation written in Go. It implements the Nostr protocol (NIPs) for decentralized social networking, handling WebSocket connections and managing event storage/retrieval.

## Essential Commands

### Building and Running
- `make build` - Build the mocrelay binary in cmd/mocrelay/
- `make run` - Build and run the relay (defaults to localhost:8234)
- `make clean` - Remove built binaries

### Testing and Quality
- `make test` - Run all tests
- `make bench` - Run benchmarks with memory profiling
- `go test ./path/to/package -run TestName` - Run a single test
- `go test -v -run TestName ./...` - Run specific test(s) across all packages

### Code Quality
- `make check` - Run go vet and check formatting (use before commits)
- `make fmt` - Format code with goimports and gofumpt
- `make setup` - Install required tools (lefthook, goimports, gofumpt)

### Git Hooks
The project uses lefthook for pre-commit hooks (configured in lefthook.yml):
- Automatically runs `make fmt` and `make check` before commits
- Run `go tool lefthook install` after setup to enable hooks

## Core Architecture

### The Handler System

mocrelay's architecture is built around a composable **Handler** pattern, similar to http.Handler but for Nostr protocol messages:

```go
type Handler interface {
    ServeNostr(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) error
}
```

**Key Concept**: Handlers process bidirectional message streams. They receive `ClientMsg` (EVENT, REQ, CLOSE, COUNT) from clients and send `ServerMsg` (EVENT, OK, EOSE, CLOSED, NOTICE) back.

### Request Flow

1. **WebSocket Connection** → `Relay.ServeHTTP` (relay.go)
   - Upgrades HTTP connection to WebSocket
   - Creates bidirectional channels (send/recv)
   - Spawns read loop, write loop, and handler goroutines

2. **Message Parsing** → `Relay.serveRead` (relay.go)
   - Parses raw JSON into typed ClientMsg
   - Validates message structure and event signatures
   - Applies rate limiting

3. **Handler Processing** → `Handler.ServeNostr`
   - Routes messages through handler chain
   - Handlers can be composed/wrapped with middleware

4. **Response** → `Relay.serveWriteLoop` (relay.go)
   - Marshals ServerMsg to JSON
   - Sends via WebSocket with timeout protection

### Handler Composition Patterns

#### 1. SimpleHandler (handler.go:42-91)
Wrapper for handlers that process messages one at a time:
```go
type SimpleHandlerBase interface {
    ServeNostrStart(context.Context) (context.Context, error)
    ServeNostrEnd(context.Context) error
    ServeNostrClientMsg(context.Context, ClientMsg) (<-chan ServerMsg, error)
}
```
Use this for straightforward handlers that process one message, return responses.

#### 2. MergeHandler (handler.go:394-694)
Merges multiple handlers to act as one, running them in parallel:
- Broadcasts incoming ClientMsg to all sub-handlers
- Aggregates responses intelligently:
  - **OK messages**: Waits for all handlers, returns first negative or last positive
  - **EVENT messages**: Deduplicates events, respects created_at ordering
  - **EOSE messages**: Waits for all handlers before sending to client
  - **COUNT messages**: Takes maximum count from all handlers

**Example** (cmd/mocrelay/main.go:44-48):
```go
h := mocrelay.NewMergeHandler(
    mocrelay.NewCacheHandler(100),     // Fast in-memory lookup
    mocrelay.NewRouterHandler(100),    // In-memory pub/sub
    sqliteHandler,                      // Persistent storage
)
```

#### 3. RouterHandler (handler.go:132-280)
In-memory pub/sub router for active subscriptions:
- Maintains map of active subscriptions per WebSocket connection
- When EVENT received: broadcasts to matching subscriptions
- When REQ received: stores subscription, immediately sends EOSE
- When CLOSE received: removes subscription

#### 4. CacheHandler (handler.go:282-392)
In-memory LRU cache with event replacement logic:
- Stores up to N most recent events
- Handles replaceable events (kinds 0, 3, 10000-19999)
- Handles parametrized replaceable events (kinds 30000-39999)
- Handles deletion events (kind 5)
- Can dump/restore cache to disk (Dump/Restore methods)

### Middleware System

Middleware wraps handlers to add cross-cutting concerns:

```go
type Middleware func(Handler) Handler
```

Apply middleware by wrapping handlers:
```go
h = middlewareFunc(h)
```

**Built-in Middleware** (handler.go:910-1978):
- `MaxSubscriptionsMiddleware` - Limits concurrent REQ subscriptions per client
- `MaxReqFiltersMiddleware` - Limits filters per REQ message
- `MaxLimitMiddleware` - Caps the limit field in filters
- `MaxEventTagsMiddleware` - Limits tags per event
- `MaxContentLengthMiddleware` - Limits event content length
- `CreatedAtLowerLimitMiddleware` - Rejects events too old
- `CreatedAtUpperLimitMiddleware` - Rejects events too far in future
- `RecvEventUniqueFilterMiddleware` - Deduplicates received events (per session)
- `SendEventUniqueFilterMiddleware` - Deduplicates sent events (per session)
- `RecvEventAllowFilterMiddleware` - Whitelist events by matcher
- `RecvEventDenyFilterMiddleware` - Blacklist events by matcher
- `LoggingMiddleware` - Logs all client/server messages

**NIP11 Auto-Configuration** (handler.go:1051-1081):
`BuildMiddlewareFromNIP11(nip11)` automatically constructs middleware chain from NIP11 limitations.

### Storage Layer: SQLiteHandler

The SQLite handler (handler/sqlite/handler.go) provides persistent storage:

**Key Features**:
- Bulk insert with configurable batching (EventBulkInsertNum, EventBulkInsertDur)
- Automatic WAL checkpoint on bulk insert timer
- Retry logic for failed inserts
- Uses xxHash for efficient indexing
- Schema migrations handled automatically (Migrate function)

**Query Strategy** (handler/sqlite/query.go):
- Converts ReqFilter to SQL with multiple optimization strategies
- Uses indexes on id, pubkey, kind, created_at, and tag keys
- Supports prefix matching for authors/ids

### Event Processing

#### Event Types (message.go:1163-1171)
- **Regular** (default): Stored forever, no replacement
- **Replaceable** (kind 0,3,10000-19999): Latest event per (kind,pubkey) kept
- **Ephemeral** (kind 20000-29999): Not stored persistently
- **Parametrized Replaceable** (kind 30000-39999): Latest per (kind,pubkey,d-tag)

#### Event Cache (event_cache.go)
Sophisticated in-memory cache with multiple indexes:
- Primary storage: `map[eventKey]*Event`
- CreatedAt index: TreeMap for time-ordered queries
- Multi-field indexes: by ID, Author, Kind, Tags for fast filtering
- Handles deletion events (kind 5) that mark other events as deleted

#### Event Matching (event_matcher.go)
Filter events based on ReqFilter criteria:
```go
type EventMatcher interface {
    Match(*Event) bool
}

type EventLimitMatcher interface {
    EventMatcher
    LimitMatch(*Event) bool  // Tracks count toward limit
    Done() bool              // Returns true when limit reached
}
```

### Message Types

All messages are JSON arrays with a string label as first element:

**Client → Server** (message.go:69-473):
- `["EVENT", <event>]` - Submit event for storage
- `["REQ", <sub_id>, <filter>...]` - Subscribe to events
- `["CLOSE", <sub_id>]` - Close subscription
- `["COUNT", <sub_id>, <filter>...]` - Count matching events
- `["AUTH", <event>]` - Authenticate (NIP-42)

**Server → Client** (message.go:734-1161):
- `["EVENT", <sub_id>, <event>]` - Event matching subscription
- `["OK", <event_id>, <accepted>, <message>]` - Accept/reject event
- `["EOSE", <sub_id>]` - End of stored events
- `["CLOSED", <sub_id>, <message>]` - Subscription closed by relay
- `["NOTICE", <message>]` - Human-readable message
- `["COUNT", <sub_id>, {"count": N}]` - Event count response

**Machine-Readable Prefixes** (message.go:35-42):
OK/CLOSED messages may have prefixed reasons:
- `"pow: "` - Insufficient proof of work
- `"duplicate: "` - Event already stored
- `"blocked: "` - Event blocked by policy
- `"rate-limited: "` - Too many requests
- `"invalid: "` - Invalid event format
- `"error: "` - Internal error

### NIP-11 Relay Information

The NIP11 struct (nip11.go) provides relay metadata via HTTP GET with `Accept: application/nostr+json` header:
- Returned as JSON when accessing relay URL via browser
- Describes relay capabilities, limitations, fees
- Can auto-configure middleware via `BuildMiddlewareFromNIP11`

### Prometheus Metrics

The prometheus middleware (middleware/prometheus/prometheus.go) exports metrics:
- `/metrics` endpoint in cmd/mocrelay/main.go
- Tracks message counts, active connections, event counts by kind

## Package Organization

- **Root package** (`github.com/high-moctane/mocrelay`): Core types and handlers
  - `relay.go` - WebSocket management, connection handling
  - `handler.go` - Handler interfaces and built-in handlers
  - `message.go` - Message types and JSON marshaling
  - `event_cache.go` - In-memory event cache with indexes
  - `event_matcher.go` - Event filtering logic
  - `nip11.go` - NIP-11 relay information document
  - `server.go` - ServeMux for routing relay vs NIP-11 requests

- **handler/sqlite** - SQLite storage backend
  - `handler.go` - SQLite handler implementation
  - `query.go` - SQL query generation
  - `insert.go` - Bulk insert logic
  - `migrate.go` - Schema migrations

- **middleware/prometheus** - Metrics collection
  - `prometheus.go` - Prometheus metrics middleware

- **cmd/mocrelay** - Executable
  - `main.go` - Application entry point with example configuration

## Development Workflow

### Typical Handler Development
1. Implement `SimpleHandlerBase` interface for message-by-message processing
2. Or implement `Handler` interface for full control over message streams
3. Add unit tests alongside implementation (e.g., `handler_test.go`)
4. Use table-driven tests for message parsing/validation

### Middleware Development
1. Implement `SimpleMiddlewareBase` for per-message middleware
2. Or create a `Middleware` func for complex flow control
3. Test by wrapping a mock handler and verifying behavior

### Testing Patterns
- Most tests use table-driven approach with subtests
- Event parsing tests are exhaustive (see message_test.go)
- Handler tests often create mock send/recv channels
- Use `t.Parallel()` for independent tests

## Key Design Principles

1. **Composability**: Handlers and middleware can be freely combined
2. **Concurrency**: Each WebSocket connection runs independent goroutines
3. **Type Safety**: Strongly-typed message parsing with validation
4. **Streaming**: Messages processed as streams, not batches
5. **Fail-Safe**: Handlers return errors; relay handles disconnection gracefully
6. **Performance**: Multiple indexes, bulk operations, connection pooling
