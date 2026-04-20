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
| **Info** | Structural lifecycle events — one per connection or per relay, not per message. | Connection start / end, Relay shutdown progress. |
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

**Slow REQ / COUNT logging** (`StorageHandlerOptions.SlowQueryThreshold`):

`StorageHandler` emits a **Warn** log (`"storage: slow REQ"` /
`"storage: slow COUNT"`) when a subscription's total duration exceeds a
configurable threshold (default 1s; zero uses the default, negative
disables). The log carries:

- `subscription_id`, `filters`
- `events_sent` (REQ) or `count` (COUNT)
- `total_ms` — wall time from `Query` call through EOSE / COUNT yield
- For REQ: `scan_ms` and `send_wait_ms` — time spent outside vs inside
  `yield`, summing to `total_ms`
- `completed` (REQ) — false if the REQ was abandoned before EOSE

Why the scan / send-wait split exists: `iter.Seq` is pull-driven, so a
blocked `yield` also pauses the storage iterator — the two time windows
are not independent. But the split is still operationally useful because
*which side owns the stall* determines the fix:

- `send_wait_ms >> scan_ms` → the client is failing to drain its send
  path (slow consumer, dead TCP peer, network congestion). Look at WS
  write timeouts and router drop metrics.
- `scan_ms >> send_wait_ms` → the filter / index / data volume is the
  bottleneck. Look at `db.Metrics()` (L0 files, compaction debt) and
  the filter shape.
- Both large → genuinely heavy query *and* slow consumer; typical when a
  client subscribes with a broad filter and can't keep up.

This is the complement to the `QueryDuration` histogram: the histogram
answers "how often are REQs slow?", the Warn log answers "which REQs,
with what shape, and which side owns the latency?"

**Query abort** (`StorageHandlerOptions.QueryTimeout`):

When the slow-query log is a passive observer, `QueryTimeout` is the
active enforcement: a REQ or COUNT whose total duration exceeds it is
*aborted*, not just logged. The handler sends a CLOSED message with
reason `"error: query timeout"` (no EOSE / COUNT is emitted), the
context passed to `Storage.Query` is canceled, and the in-progress
iteration is stopped at the next event boundary. If the slow-query
log is also enabled, it fires with `completed=false` so the abort
shows up alongside other slow subscriptions.

Zero (the default) disables the timeout — preserving prior behavior.
A negative value also disables it. When enabled, set it well above
`SlowQueryThreshold` so operators see the slow-query log fire and
have a chance to investigate before the abort kicks in.

Motivating case: a live-but-slow client keeps the WebSocket alive
(so `ping_timeout` never fires) but drains EVENTs so slowly that a
broad REQ can stall the pull-driven `iter.Seq` for hours. Without
this option the relay has no ceiling on REQ wall-time; with it, a
hard deadline is in place.

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
- **Difference = amount rejected by middleware.** The breakdown by
  cause is visible on the unified counter
  `mocrelay_rejections_total{middleware, reason}` — every middleware
  contributes to the same metric via [logRejection], distinguished by
  the `middleware` label. See Known concerns #2 for why this shape
  (rather than one metric per middleware) was chosen.

#### Log ↔ metric cross-index

`logRejection` is the single place where every middleware reports a
rejection. It performs two side-effects from the same call site:

1. Increments the context-injected `mocrelay_rejections_total{middleware,
   reason}` counter (installed by `Relay` when `RelayOptions.Registerer`
   is set; no-op if absent).
2. Emits a Debug-level structured log entry with the same `middleware`
   and `reason` fields.

Because the two sinks share the exact label/field strings, the
following queries target the same events:

```
grep 'reason=max_content_length' relay.log
rate(mocrelay_rejections_total{reason="max_content_length"}[5m])
```

The `middleware` axis lets operators scope further:
`sum by(reason) (mocrelay_rejections_total{middleware="auth"})`.

#### Instrument choice

| Instrument | When | Hot-path note |
|---|---|---|
| **Counter** | Any monotonic count (requests, errors, rejections, drops, bytes in / out) | Lock-free, cheap; fine on every message |
| **Gauge** | Current level (connections, subscriptions, buffer depth) | Must have matched Inc / Dec (`defer`, cleanup, or `Set`) or it drifts upward forever |
| **Histogram** | Latency / size distributions where the tail matters | More expensive than Counter; acceptable per REQ / per Store. For per-EVENT hot paths, audit buckets first |

