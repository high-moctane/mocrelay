# NIP-11: Relay Information Document

**ステータス**: `draft` `optional` `relay`

## 概要

リレーのメタデータ、制限、ポリシーを JSON で提供する。HTTP で同じ URI にアクセス可能。

## リレーの責務

### HTTP エンドポイント

- **URL**: WebSocket と同じ URI
- **Content-Type**: `Accept: application/nostr+json` で JSON を返す
- **CORS**: `Access-Control-Allow-Origin` などのヘッダーを送信 MUST

### 提供すべき情報

#### 基本メタデータ

```json
{
  "name": "リレー名",
  "description": "説明文",
  "banner": "https://example.com/banner.png",
  "icon": "https://example.com/icon.jpg",
  "pubkey": "管理者の公開鍵",
  "self": "リレー自身の公開鍵",
  "contact": "mailto:admin@example.com",
  "supported_nips": [1, 9, 11, 13, 40, 42, 45, 50, 70],
  "software": "https://github.com/user/relay",
  "version": "1.0.0",
  "privacy_policy": "https://example.com/privacy.html",
  "terms_of_service": "https://example.com/tos.html"
}
```

#### 制限 (limitation)

```json
{
  "limitation": {
    "max_message_length": 16384,
    "max_subscriptions": 300,
    "max_limit": 5000,
    "max_subid_length": 100,
    "max_event_tags": 100,
    "max_content_length": 8196,
    "min_pow_difficulty": 30,
    "auth_required": false,
    "payment_required": false,
    "restricted_writes": false,
    "created_at_lower_limit": 31536000,
    "created_at_upper_limit": 3,
    "default_limit": 500
  }
}
```

#### 保持ポリシー (retention)

```json
{
  "retention": [
    {"kinds": [0, 1, [5, 7]], "time": 3600},
    {"kinds": [[30000, 39999]], "count": 1000}
  ]
}
```

#### 料金 (fees)

```json
{
  "payments_url": "https://relay.example.com/payments",
  "fees": {
    "admission": [{"amount": 1000000, "unit": "msats"}],
    "subscription": [{"amount": 5000000, "unit": "msats", "period": 2592000}],
    "publication": [{"kinds": [4], "amount": 100, "unit": "msats"}]
  }
}
```

## 実装ポイント

### supported_nips フィールド

- **含めるべき**: リレーが実装している NIP の番号（整数）
- **含めるべきでない**: クライアント専用の NIP

### limitation の意味

- **max_message_length**: WebSocket フレームの最大サイズ（バイト、UTF-8）
- **max_subscriptions**: 1 接続あたりの同時サブスクリプション数
- **max_limit**: フィルタの `limit` の上限値（リレーが自動的に clamp）
- **min_pow_difficulty**: NIP-13 PoW の最小難易度
- **auth_required**: 接続時に NIP-42 認証が必須
- **payment_required**: 支払いが必須
- **restricted_writes**: 何らかの条件でイベント受付を制限

### retention の仕組み

- **time**: 秒単位の保持期間（`null` = 無期限、`0` = 保存しない）
- **count**: 最大保存件数
- **kinds**: 対象の kind（範囲指定可能: `[40, 49]`）
- ephemeral イベント（20000-29999）は保持ポリシー不要

### エッジケース

- **未知のフィールド**: クライアントは無視すべき MUST
- **省略可能**: すべてのフィールドは省略可能

## 他の NIP との関連

- **NIP-13**: `min_pow_difficulty`
- **NIP-42**: `auth_required`
- **NIP-50**: `supported_nips` に 50 を含める

## 参考

- [NIP-11 原文](https://github.com/nostr-protocol/nips/blob/master/11.md)
