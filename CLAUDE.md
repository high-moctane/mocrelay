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

### Channel 操作のパターン（goroutine leak 防止）

**基本原則**：channel への send/recv 時は `ctx.Done()` をチェックして、goroutine が終了できるようにする。

#### ✅ 安全なパターン

```go
// send: ctx.Done() と併用
select {
case <-ctx.Done():
    return ctx.Err()
case ch <- msg:
}

// recv: ctx.Done() と併用
select {
case <-ctx.Done():
    return ctx.Err()
case msg, ok := <-ch:
    if !ok {
        return nil // channel closed
    }
}

// best effort send: ブロックしない（Router の broadcast など）
select {
case ch <- msg:
default:
    // drop
}
```

#### ⚠️ 注意が必要なケース

**Buffered channel への send**：
- `ch := make(chan T, 1)` の場合、1回目の send はブロックしない
- 複数回 send する可能性がある場合は `select` + `ctx.Done()` が必要

**Unbuffered channel への send**：
- 常に `select` + `ctx.Done()` が必要（ブロックする可能性がある）

**Pebble などの I/O 操作**：
- Pebble の API は ctx を尊重しない（Go の io 全般の制限）
- 通常はミリ秒単位で完了するので許容
- 長時間ブロックする場合は、より大きな問題がある状況

### メモリリーク防止（状態管理）

**問題**：Handler が subscription ごとに状態を持つ場合、CLOSE を受け取らずに接続が切断されると状態が残り続ける可能性がある。

**対策**：

1. **接続終了時のクリーンアップ**：
   - `ServeNostr()` が終了すると、その Handler の状態は GC される
   - ぶち切りされても問題なし

2. **CLOSE メッセージでのクリーンアップ**：
   - 長時間の接続で、大量の subscription を作成→放置するケースに対応
   - MergeHandler: `closeSubscription(subID)` で状態を削除
   - Router: `Unsubscribe(connID, subID)` で購読を削除

**状態を持つ Handler の例**：

| Handler | 状態 | クリーンアップ |
|---------|------|--------------|
| **MergeHandler** | `pendingEOSEs`, `completedSubs`, `limitReachedSub` | CLOSE で削除 |
| **Router** | `connections[connID].subscriptions` | CLOSE で削除、切断時は Unregister で接続ごと削除 |

**状態を持たない Handler**（心配不要）：
- `StorageHandler`: REQ ごとに処理完結、状態なし
- `NopHandler`: 状態なし

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
| `MergeHandler` ✅ | 複数 Handler を並列実行してレスポンスを統合 |

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

### MergeHandler の設計 ✅

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
| **EVENT (handler EOSE後)** | その handler からの EVENT は pass through（リアルタイムイベント） |
| **COUNT** | 全 handler の最大値を取る |

**limit の扱い**：
- **最初の filter の limit を使用**（mocrelay の態度として）
- REQ につき最大 limit 件だけ返して EOSE
- 子 handler にも同じ limit を渡す
- **limit 到達後の EVENT は drop**（リアルタイムイベントも含む）

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

**iter.Seq パターン**（Go 1.23+）を採用：

```go
type Storage interface {
    Store(ctx context.Context, event *Event) (bool, error)
    Query(ctx context.Context, filters []*ReqFilter) (events iter.Seq[*Event], err func() error, close func() error)
}

// Optional: Count をサポートする Storage（NIP-45）
type CountableStorage interface {
    Storage
    Count(ctx context.Context, filters []*ReqFilter) (int64, error)
}
```

**使用側**：
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

**メリット**：
- **for-range で直感的**：`for event := range events` でスッキリ
- **ストリーミング応答**：全件揃う前にクライアントに返せる
- **PebbleStorage で Snapshot 使用**：Query 中も Write をブロックしない（MVCC）
- **シンプルなインターフェース**：Store と Query の 2 メソッドのみ

**Delete / DeleteByAddr を削除した理由**：
- Kind 5 の処理は `Store` 内で完結している
- NIP-86（管理 API）にも特定 event の削除 API はない
- 外部 API としては不要

**StorageHandler の役割**：
- EVENT → Store して OK 返す
- REQ → Query して EVENT 列 + EOSE 返す（ストリーミング）
- COUNT → Count があれば使う、なければ Query で代替
- **購読管理はしない**（それは RouterHandler の仕事）
- EOSE を返したら、その REQ についての役割は終了

**InMemoryStorage**：
- slice + 全件走査の O(n) 脳筋実装
- テストがしっかりしているので後で最適化しても安心
- NIP-09 対応（timestamp チェック、kind 5 削除無効）

**永続化**: Pebble（決定）
- **github.com/cockroachdb/pebble**: CockroachDB 製の LSM-tree ベース KV ストア
- Pure Go（cgo なし）、組み込み、デプロイがシンプル
- **Snapshot** で読み取り時のロック不要（MVCC）

**選定理由**：
- PostgreSQL: 全文検索（pgroonga）は魅力だが、外部プロセス管理が必要
- DuckDB: OLAP 向き、リアルタイム書き込みが苦手
- SQLite: cgo 問題、パーティショニングが難しい
- **Pebble**: Pure Go、ストリーミング取得◎、Nostr の追記ワークロードと相性◎

**Key スキーマ（バイナリ固定長）**：
```
主データ:
[0x01][event_id:32]  →  event_json                    (33 bytes)

インデックス（Value は空）:
[0x02][inverted_ts:8][id:32]                          (41 bytes)
[0x03][pubkey:32][inverted_ts:8][id:32]               (73 bytes)
[0x04][kind:8][inverted_ts:8][id:32]                  (49 bytes)
[0x05][tag_name:1][tag_hash:32][inverted_ts:8][id:32] (74 bytes)

Replaceable/Addressable 専用（Value は event_id:32）:
[0x06][addr_hash:32]  → [event_id:32]  (33 bytes key)

削除マーカー:
[0x08][event_id:32]   → [pubkey:32][created_at:8]  (33 bytes key, 40 bytes value)
[0x09][addr_hash:32]  → [pubkey:32][created_at:8]  (33 bytes key, 40 bytes value)
```

- **addr_hash**: `SHA256("kind:pubkey:d-tag")` で統一（replaceable は d-tag 空）

- **バイナリ固定長**: parse がシンプル、Key 長が予測可能
- **inverted_ts**: `math.MaxInt64 - created_at`（辞書順で降順になる）
- **tag_hash**: SHA256(tag_value) で 32 bytes 固定（collision は無視できる）
- **複合インデックス**: なし（Multi-Cursor Merge で対応、必要なら後から追加）

**全文検索（NIP-50）**：
- Pebble では対応しない
- 必要なら別の検索エンジン（Bleve, Meilisearch 等）を MergeHandler で統合

**PebbleStorageOptions**：
```go
type PebbleStorageOptions struct {
    CacheSize int64  // ブロックキャッシュ（デフォルト 8MB、本番は 64-256MB 推奨）
    FS        vfs.FS // テスト用（vfs.NewMem()）
}
```

**固定設定（変更不要）**：
- Bloom filter: 10 bits/key（~1% false positive）、全レベル Table-level filter
- MemTableSize: 4MB（約 4000 イベント分、mocvps 規模なら十分）
- その他の Pebble オプション: デフォルトで良い、必要になったら追加

**Differential Testing**：
- `storage_differential_test.go` で InMemory と Pebble の挙動一致を検証
- シードベースのランダムテスト（再現可能）
- StorageHandler レベルでも検証（EVENT→OK、REQ→EVENT*+EOSE）

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