#### Registerer injection

Metrics are optional, matching the philosophy of `Relay.Logger`. Every
component that owns instrumented behavior exposes a single
`Registerer prometheus.Registerer` field on its `*Options` struct:

- `RelayOptions.Registerer` — Relay's own conn / message / event
  counters, *plus* the request-ctx-injected rejection and auth
  counters (one field, three metric families registered).
- `RouterOptions.Registerer` — router saturation.
- `CompositeStorageOptions.Registerer` — search / index RED.
- `MergeHandlerOptions.Registerer` — merge-handler USE.
- `NewMetricsStorage(storage, reg)` — Storage RED (decorator, no
  Options struct; see Constructor API exceptions below).

Setting the field causes the constructor to build and register every
metric the component owns in one shot. Leaving it nil disables
instrumentation: no collectors are created, every instrument site
short-circuits on a nil check, and no Prometheus code runs on the
hot path. The authoritative list of what each `Registerer` registers
lives in `metrics.go`.

Two consumption shapes coexist under the same field, determined by
whether the metric's natural consumer is the owning type itself or
something composed *into* a Relay:

- **Self-owned** — Relay's own WS counters, Router, Composite, Merge,
  the Storage decorator. The constructor builds the metrics struct
  internally and the owning type calls into it directly.
- **Ctx-injected** — the rejection and auth counters. The `Relay`
  constructor builds both structs from `RelayOptions.Registerer`,
  installs them into every request context, and the consuming
  middleware reads them back via internal helpers — mirroring the
  `ContextWithLogger` pattern. Middleware code never holds a
  reference and never nil-checks. The rule for picking the ctx path
  is "the metric's natural consumer is a unit that composes *into* a
  Relay, rather than the Relay itself."

These structs (`relayMetrics`, `rejectionMetrics`, `authMetrics`,
etc.) and the accompanying `contextWith…` / `…FromContext` helpers
are all unexported: the public API surface is just the `Registerer`
field on each Options struct. Subregistry use cases
(`prometheus.WrapRegistererWithPrefix`, etc.) compose naturally — wrap
the registry and hand the wrapper in.

Just as `Logger == nil` falls back to `slog.Default()`, every
`Registerer == nil` path falls back to silence.

`MergeHandlerMetrics` is the next candidate to consider for the
ctx-injected path under the same rule (a handler that composes into
Relay rather than the Relay itself). It stays self-owned for now; a
follow-up PR can evaluate the move once a concrete need arises.

#### Coverage by layer (drives follow-up PRs)

Series names below are the Prometheus metric names (all prefixed
`mocrelay_`). The authoritative mapping to internal Go fields is in
`metrics.go`; there is no `*Metrics` public type to reference.

| Layer | Kind | Existing | Gaps (future PRs) |
|---|---|---|---|
| **Relay (WS conn)** | RED-R | `connections_total`, `messages_received_total{type}`, `messages_sent_total{type}`, `events_received_total{kind, type}` | — |
| | RED-E | `ws_parse_errors_total{reason}`, `ws_write_errors_total{reason}` | — |
| | RED-D | `ws_write_duration_seconds` | — |
| | USE-Sat | `connections_current` | — (see note below) |
| **Router** | USE-Sat | `router_messages_dropped_total`, `router_subscriptions_current` | — |
| **Middleware: Auth** | RED-R+E | `auth_total{result}`, `auth_authenticated_connections_current`, rejections via unified `rejections_total{middleware="auth", reason}` | — |
| **Middleware: others (10)** | RED-E | Rejections via unified `rejections_total{middleware, reason}` (wired automatically through `logRejection`) | — |
| **StorageHandler** | RED-R+D+E | `events_stored_total{kind, type, stored}`, `store_duration_seconds`, `query_duration_seconds`, `store_errors_total`, `query_errors_total` | — |
| **MergeHandler** | USE-Sat+E | `merge_lost_children_total`, `merge_broadcast_timeouts_total`, `merge_event_drops_total{reason}` | — |
| **Pebble (internals)** | USE | — (caller-owned) | External: expose via caller's `*pebble.DB` — `db.Metrics()` → L0 files, compactions, WAL bytes |
| **Bleve / SearchIndex (via CompositeStorage)** | RED-R+E | `search_total`, `search_errors_total`, `index_total`, `index_errors_total` | — |
| | USE | — (caller-owned) | External: expose via caller's `bleve.Index` — `idx.StatsMap()` → doc count, batch stats |

