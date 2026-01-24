# CLAUDE.md

mocrelay - A Nostr relay implementation in Go.

## Project Status

This is the `rewrite` branch - rebuilding from scratch.

## Commands

```bash
go build ./...     # Build
go test ./...      # Test
go tool lefthook install  # Install git hooks
```

## Rewrite Branch Policy

### Basic Principles

- **Complete rewrite**: LLM-assisted development for consistency and quality
- **Destructive changes OK**: No users depend on this yet
- **Start minimal, grow incrementally**: Build the smallest working thing first
- **Discuss as we go**: Design decisions made through conversation

### Scope

- **In scope**: NIP-01 (basic protocol) working correctly
- **Out of scope (for now)**:
  - VPS operation conveniences
  - SQLite / persistent storage
  - Prometheus metrics
  - Dockerfile

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

## LLM が間違えやすいポイント（学習データとの差分）

⚠️ 以下は 2025年1月頃までの学習データと現在の NIP-01 で混乱しやすい点です。

### Filter は exact match（prefix match ではない）

❌ 間違い：`{"ids": ["abcdef"]}` で prefix match できる
✅ 正解：**64文字の lowercase hex のみ**

> "The `ids`, `authors`, `#e` and `#p` filter lists MUST contain exact 64-character lowercase hex values."

mocrelay では exact match を採用（DB インデックスの効率を考慮）。

### limit の適用範囲

- **initial query にのみ適用**（リアルタイム更新には適用されない）
- ソート順：`created_at DESC`、同値なら `id ASC`（lexical order）

### e タグの 4番目のフィールド

`["e", <event_id>, <relay_url>, <author_pubkey>]`

4番目に author の pubkey を追加可能（optional）。

### a タグの末尾コロン

- addressable: `30023:pubkey:identifier`
- replaceable: `10000:pubkey:` ← **末尾コロン必須**

### タグは最初の値のみインデックス

> "Only the first value in any given tag is indexed."

`["e", "id1", "relay", "author"]` → `id1` のみがフィルタ対象。

## Architecture

(To be documented as we build)

## NIP Support

- NIP-01: Basic protocol (in progress)
