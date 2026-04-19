# CLAUDE.md

mocrelay - A Nostr relay implementation in Go.

## Project Status

Production-ready.

## Commands

```bash
go build ./...     # Build
go test ./...      # Test
lefthook install  # Install git hooks (optional; requires lefthook binary)
```

## Development Principles

- **LLM-assisted development**: Consistency and quality through collaboration
- **Start minimal, grow incrementally**: Build the smallest working thing first
- **Discuss as we go**: Design decisions made through conversation
- **English throughout**: All commit messages, code comments, documentation, and CLAUDE.md in English

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
| **MergeHandler** | `pendingOKs`, `pendingEOSEs`, `pendingCounts`, `completedSubs` | Per-subscription state cleared on CLOSE; lost-handler pendings advanced via `handlerClosed`. |
| **Router** | `connections[connID].subscriptions` | Deleted on CLOSE; on disconnect, Unregister removes entire connection |

**Stateless Handlers** (no concern):
- `StorageHandler`: Completes per REQ, no state
- `NopHandler`: No state

### Logging

**Context-based logger propagation**:

```go
// logger.go: package-level API
ContextWithLogger(ctx, logger) context.Context
LoggerFromContext(ctx) *slog.Logger  // returns slog.Default() if not set
```

**Design**:
- `Relay.Logger *slog.Logger`: nil → `slog.Default()` fallback (net/http style)
- Relay injects connID-tagged logger into ctx at connection start: `r.logger().With("conn_id", connID)`
- All Handlers use `LoggerFromContext(ctx)` — connID propagates automatically
- Users with zerolog/zap can wrap their logger as `*slog.Logger` via bridge adapters

**Usage in Handlers**:
```go
// ✅ Correct: use LoggerFromContext
LoggerFromContext(ctx).WarnContext(ctx, "query error", "error", err)

// ❌ Wrong: direct slog call (bypasses context logger)
slog.WarnContext(ctx, "query error", "error", err)
```

**Log level policy**:

| Level | What goes here | Examples |
|---|---|---|
| **Error** | Data loss / corruption; operator must act now. | Storage.Store failure, unrecoverable I/O. |
| **Warn** | Unexpected / externally-caused failures; individually tolerable but worth watching. | WebSocket parse / verify failure, MergeHandler lost child (timeout / early close), child handler returned error, Shutdown deadline exceeded. |
| **Info** | Structural lifecycle events — one per connection or per relay, not per message. | Connection start / end, PebbleStorage / BleveIndex open / close, Relay shutdown progress. |
| **Debug** | Hot-path events: every accepted EVENT, every REQ, every middleware rejection, every dropped broadcast. Off in production. | `logRejection` output, routerHandler per-message events, MergeHandler EVENT drop. |

Rule of thumb: if the log fires "at Info or higher and would be noisy in
production under normal load", it is at the wrong level.

**Middleware rejection log helper** (`logRejection`):

Every middleware in mocrelay that drops / rejects a client message calls the
internal `logRejection` helper so operators can find all rejections with a
single grep:

```
grep "message rejected" relay.log
```

Emitted keys are uniform: `middleware`, `reason`, and message-specific
extras (e.g. `event_id`, `pubkey`, `sub_id`, `kind`). The `reason` string is
stable and intentionally aligned with the corresponding Prometheus
`rejections_total{reason=…}` label where one exists, so logs and metrics are
cross-indexable.

Rejections are **Debug**, not Warn: they are an expected outcome of normal
middleware policy (`MaxEventTags` dropping oversized events, `AuthRequired`
rejecting unauthenticated REQs, …). Enable Debug when investigating why a
specific client is being dropped; rely on metrics day-to-day.

### Metrics

**Observability stance**: metrics and logs are complementary. Logs (from
`logRejection` etc.) are the per-event narrative — searchable, costly to
retain long-term. Metrics are the aggregated time series — cheap,
cross-indexable with log `reason` labels. Always instrument both.