Notes on retracted gaps:

- **`send_buf_full_drops_total`** — withdrawn. The Relay send channel
  uses a blocking send path throughout (`readLoop` → handler pipeline
  is unbuffered at the recv boundary; `sendNotice` blocks on
  `ctx.Done()` or delivery), so there is no drop site to instrument
  under the current design. If the send boundary is ever converted to
  best-effort drop (e.g. to mirror the Router's broadcast semantics),
  this counter should be reintroduced together with the policy change.
- **`write_timeouts_total`** — subsumed by
  `WSWriteErrors{reason="timeout"}`. The writeLoop distinguishes
  `context.DeadlineExceeded` on `conn.Write` from other failures via
  the `reason` label; a dedicated counter would duplicate this.
- **`event_sort_drops_total`** — shipped as
  `EventDrops{reason}` instead, covering dedup / sort / limit drops
  (`duplicate` / `out_of_order` / `after_limit`) *and* the broadcast
  recv-buffer-full drop (`recv_buf_full`) in a single counter. The
  broader name reflects the broader coverage.

#### Metric naming conventions

All Prometheus series emitted by mocrelay use the `mocrelay_` prefix
with standard instrument-type suffixes (`_total` for Counters,
`_seconds` for duration Histograms, `_current` for Gauges tracking
level, unsuffixed for plain Counters measuring sizes / offsets).
Internal Go fields in `metrics.go` follow the unsuffixed,
CamelCase counterpart (e.g. the Go field `QueryDuration` drives the
series `mocrelay_query_duration_seconds`), but the fields are
unexported: the authoritative list is the constructor body of each
`new*Metrics` function.

Notable gotcha: there is **no `storage_` infix** for StorageHandler
series. They are flat — `mocrelay_query_duration_seconds`,
`mocrelay_store_errors_total`, etc. — not
`mocrelay_storage_query_*`. PromQL that naïvely prefixes with
`storage` will match nothing.

#### Known concerns (decide before v0.x freeze)

1. **`kind` label explosion** — **implemented: two-axis label
   (`kind`, `type`).** `EventsReceived` and `EventsStored` originally
   took a single free-form `kind` label (`int64`), letting a hostile or
   buggy client grow the series set without bound. Mitigation lives in
   `kind_label.go` via the `kindLabels(int64) (kind, type string)`
   helper, with the two labels carrying deliberately redundant
   information:
   - **`kind`** — decimal form if the kind is in the known-kind
     allowlist (0, 1, 3, 4, 5, 6, 7, 13, 14, 40-44, 1059, 1111, 1984,
     9734, 9735, 10002, 10050, 30023); otherwise `"other"`.
   - **`type`** — NIP-01 category: `"regular"` (kind 1, 2, 4-44,
     1000-9999), `"replaceable"` (0, 3, 10000-19999), `"ephemeral"`
     (20000-29999), `"addressable"` (30000-39999), `"unknown"` for
     anything else (45-999, ≥ 40000, negatives).

   Because `type` is a function of `kind`, the two-axis label is *not*
   a cardinality cross-product: the observed series set is bounded at
   `len(knownKinds) + 5 = 28` (23 allowlist kinds each paired with
   exactly one type, plus `kind="other"` paired with each of the 5
   type labels). The redundancy is a query-ergonomics trade: operators
   get `sum by(type)` for category-level shape and `sum by(kind)` for
   per-kind fidelity in the same metric without managing an info
   metric + `group_left` join. Prometheus best practice usually favours
   orthogonal labels, but the `(kind, type)` functional relationship is
   fixed by the NIP-01 spec so the usual cardinality-blowup concern
   doesn't apply.

   Alternatives considered and rejected: single-label NIP-01 range
   buckets (loses per-kind detail for e.g. kind 1, 7, 10002, 30023 —
   operationally interesting); single-label encoded pair like
   `kind="1:regular"` (ugly, requires client-side parsing); dropping
   the label entirely (recoverable from the event log but inconvenient
   for dashboards / alerts); info metric + `group_left` join (PromQL
   overhead not justified given the fixed NIP mapping). Adjust the
   allowlist based on operational experience.

2. **Rejection counter scope** — **implemented: unified
   `mocrelay_rejections_total{middleware, reason}`.** A previous draft
   of this policy argued for keeping one `<name>_rejections_total`
   metric per middleware. In practice that shape required ten new
   `*Options` structs and ~30 call-site updates across examples and
   tests for the initial roll-out, so the convenience trade flipped:
   a single counter with `{middleware, reason}` labels gives the same
   observability with dramatically less surface area. Labels are a
   functional pair (each middleware emits a fixed, small enum of
   reasons), so the label set is bounded and does not form a true
   cross-product — the same argument as Known concern #1's
   `(kind, type)` two-axis label.

   Implementation:
   - The unified counter lives in `metrics.go` as the internal
     `rejectionMetrics` struct, wrapping a single
     `mocrelay_rejections_total` CounterVec.
   - `Relay` builds and registers it from `RelayOptions.Registerer`
     and installs it into the request context, mirroring
     `ContextWithLogger`.
   - `logRejection` reads it from context on every call and increments
     `{middleware, reason}` alongside its Debug log line. Middleware
     authors don't touch metrics code at all.
   - Auth-specific rejection reasons (`event_unauthenticated`,
     `expired`, …) land on the unified counter with `middleware="auth"`;
     there is no separate per-middleware rejection metric.

   Rejected alternatives: per-middleware `*Options` with a `Metrics`
   field (API bloat across 10 middleware, constructor signature churn
   propagated to every call site, little benefit since the reason
   enums never collide once the `middleware` label is present);
   removing the rejection counter entirely in favour of log-only
   observability (forces operators to run log aggregators for basic
   rate questions and loses the cheap Prometheus alert surface).

3. **Pebble / Bleve internals — USE is exposed by the caller, RED for
   Bleve search is internal.** `*pebble.DB` and `bleve.Index` are passed
   in by the caller (see the "Caller-owned `*pebble.DB`" section), so
   `db.Metrics()` — L0 files, pending compactions, WAL bytes — and
   `idx.StatsMap()` live on the caller's side of the boundary. The
   usual pattern is a periodic collector goroutine in the caller's
   main reading these every N seconds into Gauges registered to the
   same Prometheus registry that mocrelay's own metrics use. Putting
   this inside mocrelay was the original plan, but it forced mocrelay
   to (a) pin Pebble / Bleve versions tightly and (b) grow a new
   "proxy every useful field" API surface for every new Pebble
   release; moving the DB handle out eliminates both costs.

   Request-path RED for Bleve search (rate / errors) **has been added
   inside mocrelay** on `CompositeStorage` via `CompositeStorageMetrics`:
   `SearchTotal`, `SearchErrors`, `IndexTotal`, `IndexErrors`. These
   instrument the two call sites where `CompositeStorage` would
   otherwise silently swallow a backend failure — `Search` falls back
   to the primary on error, and `Index` is fire-and-forget on the
   success path of `Store`. Without these counters the search backend
   could be fully broken (clients still see results, just not
   search-scored) or the search index could silently drift out of sync
   with the primary, and nobody would notice until a user complained.
   Each failure is also logged at Warn via `LoggerFromContext(ctx)`
   with matching fields (`event_id` / `query`, `error`) so logs and
   metrics stay cross-indexable, mirroring the `logRejection` pattern.

   Duration (RED-D) was deliberately not added on the search side: the
   Bleve hop is dominated by `Search`'s own latency (primary
   `IDs → Event` fetch is O(1) per ID), and that latency is already
   captured inside `StorageMetrics.QueryDuration`. A separate
   `SearchDuration` histogram would track the same shape; it can be
   introduced later if a concrete operational need emerges (e.g.
   isolating Bleve slowness from Pebble slowness when
   `QueryDuration` spikes).

