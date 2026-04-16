# mocrelay

A middleware-composable [Nostr](https://nostr.com/) relay library for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/high-moctane/mocrelay.svg)](https://pkg.go.dev/github.com/high-moctane/mocrelay)

## Features

- **Middleware Architecture** - Compose handlers and middleware to build your relay
- **NIP-11 Driven** - Built-in middleware maps directly to NIP-11 `limitation` fields
- **Pure Go** - No cgo dependencies, single binary deployment
- **Persistent Storage** - Pebble-based LSM-tree storage with MVCC support
- **Full-text Search** - Bleve-based NIP-50 search with CJK support
- **Prometheus Metrics** - Built-in instrumentation for relay, router, auth, and storage
- **Standard http.Handler** - Composes naturally with `net/http.ServeMux`

## Requirements

- Go 1.25 or later
- `GOEXPERIMENT=jsonv2` environment variable

```bash
export GOEXPERIMENT=jsonv2
```

## Installation

```bash
go get github.com/high-moctane/mocrelay
```

## Quick Start

The simplest possible relay is a single line:

```go
func main() {
	log.Fatal(http.ListenAndServe(":7447", mocrelay.NewRelay(mocrelay.NewNopHandler())))
}
```

This accepts all events and returns EOSE for all subscriptions — a valid relay with zero storage, in one line.

For a more practical starting point, see the [examples](cmd/examples/).

## Examples

The [`cmd/examples/`](cmd/examples/) directory provides three graduated examples:

| Example | Description |
|---------|-------------|
| **[nop](cmd/examples/nop/)** | One-line relay. Demonstrates that a `Handler` is all you need. |
| **[minimal](cmd/examples/minimal/)** | In-memory storage, real-time routing, and basic middleware. A functional relay in ~80 lines. |
| **[kitchen-sink](cmd/examples/kitchen-sink/)** | Pebble + Bleve + all middleware + Prometheus + ServeMux. Everything mocrelay offers. |

## Architecture

```
            ┌──────────────────────────────────────────────┐
            │          Relay (http.Handler)                │
            │          WebSocket lifecycle management      │
            ├──────────────────────────────────────────────┤
            │          Middleware Pipeline                 │
            │   Auth → ProtectedEvents → Limits → ...      │
            ├──────────────────────────────────────────────┤
            │              MergeHandler                    │
            │     ┌────────────────┬──────────────────┐    │
            │     │ StorageHandler │  RouterHandler   │    │
            │     │ (past events)  │  (real-time)     │    │
            │     └───────┬────────┴────────┬─────────┘    │
            ├─────────────┼─────────────────┼──────────────┤
            │   ┌─────────┴────────┐  ┌─────┴───────┐      │
            │   │ CompositeStorage │  │   Router    │      │
            │   │  ┌──────┬──────┐ │  │ (broadcast) │      │
            │   │  │Pebble│Bleve │ │  └─────────────┘      │
            │   │  └──────┴──────┘ │                       │
            │   └──────────────────┘                       │
            └──────────────────────────────────────────────┘
```

- **[Relay](https://pkg.go.dev/github.com/high-moctane/mocrelay#Relay)** serves HTTP/WebSocket and manages connection lifecycles
- **[Handler](https://pkg.go.dev/github.com/high-moctane/mocrelay#Handler)** processes messages for a single connection
- **[Middleware](https://pkg.go.dev/github.com/high-moctane/mocrelay#Middleware)** wraps a Handler to add cross-cutting concerns
- **[Storage](https://pkg.go.dev/github.com/high-moctane/mocrelay#Storage)** persists and queries events using Go iterators (`iter.Seq`)
- **[Router](https://pkg.go.dev/github.com/high-moctane/mocrelay#Router)** routes events between connected clients in real-time

## Available Middleware

| Middleware | NIP-11 Field | Description |
|------------|--------------|-------------|
| `MaxSubscriptions` | `max_subscriptions` | Limit subscriptions per connection |
| `MaxSubidLength` | `max_subid_length` | Limit subscription ID length |
| `MaxLimit` | `max_limit`, `default_limit` | Clamp filter limit values |
| `MaxEventTags` | `max_event_tags` | Limit tags per event |
| `MaxContentLength` | `max_content_length` | Limit content length |
| `CreatedAtLimits` | `created_at_lower/upper_limit` | Restrict event timestamps |
| `KindDenylist` | `retention` | Block specific event kinds |
| `RestrictedWrites` | `restricted_writes` | Pubkey allow/deny list |
| `MinPowDifficulty` | `min_pow_difficulty` | Require proof-of-work (NIP-13) |
| `Auth` | `auth_required` | Require authentication (NIP-42) |
| `Expiration` | - | Handle event expiration (NIP-40) |
| `ProtectedEvents` | - | Protect events from republishing (NIP-70) |

Multiple middleware are composed into a single pipeline via [`NewSimpleMiddleware`](https://pkg.go.dev/github.com/high-moctane/mocrelay#NewSimpleMiddleware):

```go
handler = mocrelay.NewSimpleMiddleware(
    mocrelay.NewMaxSubscriptionsMiddlewareBase(20),
    mocrelay.NewMaxLimitMiddlewareBase(500, 100),
    mocrelay.NewKindDenylistMiddlewareBase([]int64{4, 1059}),
)(handler)
```

## NIP Support

| NIP | Description |
|-----|-------------|
| [NIP-01](https://github.com/nostr-protocol/nips/blob/master/01.md) | Basic Protocol |
| [NIP-09](https://github.com/nostr-protocol/nips/blob/master/09.md) | Event Deletion |
| [NIP-11](https://github.com/nostr-protocol/nips/blob/master/11.md) | Relay Information |
| [NIP-13](https://github.com/nostr-protocol/nips/blob/master/13.md) | Proof of Work |
| [NIP-40](https://github.com/nostr-protocol/nips/blob/master/40.md) | Expiration Timestamp |
| [NIP-42](https://github.com/nostr-protocol/nips/blob/master/42.md) | Authentication |
| [NIP-45](https://github.com/nostr-protocol/nips/blob/master/45.md) | Event Counts |
| [NIP-50](https://github.com/nostr-protocol/nips/blob/master/50.md) | Search Capability |
| [NIP-70](https://github.com/nostr-protocol/nips/blob/master/70.md) | Protected Events |

## License

[MIT](LICENSE)
