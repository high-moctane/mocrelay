# CLAUDE.md

mocrelay - A Nostr relay implementation in Go.

## Project Status

Production-ready. Running on salmon (KAGOYA VPS).

## Commands

```bash
go build ./...     # Build
go test ./...      # Test
go tool lefthook install  # Install git hooks
```

## Development Principles

- **LLM-assisted development**: Consistency and quality through collaboration
- **Start minimal, grow incrementally**: Build the smallest working thing first
- **Discuss as we go**: Design decisions made through conversation

### Development Workflow

**Handler/Middleware development loop**:

```
Implement → Test → Update CLAUDE.md → Commit
```

One step at a time.

**Implementation policy**:
- Prefer `SimpleHandlerBase` / `SimpleMiddlewareBase` as base
- Makes testing easier, hides async complexity
- Only implement `Handler` directly for 1:N transformations

### Channel Patterns (goroutine leak prevention)

**Basic principle**: Always check `ctx.Done()` on channel send/recv to allow goroutine termination.

#### ✅ Safe Patterns

```go
// send: with ctx.Done()
select {
case <-ctx.Done():
    return ctx.Err()
case ch <- msg:
}

// recv: with ctx.Done()
select {
case <-ctx.Done():
    return ctx.Err()
case msg, ok := <-ch:
    if !ok {
        return nil // channel closed
    }
}

// best effort send: non-blocking (for Router broadcast etc.)
select {
case ch <- msg:
default:
    // drop
}
```

#### ⚠️ Cases Requiring Attention

**Buffered channel send**:
- `ch := make(chan T, 1)` - first send won't block
- If multiple sends possible, use `select` + `ctx.Done()`

**Unbuffered channel send**:
- Always use `select` + `ctx.Done()` (may block)

**Pebble and other I/O operations**:
- Pebble API doesn't respect ctx (Go io limitation)
- Usually completes in milliseconds, acceptable
- Long blocking indicates bigger problems

### Memory Leak Prevention (State Management)

**Problem**: If a Handler holds per-subscription state and the connection drops without CLOSE, state may persist.

**Countermeasures**:

1. **Cleanup on connection end**:
   - When `ServeNostr()` returns, Handler state is GC'd
   - Abrupt disconnection is fine

2. **Cleanup on CLOSE message**:
   - Handles long connections creating many subscriptions then abandoning them
   - MergeHandler: `closeSubscription(subID)` deletes state
   - Router: `Unsubscribe(connID, subID)` removes subscription

**Handlers with state**:

| Handler | State | Cleanup |
|---------|-------|---------|
| **MergeHandler** | `pendingEOSEs`, `completedSubs`, `limitReachedSub` | Deleted on CLOSE |
| **Router** | `connections[connID].subscriptions` | Deleted on CLOSE; on disconnect, Unregister removes entire connection |

**Stateless Handlers** (no concern):
- `StorageHandler`: Completes per REQ, no state
- `NopHandler`: No state

### Design Decisions

#### Handler Interface

- Middleware-composable architecture (key feature of mocrelay)
- Signature and error handling under discussion:
  - **Error levels**: Relay-level (fatal), Connection-level (disconnect), Handler-level (OK false / NOTICE)
  - Current thinking: distinguish these three levels clearly

#### Message Types

- `ServerMsg` / `ClientMsg` as **structs** (not interfaces)
  - Simpler, more Go-like
  - Interface would require too many methods

#### Event Type