### Design Decisions

#### Constructor API

All public constructors in mocrelay follow a single shape:

```go
NewX(required..., opts *XOptions) *X
```

Rules:

- **Positional args are required** (e.g. `handler`, `relayURL`, `db`).
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
- **Prometheus registration is a single `Registerer` field on `*Options`**,
  never a separate positional arg or a post-construction field.
  Each component owns the metrics it registers; metrics whose natural
  consumer is a middleware (rejection, auth) are registered by the
  Relay and ctx-injected. See the "Registerer injection" section in
  the Metrics policy above for the full rule.

This pattern is used by `NewRelay`, `NewRouter`, `NewAuthMiddlewareBase`,
`NewPebbleStorage`, `NewBleveIndex`, `NewCompositeStorage`, and
`NewStorageHandler`. New
Handlers / Middlewares / Storages added to mocrelay should follow the
same shape so third-party code composing with mocrelay has a predictable
surface.

Exceptions:
- `MetricsStorage` uses the decorator pattern
  (`NewMetricsStorage(storage Storage, reg prometheus.Registerer)`)
  because `Storage` is an interface with multiple implementations
  (InMemory / Pebble / Composite) and a decorator is the natural way
  to inject cross-cutting behavior. It takes the Registerer
  positionally rather than through an Options struct; `reg == nil` is
  valid and makes every call pass straight through to the wrapped
  storage.
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
        NewStorageHandler(storage, nil),  // Fetch past events
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

