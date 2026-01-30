# mocrelay

A middleware-composable Nostr relay implementation in Go.

## Features

- **Middleware Architecture**: Compose handlers and middlewares to build your relay
- **NIP-11 Driven**: Middlewares map directly to NIP-11 `limitation` fields
- **Pure Go**: No cgo dependencies, easy deployment
- **Persistent Storage**: Pebble-based storage with MVCC support

## Requirements

- Go 1.25+
- `GOEXPERIMENT=jsonv2` (for `encoding/json/v2` features)

```bash
export GOEXPERIMENT=jsonv2
```

## Installation

```bash
go get github.com/high-moctane/mocrelay
```

## Quick Start

```go
package main

import (
    "log"
    "net/http"

    "github.com/high-moctane/mocrelay"
)

func main() {
    // Create storage
    storage, _ := mocrelay.NewPebbleStorage("/var/lib/mocrelay/data", nil)
    defer storage.Close()

    // Build handler with middlewares
    handler := mocrelay.NewMergeHandler(
        mocrelay.NewStorageHandler(storage),
        mocrelay.NewRouterHandler(mocrelay.NewRouter()),
    )

    // Apply middlewares
    handler = mocrelay.NewMaxSubscriptionsMiddleware(10)(handler)
    handler = mocrelay.NewMaxContentLengthMiddleware(8192)(handler)
    handler = mocrelay.NewCreatedAtLimitsMiddleware(60*60*24*30, 60*60)(handler) // 30 days old to 1 hour future

    // Create relay
    relay := mocrelay.NewRelay(handler, nil)

    log.Fatal(http.ListenAndServe(":8080", relay))
}
```

## Available Middlewares

| Middleware | NIP-11 Field | Description |
|------------|--------------|-------------|
| `NewMaxSubscriptionsMiddleware` | `max_subscriptions` | Limit subscriptions per connection |
| `NewMaxSubidLengthMiddleware` | `max_subid_length` | Limit subscription ID length |
| `NewMaxLimitMiddleware` | `max_limit` | Clamp filter limit values |
| `NewMaxEventTagsMiddleware` | `max_event_tags` | Limit tags per event |
| `NewMaxContentLengthMiddleware` | `max_content_length` | Limit content length |
| `NewCreatedAtLimitsMiddleware` | `created_at_*_limit` | Restrict event timestamps |
| `NewKindBlacklistMiddleware` | `retention` | Block specific event kinds |
| `NewRestrictedWritesMiddleware` | `restricted_writes` | Pubkey whitelist/blacklist |
| `NewMinPowDifficultyMiddleware` | `min_pow_difficulty` | Require proof-of-work (NIP-13) |
| `NewAuthMiddleware` | `auth_required` | Require authentication (NIP-42) |
| `NewExpirationMiddleware` | - | Handle event expiration (NIP-40) |
| `NewProtectedEventsMiddleware` | - | Protect events from republishing (NIP-70) |

## NIP Support

| NIP | Status | Description |
|-----|--------|-------------|
| [01](https://github.com/nostr-protocol/nips/blob/master/01.md) | ✅ | Basic Protocol |
| [09](https://github.com/nostr-protocol/nips/blob/master/09.md) | ✅ | Event Deletion |
| [11](https://github.com/nostr-protocol/nips/blob/master/11.md) | ✅ | Relay Information |
| [13](https://github.com/nostr-protocol/nips/blob/master/13.md) | ✅ | Proof of Work |
| [40](https://github.com/nostr-protocol/nips/blob/master/40.md) | ✅ | Expiration Timestamp |
| [42](https://github.com/nostr-protocol/nips/blob/master/42.md) | ✅ | Authentication |
| [70](https://github.com/nostr-protocol/nips/blob/master/70.md) | ✅ | Protected Events |

## License

MIT License
