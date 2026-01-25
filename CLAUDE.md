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

### Development Workflow

**Handler/Middleware の開発ループ**：

```
実装 → テスト → CLAUDE.md 完了チェック → commit
```

1つずつ確実に進める。

**実装方針**：
- 可能な限り `SimpleHandlerBase` / `SimpleMiddlewareBase` をベースに実装
- テストが書きやすく、非同期処理の複雑さを隠蔽できる
- 1:N 変換が必要な特殊ケースのみ `Handler` を直接実装

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

### Handler 一覧

| Handler | 概要 |
|---------|------|
| `NopHandler` | 虚無リレー。EVENT→OK、REQ→EOSE を返すだけ |
| `RouterHandler` | クライアント間でイベントをルーティング。中央集権 Router で購読管理 |
| `StorageHandler` ✅ | Storage を wrap。EVENT→Store→OK、REQ→Query→EVENT列+EOSE |

### 実装予定の Handler/Middleware（NIP-11 ベース）

NIP-11 の `limitation` / `retention` フィールドに対応する Handler/Middleware を提供する。
これが mocrelay の主要な提供価値。

#### Tier 1: 基本的な制限（NIP-01 のみで実装可能） ✅ 完了

| Middleware | NIP-11 フィールド | 概要 |
|------------|------------------|------|
| `MaxSubscriptions` ✅ | `limitation.max_subscriptions` | 接続あたりのサブスクリプション数制限 |
| `MaxSubidLength` ✅ | `limitation.max_subid_length` | サブスクリプションID長制限 |
| `MaxLimit` ✅ | `limitation.max_limit`, `default_limit` | limit値クランプ + デフォルト値 |
| `MaxEventTags` ✅ | `limitation.max_event_tags` | タグ数制限 |
| `MaxContentLength` ✅ | `limitation.max_content_length` | content文字数制限（Unicode） |
| `CreatedAtLimits` ✅ | `limitation.created_at_lower/upper_limit` | created_at範囲チェック |
| `KindBlacklist` ✅ | `retention` (time=0) | 特定kindの拒否（DM関連など） |
| `RestrictedWrites` ✅ | `limitation.restricted_writes` | pubkeyホワイトリスト/ブラックリスト |

#### Tier 2: WebSocket/HTTP レベル ✅ 完了

| 機能 | NIP-11 フィールド | 概要 |
|------|------------------|------|
| `MaxMessageLength` ✅ | `limitation.max_message_length` | WebSocketメッセージサイズ制限（`relay.go:78-79`） |
| `NIP11Handler` ✅ | - | NIP-11 JSON を返す HTTP ハンドラ（`relay.go:244-272`） |

#### Tier 3: 他のNIPが必要 ✅ 完了

| Middleware | NIP-11 フィールド | 依存NIP |
|------------|------------------|---------|
| `MinPowDifficulty` ✅ | `limitation.min_pow_difficulty` | NIP-13 |
| `AuthRequired` ✅ | `limitation.auth_required` | NIP-42 |

#### その他の NIP 対応 ✅

| Middleware | NIP | 概要 |
|------------|-----|------|
| `ExpirationMiddleware` ✅ | NIP-40 | `expiration` タグで期限切れイベントを拒否・配信停止 |
| `ProtectedEventsMiddleware` ✅ | NIP-70 | `["-"]` タグの再公開防止（NIP-42 AUTH 前提） |

#### 今後の NIP 実装優先度

**ストレージ統合後**：
- NIP-09: Deletion Request（削除処理が必要）
- NIP-45: COUNT（集計が必要）

**検索エンジン統合後**：
- NIP-50: Search（全文検索）

**特殊機能（必要に応じて）**：
- NIP-29: Groups（複雑、グループ管理・モデレーション）
- NIP-77: Negentropy（リレー間同期）
- NIP-86: Management API（JSON-RPC over HTTP、運用向け）

**保存するだけ（特殊処理不要）**：
- NIP-22: Comment（kind 1111）
- NIP-28: Public Chat（kind 40-44、kind 41 の replaceable-like 挙動だけ注意）

詳細は `docs/nips/` に各 NIP の調査結果あり。

#### 有料リレーについて

`payment_required` は mocrelay の middleware としては提供しない。

**理由**：NIP-11 は料金を公開する仕様のみで、実際の支払いプロトコルは標準化されていない。

**有料リレーの実現方法**：
- `RestrictedWrites` で支払い済み pubkey をホワイトリスト管理
- 外部の支払いシステム（Lightning、Stripe 等）と連携
- NIP-11 の `fees` で料金を公開

#### 日本の電気通信事業法対応

`KindBlacklist` で以下の DM 関連 kind を弾く：
- kind 4（旧 DM）
- kind 13（Seal wrapper）
- kind 14（Chat Messages）
- kind 1059（Gift Wrap）
- kind 10050（DM relay list）

