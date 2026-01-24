# NIP-42: Authentication of clients to relays

**ステータス**: `draft` `optional` `relay`

## 概要

AUTH メッセージによるクライアント認証。kind 22242 の ephemeral イベントで署名。

## リレーの責務

### AUTH フロー

1. **チャレンジ送信**: `["AUTH", "<challenge-string>"]`
   - 接続時、またはリクエスト直前
2. **AUTH イベント受信**: `["AUTH", <signed-event-json>]`
   - 複数の pubkey からの AUTH を受け付け MAY
3. **OK 応答**: `["OK", <event-id>, <bool>, <message>]`

### 認証が必要な場合

- **REQ 拒否**: `["CLOSED", <sub_id>, "auth-required: ..."]`
- **EVENT 拒否**: `["OK", <event_id>, false, "auth-required: ..."]`
- **認証済みでも権限不足**: `["CLOSED", <sub_id>, "restricted: ..."]`

### kind 22242 イベントの検証

```json
{
  "kind": 22242,
  "tags": [
    ["relay", "wss://relay.example.com/"],
    ["challenge", "<challenge-string>"]
  ],
  "created_at": <現在時刻>,
  "pubkey": "<認証する公開鍵>"
}
```

**検証項目**:
- `kind == 22242`
- `created_at` が現在時刻の ±10分以内 SHOULD
- `challenge` タグが一致
- `relay` タグが一致（URL 正規化可能）
- 署名検証

### kind 22242 の扱い

- **配信禁止**: kind 22242 を他のクライアントに配信してはならない MUST

## 実装ポイント

### チャレンジ文字列の生成

```go
challenge := generateRandomString(32)
// セッションに保存
session.challenge = challenge
session.challengeTime = time.Now()
```

### セッション管理

```go
type Session struct {
    Challenge     string
    ChallengeTime time.Time
    AuthedPubkeys []string  // 認証済み pubkey のリスト
}
```

### URL 正規化

`relay` タグの検証では、厳密な文字列一致でなくてもよい：
- ドメイン名が一致すればOK（大抵の場合）
- `wss://relay.example.com` と `wss://relay.example.com/` は同じとみなす

### エッジケース

- **複数の AUTH**: 同一セッションで複数の pubkey から AUTH 可能
- **チャレンジの有効期限**: 接続が続く限り有効、または次のチャレンジまで
- **再認証**: 新しいチャレンジを送ることで再認証を要求可能

## NIP-11 での advertise

```json
{
  "supported_nips": [42],
  "limitation": {
    "auth_required": true  // 接続時に必須の場合
  }
}
```

## 使用例

### DM へのアクセス制限

```
client: ["REQ", "sub_1", {"kinds": [4]}]
relay: ["CLOSED", "sub_1", "auth-required: we can't serve DMs to unauthenticated users"]
client: ["AUTH", {...}]
relay: ["OK", "...", true, ""]
client: ["REQ", "sub_1", {"kinds": [4]}]
relay: ["EVENT", "sub_1", {...}]
```

### 有料リレー

認証により pubkey を特定し、支払い済みか確認。

## 他の NIP との関連

- **NIP-70**: Protected Events（AUTH が必須）
- **NIP-86**: Relay Management API（NIP-98 で認証）

## 参考

- [NIP-42 原文](https://github.com/nostr-protocol/nips/blob/master/42.md)