**Caller-owned `*pebble.DB`**:

PebbleStorage does not open or close the Pebble database. The caller
opens `*pebble.DB` with their own [pebble.Options] and passes it in:

```go
db, err := pebble.Open("/path/to/db", &pebble.Options{
    Cache:  pebble.NewCache(64 << 20), // 64 MB
    Levels: bloomLevels,               // bloom filter per level
    // EventListener, FormatMajorVersion, ... all caller's choice
})
if err != nil { ... }
defer db.Close()  // ← caller closes the DB

storage := NewPebbleStorage(db, nil) // no error, no Close()
handler := NewStorageHandler(storage, nil)
relay := NewRelay(handler, nil)
```

Consequences:

- **PebbleStorage has no `Close()` method.** Lifecycle is the caller's.
- **`PebbleStorageOptions` is currently empty** (reserved for future
  use). Pass `nil`. DB-level tuning (cache size, bloom filter,
  memtable size, event listeners, …) lives on `pebble.Options`, not on
  PebbleStorageOptions.
- **`db.Metrics()` is directly accessible to the caller**, so
  Pebble-internal observability (L0 files, compaction debt, WAL bytes)
  is exposed by the caller to their own Prometheus registry; mocrelay
  no longer proxies it. See Coverage-by-layer note: the Pebble row is
  "external — exposed via the caller's *pebble.DB".
- **Pebble version** is the caller's choice too (MVS picks the highest
  v1.x in the module graph; major-version differences would require
  a `/v2` import path bump on mocrelay's side).

**Recommended Pebble.Options for Nostr workloads** (not set by mocrelay;
apply yourself if they fit your use case):

- Bloom filter 10 bits/key (~1% false positive), table-level filter on
  every level — significantly speeds up negative lookups on the
  per-event-id index (`[0x01][event_id:32]`).
- MemTableSize 4 MB — roughly 4000 events, adequate for small-to-medium
  relays; raise it for write-heavy deployments.
- Cache 64-256 MB in production, 8 MB (Pebble default) for dev.

`BleveIndex` follows the same pattern: the caller opens the
`bleve.Index` (typically with `mocrelay.BuildIndexMapping()`), passes
it to `NewBleveIndex(idx, nil)`, and closes it themselves. `BleveIndex`
has no `Close()`, and `BleveIndexOptions` is currently empty.
`BuildIndexMapping()` is exported specifically because the Bleve
mapping is part of the search semantics (CJK analyzer on `content`,
keyword on `pubkey`, numeric on `kind` / `created_at`); callers who
build their own index must use this mapping or Index/Search/Delete
will operate on a different document shape.

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