NIP-11 の `retention` で `time: 0` として公開すると、クライアントに事前通知できる。

### Router の設計

- **中央集権方式**：全接続・全購読を Router が管理
- **階層構造**：接続ID（サーバー生成）→ 購読ID（クライアント提供）
- **ベストエフォート送信**：channel が詰まったら drop（デッドロック防止）

```go
// 送信時は必ずこのパターン
select {
case ch <- msg:
    // 送れた
default:
    // 詰まってるから drop
}
```

### MergeHandler の設計

複数の Handler を並列実行して、レスポンスを統合する。

**典型的な使い方**：
```go
handler := NewMergeHandler(
    NewStorageHandler(storage),  // 過去イベント取得
    NewRouterHandler(router),    // リアルタイム配信
)
```

**統合ルール**：

| メッセージ | ルール |
|-----------|--------|
| **OK** | 全 handler の応答を待ってマージ（1つでも拒否なら拒否 = 障害発生中） |
| **EOSE** | 全 handler の EOSE を待ってから送信 |
| **EVENT (EOSE前)** | 重複排除 + ソートを乱す event は drop |
| **EVENT (EOSE後)** | そのまま流す（重複排除不要、プロトコル的に合法） |
| **COUNT** | 全 handler の最大値を取る |

**limit の扱い**：
- **最初の filter の limit を使用**（mocrelay の態度として）
- REQ につき最大 limit 件だけ返して EOSE
- 子 handler にも同じ limit を渡す

**ソートの drop ルール**：
- 子 handler がソート済みの応答を返す前提
- 到着順に流して、ソートを乱す event は drop
- ソート順：`created_at DESC`、タイブレーク `id ASC`（lexical order）

```go
// ソートを乱すかどうかの判定
if newEvent.CreatedAt > lastSentCreatedAt {
    // drop
} else if newEvent.CreatedAt == lastSentCreatedAt && newEvent.ID > lastSentID {
    // drop
} else {
    // 送信
}
```

**設計方針**：
- `sync.Mutex` を使う（チャネルでの mutex はやめる）
- SimpleHandlerBase は使えない（1:N 変換が必要）

**考慮点**：
- EOSE を返さない handler がいると session 内の map が肥大化する
- 対策：タイムアウト、CLOSE 時のクリーンアップ、購読数制限

### Storage インターフェース ✅

```go
type Storage interface {
    Store(ctx context.Context, event *Event) (stored bool, err error)
    Query(ctx context.Context, filters []*ReqFilter) ([]*Event, error)
    Delete(ctx context.Context, eventID string, pubkey string) error
    DeleteByAddr(ctx context.Context, kind int64, pubkey, dTag string) error
}
```

**StorageHandler の役割**：
- EVENT → Store して OK 返す
- REQ → Query して EVENT 列 + EOSE 返す
- COUNT → Query して件数を返す
- **購読管理はしない**（それは RouterHandler の仕事）
- EOSE を返したら、その REQ についての役割は終了

**InMemoryStorage**：
- slice + 全件走査の O(n) 脳筋実装
- テストがしっかりしているので後で最適化しても安心
- NIP-09 対応（timestamp チェック、kind 5 削除無効）

**永続化の選択肢**（未定）：
- PostgreSQL が有力（パーティショニング、スケーラビリティ）
- DuckDB: VPS では非力、Parquet bloom filter が list 型非対応
- SQLite: パーティショニングが難しい

### テストの書き方

**非同期処理のテストには `testing/synctest` を使う**（Go 1.25+）

```go
synctest.Test(t, func(t *testing.T) {
    // この中は "bubble" という隔離環境
    // - fake clock（時間が自動で進む）
    // - synctest.Wait() で「全 goroutine がブロックするまで待つ」

    router := NewRouter()
    sendCh := make(chan *ServerMsg, 10)
    connID := router.Register(sendCh)

    router.Subscribe(connID, "sub1", filters)
    router.Broadcast(event)

    synctest.Wait() // 全部の goroutine が落ち着くまで待つ

    // ここでアサーション
})
```

**注意**：ネットワーク I/O でブロックしてる goroutine は synctest の対象外。channel ベースのテストに使う。

## Documentation

- **docs/nips/**: リレーが実装すべき NIP 一覧（MUST/SHOULD/MAY に分類済み）
- **docs/encoding-json-v2.md**: Go 1.25 の `encoding/json/v2` 調査メモ（`GOEXPERIMENT=jsonv2` が必要）
- **最新 NIP 仕様**: `~/ghq/github.com/nostr-protocol/nips/` に clone 済み

## NIP Support

- NIP-01: Basic protocol (in progress)
- NIP-11: Relay Information Document ✅
