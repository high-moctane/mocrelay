# NIP-86: Relay Management API

**ステータス**: `draft` `optional`

## 概要

JSON-RPC over HTTP によるリレー管理 API。ban/allow、メタデータ変更など。

## リレーの責務

### HTTP エンドポイント

- **URL**: WebSocket と同じ URI
- **Content-Type**: `application/nostr+json+rpc`
- **認証**: NIP-98 Authorization ヘッダー必須

### リクエスト形式

```json
{
  "method": "<method-name>",
  "params": ["<param1>", "<param2>", ...]
}
```

### レスポンス形式

```json
{
  "result": {"<key>": "<value>"},
  "error": "<optional-error-message>"
}
```

### サポートすべきメソッド（オプション）

| メソッド | パラメータ | 説明 |
|---------|----------|------|
| `supportedmethods` | `[]` | サポートするメソッド一覧 |
| `banpubkey` | `[<pubkey>, <reason>]` | pubkey をバン |
| `listbannedpubkeys` | `[]` | バン済み pubkey 一覧 |
| `allowpubkey` | `[<pubkey>, <reason>]` | pubkey を許可 |
| `listallowedpubkeys` | `[]` | 許可済み pubkey 一覧 |
| `banevent` | `[<event-id>, <reason>]` | イベントをバン |
| `listbannedevents` | `[]` | バン済みイベント一覧 |
| `allowevent` | `[<event-id>, <reason>]` | イベントを許可 |
| `listeventsneedingmoderation` | `[]` | モデレーション待ちイベント |
| `changerelayname` | `[<new-name>]` | リレー名変更 |
| `changerelaydescription` | `[<new-desc>]` | 説明変更 |
| `changerelayicon` | `[<new-icon-url>]` | アイコン変更 |
| `allowkind` | `[<kind>]` | kind を許可 |
| `disallowkind` | `[<kind>]` | kind を禁止 |
| `listallowedkinds` | `[]` | 許可済み kind 一覧 |
| `blockip` | `[<ip>, <reason>]` | IP をブロック |
| `unblockip` | `[<ip>]` | IP ブロック解除 |
| `listblockedips` | `[]` | ブロック済み IP 一覧 |

## 実装ポイント

### 認証

NIP-98 Authorization ヘッダー:
```
Authorization: Nostr <base64-encoded-event>
```

- イベントには `payload` タグが必須
- `u` タグはリレー URL

### 認証失敗

```
HTTP/1.1 401 Unauthorized
```

### エラー処理

```json
{
  "result": null,
  "error": "invalid pubkey format"
}
```

## 使用例

### pubkey をバン

```bash
curl -X POST https://relay.example.com \
  -H "Content-Type: application/nostr+json+rpc" \
  -H "Authorization: Nostr <base64-event>" \
  -d '{
    "method": "banpubkey",
    "params": ["abc123...", "spam"]
  }'
```

レスポンス:
```json
{
  "result": true,
  "error": null
}
```

## NIP-11 での advertise

```json
{
  "supported_nips": [86, 98]
}
```

NIP-98 も必要（HTTP Auth）。

## 他の NIP との関連

- **NIP-11**: メタデータ変更が反映される
- **NIP-98**: HTTP Auth

## 参考

- [NIP-86 原文](https://github.com/nostr-protocol/nips/blob/master/86.md)
- [NIP-98: HTTP Auth](https://github.com/nostr-protocol/nips/blob/master/98.md)