#### Framing: RED and USE

Two methods cover different questions and are used together, not
interchangeably:

- **RED method** (Rate / Errors / Duration) — applied to **request-driven
  paths**: EVENT handling, REQ handling, Store, Query, middleware, Bleve
  search. Answers "is the client experience OK?"
- **USE method** (Utilization / Saturation / Errors) — applied to
  **resources**: connections, subscriptions, send buffers, Pebble WAL,
  goroutines. Answers "is anything inside starting to choke?"

A request path can be slow (RED-Duration bad) *because* a resource is
saturated (USE-Saturation bad). You need both axes to see causation, so
add the RED pair and the USE pair together.

#### Cardinality rules

| Rule | Labels |
|---|---|
| **Never** | `pubkey`, `conn_id`, `sub_id`, `ip`, `event_id`, free-form message strings |
| **Careful** | `kind` — bounded in spec (0-65535) but mocrelay accepts arbitrary `int64`. A hostile client can explode the series set. See Known Concerns below. |
| **Safe** | Stable enum strings: `type` (EVENT / REQ / …), `reason`, `result`, `middleware` name |

Rule of thumb: if you cannot enumerate the full label-value set at code
review time, the label is probably too cardinal.

#### Layer convention — count at both ends and subtract

- **Outer (Relay / routerHandler)**: count everything *received* on the
  wire, before middleware. This is the rate the relay is being offered.
- **Inner (Storage / MergeHandler accepted path)**: count everything
  *accepted*. This is what actually took effect.
- **Difference = amount rejected by middleware.** Each middleware emits
  its own `<middleware>_rejections_total{reason}` counter (one metric
  name per middleware, not a shared `{middleware, reason}` dimension),
  so the breakdown is visible by cause while each middleware's metric
  surface stays independent.

#### Log ↔ metric cross-index

Every middleware that calls `logRejection` also increments its own
rejection counter, carrying the **same `reason` string** as a label. The
shared key is the `reason` value, not a unified metric name:

```
grep reason=max_content_length relay.log
rate({__name__=~"mocrelay_.*_rejections_total", reason="max_content_length"}[5m])
```

should land on the same events. Today only `AuthMiddleware` is wired —
extending to all middleware is the primary follow-up.

#### Instrument choice

| Instrument | When | Hot-path note |
|---|---|---|
| **Counter** | Any monotonic count (requests, errors, rejections, drops, bytes in / out) | Lock-free, cheap; fine on every message |
| **Gauge** | Current level (connections, subscriptions, buffer depth) | Must have matched Inc / Dec (`defer`, cleanup, or `Set`) or it drifts upward forever |
| **Histogram** | Latency / size distributions where the tail matters | More expensive than Counter; acceptable per REQ / per Store. For per-EVENT hot paths, audit buckets first |

#### Nil-safe injection

Metrics are optional, matching the philosophy of `Relay.Logger`. Each of
`RelayMetrics`, `RouterMetrics`, `AuthMetrics`, `StorageMetrics` is passed
through `*Options`; `nil` means "don't measure, don't allocate." Call
sites guard with a nil check (or a no-op shim) so the instrument-free
build path has zero runtime cost and no Prometheus dependency surface.
Just as `Logger == nil` falls back to `slog.Default()`, `Metrics == nil`
falls back to silence.

#### Coverage by layer (drives follow-up PRs)