- `kind` field: use `int64` (NIP says 0-65535 but don't trust it)

#### JSON Marshal/Unmarshal

- Hand-written implementation required
- `encoding/json` insufficient for Nostr's strict requirements
- No extra fields allowed, specific escape rules

#### Dependencies

- **secp256k1**: `github.com/btcsuite/btcd/btcec/v2/schnorr` (pure Go, BIP-340)
- Minimize external dependencies

### NIP-01 Key Points

#### Event Structure

```json
{
  "id": "<32-bytes hex SHA256>",
  "pubkey": "<32-bytes hex public key>",
  "created_at": "<unix timestamp>",
  "kind": "<0-65535>",
  "tags": [["e", "..."], ["p", "..."], ...],
  "content": "<string>",
  "sig": "<64-bytes hex signature>"
}
```

#### Kind Categories

| Range | Type | Storage Rule |
|-------|------|--------------|
| 1, 2, 4-44, 1000-9999 | regular | Store all |
| 0, 3, 10000-19999 | replaceable | Keep latest per (pubkey, kind) |
| 20000-29999 | ephemeral | Don't store |
| 30000-39999 | addressable | Keep latest per (pubkey, kind, d-tag) |

#### Client → Relay

- `["EVENT", <event>]` - Submit event
- `["REQ", <sub_id>, <filter>...]` - Subscribe
- `["CLOSE", <sub_id>]` - Unsubscribe

#### Relay → Client

- `["EVENT", <sub_id>, <event>]` - Send event
- `["OK", <event_id>, <bool>, <message>]` - Event result
- `["EOSE", <sub_id>]` - End of stored events
- `["CLOSED", <sub_id>, <message>]` - Subscription closed
- `["NOTICE", <message>]` - Human-readable message

#### Filter

- Multiple conditions = AND
- Multiple filters = OR
- `limit` applies to initial query only, return `created_at DESC`
- Single-letter tags (a-z, A-Z) should be indexed

#### OK/CLOSED Prefixes

`duplicate`, `pow`, `blocked`, `rate-limited`, `invalid`, `restricted`, `mute`, `error`

## Common LLM Mistakes (Training Data vs Current NIP-01)

⚠️ Points where LLM training data (up to ~Jan 2025) may conflict with current NIP-01.

### Filter uses exact match (not prefix match)

❌ Wrong: `{"ids": ["abcdef"]}` does prefix match
✅ Correct: **64-character lowercase hex only**

> "The `ids`, `authors`, `#e` and `#p` filter lists MUST contain exact 64-character lowercase hex values."

mocrelay uses exact match (for DB index efficiency).

### limit scope

- **Applies to initial query only** (not to real-time updates)
- Sort order: `created_at DESC`, tie-breaker `id ASC` (lexical order)

### e tag 4th field

`["e", <event_id>, <relay_url>, <author_pubkey>]`

4th field can contain author's pubkey (optional).

### a tag trailing colon

- addressable: `30023:pubkey:identifier`
- replaceable: `10000:pubkey:` ← **trailing colon required**

### Only first tag value is indexed

> "Only the first value in any given tag is indexed."

`["e", "id1", "relay", "author"]` → only `id1` is filterable.

## Architecture

### Handler Overview

| Handler | Description |
|---------|-------------|
| `NopHandler` | Void relay. EVENT→OK, REQ→EOSE only |
| `RouterHandler` | Routes events between clients. Centralized Router manages subscriptions |
| `StorageHandler` ✅ | Wraps Storage. EVENT→Store→OK, REQ→Query→EVENTs+EOSE |
| `MergeHandler` ✅ | Runs multiple Handlers in parallel, merges responses |

### Handler/Middleware (NIP-11 based)

Provides Handler/Middleware corresponding to NIP-11 `limitation` / `retention` fields.
This is mocrelay's main value proposition.

#### Tier 1: Basic Limitations (NIP-01 only) ✅ Complete

| Middleware | NIP-11 Field | Description |
|------------|--------------|-------------|
| `MaxSubscriptions` ✅ | `limitation.max_subscriptions` | Per-connection subscription limit |
| `MaxSubidLength` ✅ | `limitation.max_subid_length` | Subscription ID length limit |
| `MaxLimit` ✅ | `limitation.max_limit`, `default_limit` | Clamp limit + default value |
| `MaxEventTags` ✅ | `limitation.max_event_tags` | Tag count limit |
| `MaxContentLength` ✅ | `limitation.max_content_length` | Content length limit (Unicode) |
| `CreatedAtLimits` ✅ | `limitation.created_at_lower/upper_limit` | created_at range check |
| `KindBlacklist` ✅ | `retention` (time=0) | Reject specific kinds (DMs etc.) |
| `RestrictedWrites` ✅ | `limitation.restricted_writes` | Pubkey whitelist/blacklist |

#### Tier 2: WebSocket/HTTP Level ✅ Complete

| Feature | NIP-11 Field | Description |
|---------|--------------|-------------|
| `MaxMessageLength` ✅ | `limitation.max_message_length` | WebSocket message size limit (`relay.go:78-79`) |
| `NIP11Handler` ✅ | - | HTTP handler returning NIP-11 JSON (`relay.go:244-272`) |

#### Tier 3: Requires Other NIPs ✅ Complete

| Middleware | NIP-11 Field | Depends On |
|------------|--------------|------------|
| `MinPowDifficulty` ✅ | `limitation.min_pow_difficulty` | NIP-13 |
| `AuthRequired` ✅ | `limitation.auth_required` | NIP-42 |

#### Other NIP Support ✅

| Middleware | NIP | Description |
|------------|-----|-------------|
| `ExpirationMiddleware` ✅ | NIP-40 | Reject/drop expired events via `expiration` tag |
| `ProtectedEventsMiddleware` ✅ | NIP-70 | Prevent republishing `["-"]` tagged events (requires NIP-42 AUTH) |
| `CompositeStorage` ✅ | NIP-50 | Full-text search via Bleve with CJK support |

#### Future NIP Implementation Priority

**Special features (as needed)**:
- NIP-29: Groups (complex, group management/moderation)
- NIP-77: Negentropy (relay synchronization)
- NIP-86: Management API (JSON-RPC over HTTP, for operations)

**Store only (no special processing)**:
- NIP-22: Comment (kind 1111)
- NIP-28: Public Chat (kind 40-44, note kind 41 replaceable-like behavior)

See `docs/nips/` for detailed NIP analysis.

#### Paid Relays

`payment_required` is not provided as mocrelay middleware.

**Reason**: NIP-11 only specifies fee disclosure, actual payment protocol is not standardized.

**How to implement paid relay**:
- Use `RestrictedWrites` to whitelist paid pubkeys
- Integrate with external payment systems (Lightning, Stripe, etc.)
- Publish fees via NIP-11 `fees` field

#### Japan Telecommunications Business Act Compliance

Use `KindBlacklist` to reject these DM-related kinds:
- kind 4 (legacy DM)
- kind 13 (Seal wrapper)
- kind 14 (Chat Messages)
- kind 1059 (Gift Wrap)
- kind 10050 (DM relay list)

Publish as `time: 0` in NIP-11 `retention` to notify clients in advance.

### Router Design

- **Centralized**: Router manages all connections and subscriptions
- **Hierarchical**: Connection ID (server-generated) → Subscription ID (client-provided)
- **Best-effort send**: Drop if channel full (deadlock prevention)

```go
// Always use this pattern for sending
select {
case ch <- msg:
    // sent
default:
    // full, drop
}
```

### MergeHandler Design ✅

Runs multiple Handlers in parallel and merges responses.

**Typical usage**:
```go
handler := NewMergeHandler(
    NewStorageHandler(storage),  // Fetch past events
    NewRouterHandler(router),    // Real-time delivery
)
```

**Merge rules**:

| Message | Rule |
|---------|------|
| **OK** | Wait for all handlers, merge (any rejection = rejection, indicates failure) |
| **EOSE** | Wait for all handlers' EOSE before sending |
| **EVENT (before EOSE)** | Deduplicate + drop events that break sort order |
| **EVENT (after handler EOSE)** | Pass through from that handler (real-time events) |
| **COUNT** | Take max across all handlers |

**limit handling**:
- **Uses first filter's limit** (mocrelay's stance)
- Return at most limit events per REQ, then EOSE
- Pass same limit to child handlers
- **Drop EVENTs after limit reached** (including real-time)

