# NIP-43: Relay Access Metadata and Requests

**ステータス**: `draft` `optional` `relay`

## 概要

リレーのメンバーシップ管理。メンバーリスト公開、参加リクエスト、退出リクエスト。

## リレーの責務

### メンバーリスト公開（kind 13534）

- **リレーの `self` 鍵で署名**
- **`["-"]` タグ必須**（NIP-70 Protected Event）
- **`member` タグ**: 各メンバーの pubkey

```json
{
  "kind": 13534,
  "pubkey": "<nip11.self>",
  "tags": [
    ["-"],
    ["member", "c308...fb5"],
    ["member", "ee1d...958"]
  ]
}
```

### ユーザー追加（kind 8000）

```json
{
  "kind": 8000,
  "pubkey": "<nip11.self>",
  "tags": [
    ["-"],
    ["p", "c308...fb5"]
  ]
}
```

### ユーザー削除（kind 8001）

```json
{
  "kind": 8001,
  "pubkey": "<nip11.self>",
  "tags": [
    ["-"],
    ["p", "c308...fb5"]
  ]
}
```

### 参加リクエスト処理（kind 28934）

ユーザーが送信:
```json
{
  "kind": 28934,
  "tags": [
    ["-"],
    ["claim", "<invite-code>"]
  ]
}
```

リレーは OK で応答:
```
["OK", <event-id>, true, "info: welcome to wss://relay.example.com!"]
["OK", <event-id>, false, "restricted: that invite code is expired."]
```

### 招待コード発行（kind 28935）

リクエストされたら kind 28935 を生成（ephemeral range）:
```json
{
  "kind": 28935,
  "pubkey": "<nip11.self>",
  "tags": [
    ["-"],
    ["claim", "<invite-code>"]
  ]
}
```

### 退出リクエスト（kind 28936）

ユーザーが送信:
```json
{
  "kind": 28936,
  "tags": [["-"]]
}
```

リレーは kind 13534 を更新し、kind 8001 を発行 SHOULD。

## 実装ポイント

### 招待コードの管理

- **動的生成**: kind 28935 は ephemeral range（20000-29999）
- リクエストごとに異なるコードを発行可能
- 有効期限の設定可能

### メンバーシップ確認

両方を参照:
1. リレーの kind 13534
2. ユーザーの kind 10010（ユーザー側のリレーリスト）

### NIP-11 での advertise

```json
{
  "supported_nips": [43, 70]
}
```

NIP-70 も必要（Protected Events）。

## 他の NIP との関連

- **NIP-70**: Protected Events（`["-"]` タグ必須）
- **NIP-11**: `self` 公開鍵

## 参考

- [NIP-43 原文](https://github.com/nostr-protocol/nips/blob/master/43.md)