| Layer | Kind | Existing | Gaps (future PRs) |
|---|---|---|---|
| **Relay (WS conn)** | RED-R | `ConnectionsTotal`, `MessagesReceived{type}`, `MessagesSent{type}`, `EventsReceived{kind}` | — |
| | RED-E | — | `ws_parse_errors_total`, `ws_write_errors_total` |
| | RED-D | — | `ws_write_duration_seconds` (diagnose write timeouts) |
| | USE-Sat | `ConnectionsCurrent` | `send_buf_full_drops_total`, `write_timeouts_total` |
| **Router** | USE-Sat | `MessagesDropped` | `subscriptions_current` (Gauge) |
| **Middleware: Auth** | RED-R+E | `AuthTotal{result}`, `RejectionsTotal{reason}`, `AuthenticatedConnectionsCurrent` | — |
| **Middleware: others (11)** | RED-E | — | Per-middleware `<name>_rejections_total{reason}`, aligned with `logRejection` |
| **StorageHandler** | RED-R+D | `EventsStored{kind, stored}`, `Store / QueryDuration` | `store_errors_total`, `query_errors_total` |
| **MergeHandler** | USE-Sat+E | — | `lost_children_total`, `broadcast_timeouts_total`, `event_sort_drops_total` |
| **Pebble (internals)** | USE | — | Expose `pebble.Metrics()` — L0 files, compactions, WAL bytes |
| **Bleve** | RED-R+D+E | — | `search_total`, `search_duration_seconds`, `search_errors_total` |

#### Known concerns (decide before v0.x freeze)

1. **`kind` label explosion** — **implemented: known-kind allowlist +
   NIP-01 range buckets.** `EventsReceived{kind}` and
   `EventsStored{kind, stored}` accept arbitrary `int64`; a hostile or
   buggy client sending a unique kind per message would grow the series
   set without bound. Mitigation lives in `kind_label.go` via the
   `kindLabel(int64) string` helper:
   - Known kinds return their decimal form: 0, 1, 3, 4, 5, 6, 7, 13, 14,
     40-44, 1059, 1111, 1984, 9734, 9735, 10002, 10050, 30023.
   - Unknown kinds are bucketed by NIP-01 range — `regular_other`
     (1..9999), `replaceable_other` (10000..19999), `ephemeral`
     (20000..29999), `addressable_other` (30000..39999), `other` for
     everything else (including negatives and values ≥ 40000).

   Total cardinality is bounded at `len(knownKinds) + 5` — currently
   28 — regardless of client behaviour, while per-kind fidelity is
   preserved for the common cases (kind 1 / 7 in particular are worth
   watching). Alternatives considered and rejected before landing on
   this hybrid: NIP-01 range buckets alone (loses per-kind detail), and
   dropping the label entirely (recoverable from the event log but
   inconvenient for dashboards / alerts). Adjust the allowlist based on
   operational experience.

2. **Per-middleware counter scope** — **decided: stay per-middleware.**
   An earlier draft proposed promoting `RejectionsTotal` from
   `AuthMetrics` to a relay-wide `mocrelay_rejections_total{middleware,
   reason}`. We chose instead to keep each middleware's rejection
   counter as its own metric (e.g. `mocrelay_auth_rejections_total`,
   `mocrelay_max_content_length_rejections_total`, …). Rationale:
   (a) it matches the existing per-middleware `*Metrics` / `*Options`
   surface, so nil-safe injection stays local to each middleware;
   (b) middlewares remain independent — adding or removing one doesn't
   touch the others' label spaces; (c) each middleware's `reason` enum
   stays small and closed, with no risk of colliding with another
   middleware's vocabulary. The `{middleware, reason}` cross-product
   view is still recoverable at query time via, e.g.,
   `sum by(reason)({__name__=~"mocrelay_.*_rejections_total"})` —
   PromQL is expressive enough that storage-layer independence doesn't
   cost us aggregation at the query layer.

3. **Pebble / Bleve internals**: `pebble.Metrics()` returns rich
   saturation signals (L0 files, pending compactions, WAL bytes) that
   are not exposed today. The usual pattern is a periodic collector
   goroutine reading `Metrics()` every N seconds into Gauges. Out of
   scope for the initial policy PR; planned as a later follow-up.

### Design Decisions

#### Constructor API

All public constructors in mocrelay follow a single shape:

```go
NewX(required..., opts *XOptions) *X     // or (*X, error) for I/O
```

Rules:

- **Positional args are required** (e.g. `handler`, `relayURL`, `path`).
  If a value has no sensible zero default, it goes here.
- **Optional config lives in `*XOptions`** — one Options struct per type,
  right next to its constructor. `opts == nil` is always valid and means
  "all defaults."
- **Struct fields are unexported.** Post-construction mutation is not a
  supported API; if a knob matters later, promote it into Options.
- **Defaults are applied once, inside the constructor.** Runtime code
  does not re-check for zero values. Writing `if x == 0 { x = default }`
  at call sites is a smell — it means the constructor did not finish
  its job.
- **Metrics are a field of Options**, not a separate positional arg or
  a post-construction field. Uniform with every other optional setting.

This pattern is used by `NewRelay`, `NewRouter`, `NewAuthMiddlewareBase`,
`NewPebbleStorage`, and `NewBleveIndex`. New Handlers / Middlewares /
Storages added to mocrelay should follow the same shape so third-party
code composing with mocrelay has a predictable surface.

Exceptions:
- `MetricsStorage` uses the decorator pattern (`NewMetricsStorage(storage, metrics)`)
  because `Storage` is an interface with multiple implementations
  (InMemory / Pebble / Composite) and a decorator is the natural way
  to inject cross-cutting behavior.
- Middlewares whose *entire* configuration is required (e.g.
  `NewMaxLimitMiddlewareBase(maxLimit, defaultLimit int64)`) skip the
  Options struct because there is nothing optional to collect.

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
| `KindDenylist` ✅ | `retention` (time=0) | Reject specific kinds (DMs etc.) |
| `RestrictedWrites` ✅ | `limitation.restricted_writes` | Pubkey allowlist/denylist |

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


#### Paid Relays

`payment_required` is not provided as mocrelay middleware.

**Reason**: NIP-11 only specifies fee disclosure, actual payment protocol is not standardized.

**How to implement paid relay**:
- Use `RestrictedWrites` to allowlist paid pubkeys
- Integrate with external payment systems (Lightning, Stripe, etc.)
- Publish fees via NIP-11 `fees` field

#### Japan Telecommunications Business Act Compliance

Use `KindDenylist` to reject these DM-related kinds:
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

Runs multiple Handlers in parallel and merges responses. Designed to remain
correct even when a downstream Handler stalls, exits early, or otherwise
deviates from contract — important for proxy-style use cases where one
child wraps an external relay.

**Typical usage**:
```go
handler := NewMergeHandler(
    []Handler{
        NewStorageHandler(storage),  // Fetch past events
        NewRouterHandler(router),    // Real-time delivery
    },
    nil, // *MergeHandlerOptions; nil = defaults
)
```

**Merge rules**:

| Message | Rule |
|---------|------|
| **OK** | Wait for all handlers, merge (any `accepted=false` => merged `false`). If any handler is lost mid-flight (timeout / early exit), forced to `accepted=false` with `MergeHandlerOKLostHandlerMessage` so the client retries. |
| **EOSE** | Wait for all handlers' EOSE before sending. A lost handler's contribution is filled in by `handlerClosed` so the client isn't stranded. |
| **EVENT (before EOSE)** | Deduplicate + drop events that break sort order |
| **EVENT (after handler EOSE)** | Pass through from that handler (real-time events) |
| **COUNT** | Take max across all handlers. A lost handler's contribution is filled in by `handlerClosed`. |

**Broadcast semantics (parent recv → children)**:

| Message type | Delivery |
|---|---|
| **EVENT** | Best-effort drop on full child recv buffer. EVENT loss is signalled to the client via OK `accepted=false` (driven by `hadHandlerLoss`), which prompts a retry. |
| **REQ / COUNT / CLOSE** | Guaranteed delivery, capped per child by `MergeHandlerOptions.BroadcastTimeout` (default 30s). A child that fails to drain in time is treated as dead — its case is removed, its slot in `childRecvs` is nil'd, and `handlerClosed` advances any merged response that depended on it. |

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
} else if newEvent.CreatedAt == lastSentCreatedAt && newEvent.ID < lastSentID {
    // drop (id ASC tiebreak — newer id is lower lexically)
} else {
    // send
}
```

**Design decisions**:
- Use `sync.Mutex` (not channel-based mutex)
- Cannot use SimpleHandlerBase (requires 1:N transformation)
- Phase-1 death detection is single-shot timeout. N-of-M streaks /
  metrics-driven scoring are intentionally deferred until real data
  argues for them (YAGNI).
- Reconnect / retry of a flapping downstream is **the Handler's own
  responsibility**, not MergeHandler's. MergeHandler retires a stalled
  child and never restarts it; a Handler that wraps a recoverable
  external resource (e.g. an upstream relay) should absorb short-term
  contention and reconnect internally.

**Handler contract for use under MergeHandler**:
- A Handler MUST drain its `recv` channel in a timely fashion. Brief
  stalls are absorbed by an internal per-child recv buffer; sustained
  stalls trip `BroadcastTimeout` and the Handler is retired.
- If a Handler wraps a slow external resource, it should buffer / retry
  internally. MergeHandler is intentionally not the only buffer in the
  chain.
- A Handler that exits early (`return err`) leaves its child recv channel
  alive but unread; MergeHandler sees the close-of-childSends and runs
  `handlerClosed`. No additional cleanup is required from the Handler.

**Considerations**:
- If a handler never sends EOSE *and* never gets retired, session maps
  grow per subscription. Countermeasures: BroadcastTimeout, CLOSE cleanup,
  subscription limits.

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
[0x06][field_hash:8][inverted_ts:8][id:32]            (49 bytes)

Replaceable/Addressable (value is event_id:32):
[0x03][addr_hash:32]  → [event_id:32]                 (33 bytes key)

Deletion markers:
[0x04][event_id:32]   → [pubkey:32][created_at:8]     (33 bytes key, 40 bytes value)
[0x05][addr_hash:32]  → [pubkey:32][created_at:8]     (33 bytes key, 40 bytes value)
```

- **Binary fixed-length**: Simple parsing, predictable key length
- **inverted_ts**: `math.MaxInt64 - created_at` (descending order in lexical sort)
- **addr_hash**: `SHA256("kind:pubkey:d-tag")` unified (replaceable uses empty d-tag)
- **field_hash**: FNV-1a 64-bit hash of each field (author, kind, tag). NUL delimiter to prevent injection

**Query architecture (queryCursor tree)**:
- **indexCursor**: Wraps a single Pebble iterator (leaf node)
- **unionCursor**: Heap-based OR merge of multiple cursors
- **intersectCursor**: Sort-merge join with SeekGE optimization (AND)
- **sliceCursor**: Pre-sorted slice for IDs filter (direct Get, no scan)
- Filter `{authors: [A,B], kinds: [1,7]}` → `intersect(union(A,B), union(kind1,kind7))`
- IDs filter short-circuits to direct `[0x01][event_id]` Get (O(1) per event)

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
- MemTableSize: 4MB (~4000 events, sufficient for small-to-medium scale relays)
- Other Pebble options: defaults are fine, add as needed

**PebbleStorage Close**:
- Caller responsible for `Close()` ("creator closes" principle)
- Required to properly close WAL and files

```go
storage, _ := NewPebbleStorage("/path/to/db", nil)
defer storage.Close()  // ← Don't forget!

handler := NewStorageHandler(storage)
relay := NewRelay(handler, nil)
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

    router := NewRouter(nil)
    sendCh := make(chan *ServerMsg, 10)
    connID := router.Register(sendCh)

    router.Subscribe(connID, "sub1", filters)
    router.Broadcast(event)

    synctest.Wait() // Wait for all goroutines to settle

    // Assert here
})
```

**Note**: Goroutines blocked on network I/O are not covered by synctest. Use for channel-based tests.

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