**Sort drop rules**:
- Assumes child handlers return sorted responses
- Stream in arrival order, drop events that break sort
- Sort order: `created_at DESC`, tie-breaker `id ASC` (lexical order)

```go
// Check if event breaks sort order
if newEvent.CreatedAt > lastSentCreatedAt {
    // drop
} else if newEvent.CreatedAt == lastSentCreatedAt && newEvent.ID > lastSentID {
    // drop
} else {
    // send
}
```

**Design decisions**:
- Use `sync.Mutex` (not channel-based mutex)
- Cannot use SimpleHandlerBase (requires 1:N transformation)

**Considerations**:
- If a handler never sends EOSE, session maps grow indefinitely
- Countermeasures: timeout, CLOSE cleanup, subscription limits

### Storage Interface ✅

**iter.Seq pattern** (Go 1.23+):

```go
type Storage interface {
    Store(ctx context.Context, event *Event) (bool, error)
    Query(ctx context.Context, filters []*ReqFilter) (events iter.Seq[*Event], err func() error, close func() error)
}

// Future optimization: CountableStorage for efficient counting
// Currently COUNT uses Query + iteration, which is sufficient for typical usage.
```

**Usage**:
```go
events, errFn, closeFn := storage.Query(ctx, filters)
defer closeFn()

for event := range events {
    ch <- NewServerEventMsg(subID, event)
}

if err := errFn(); err != nil {
    return err
}
```

**Benefits**:
- **Intuitive for-range**: Clean `for event := range events`
- **Streaming response**: Return to client before all events ready
- **PebbleStorage uses Snapshot**: Query doesn't block Write (MVCC)
- **Simple interface**: Only Store and Query methods

**Why Delete / DeleteByAddr were removed**:
- Kind 5 processing is complete within `Store`
- NIP-86 (management API) has no specific event deletion API
- Not needed as external API

**StorageHandler responsibilities**:
- EVENT → Store and return OK
- REQ → Query and return EVENTs + EOSE (streaming)
- COUNT → Query and count (iterating over results)
- **Does not manage subscriptions** (RouterHandler's job)
- Role ends after sending EOSE for a REQ

**InMemoryStorage**:
- Slice + O(n) full scan (simple implementation)
- Well-tested, safe to optimize later
- NIP-09 compliant (timestamp check, kind 5 deletion disabled)

**Persistence**: Pebble (decided)
- **github.com/cockroachdb/pebble**: LSM-tree based KV store by CockroachDB
- Pure Go (no cgo), embedded, simple deployment
- **Snapshot** for lock-free reads (MVCC)

**Selection rationale**:
- PostgreSQL: Full-text search (pgroonga) attractive, but requires external process
- DuckDB: OLAP-oriented, weak at real-time writes
- SQLite: cgo issues, partitioning difficult
- **Pebble**: Pure Go, excellent streaming, fits Nostr's append workload

**Key schema (binary fixed-length)**:
```
Primary data:
[0x01][event_id:32]  →  event_json                    (33 bytes)

Indexes (empty value):
[0x02][inverted_ts:8][id:32]                          (41 bytes)
[0x03][pubkey:32][inverted_ts:8][id:32]               (73 bytes)
[0x04][kind:8][inverted_ts:8][id:32]                  (49 bytes)
[0x05][tag_name:1][tag_hash:32][inverted_ts:8][id:32] (74 bytes)

Replaceable/Addressable specific (value is event_id:32):
[0x06][addr_hash:32]  → [event_id:32]  (33 bytes key)

Deletion markers:
[0x08][event_id:32]   → [pubkey:32][created_at:8]  (33 bytes key, 40 bytes value)
[0x09][addr_hash:32]  → [pubkey:32][created_at:8]  (33 bytes key, 40 bytes value)
```

- **addr_hash**: `SHA256("kind:pubkey:d-tag")` unified (replaceable uses empty d-tag)

- **Binary fixed-length**: Simple parsing, predictable key length
- **inverted_ts**: `math.MaxInt64 - created_at` (descending order in lexical sort)
- **tag_hash**: SHA256(tag_value) fixed 32 bytes (collision negligible)
- **No compound indexes**: Use Multi-Cursor Merge, add later if needed

**Full-text search (NIP-50)**:
- Not supported in Pebble
- Use separate search engine (Bleve, Meilisearch, etc.) via MergeHandler if needed

**PebbleStorageOptions**:
```go
type PebbleStorageOptions struct {
    CacheSize int64  // Block cache (default 8MB, production 64-256MB recommended)
    FS        vfs.FS // For testing (vfs.NewMem())
}
```

**Fixed settings (no need to change)**:
- Bloom filter: 10 bits/key (~1% false positive), Table-level filter on all levels
- MemTableSize: 4MB (~4000 events, sufficient for mocvps scale)
- Other Pebble options: defaults are fine, add as needed

**PebbleStorage Close**:
- Caller responsible for `Close()` ("creator closes" principle)
- Required to properly close WAL and files

```go
storage, _ := NewPebbleStorage("/path/to/db", nil)
defer storage.Close()  // ← Don't forget!

handler := NewStorageHandler(storage)
relay := NewRelay(handler)
```

**Differential Testing**:
- `storage_differential_test.go` verifies InMemory and Pebble behavior match
- Seed-based random tests (reproducible)
- Also verified at StorageHandler level (EVENT→OK, REQ→EVENT*+EOSE)

### Writing Tests

**Use `testing/synctest` for async tests** (Go 1.25+)

```go
synctest.Test(t, func(t *testing.T) {
    // Inside is an isolated "bubble" environment
    // - fake clock (time advances automatically)
    // - synctest.Wait() waits until all goroutines block

    router := NewRouter()
    sendCh := make(chan *ServerMsg, 10)
    connID := router.Register(sendCh)

    router.Subscribe(connID, "sub1", filters)
    router.Broadcast(event)

    synctest.Wait() // Wait for all goroutines to settle

    // Assert here
})
```

**Note**: Goroutines blocked on network I/O are not covered by synctest. Use for channel-based tests.

## Documentation

- **docs/nips/**: NIPs relays should implement (categorized by MUST/SHOULD/MAY)
- **docs/encoding-json-v2.md**: Go 1.25 `encoding/json/v2` notes (requires `GOEXPERIMENT=jsonv2`)

## NIP Support

- NIP-01: Basic protocol ✅
- NIP-09: Event Deletion ✅
- NIP-11: Relay Information Document ✅
- NIP-13: Proof of Work ✅
- NIP-40: Expiration Timestamp ✅
- NIP-42: Authentication ✅
- NIP-45: Event Counts ✅
- NIP-50: Search Capability ✅ (Bleve + CJK)
- NIP-70: Protected Events ✅
