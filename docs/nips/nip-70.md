# NIP-70: Protected Events

**ステータス**: `draft` `optional` `relay`

## 概要

`["-"]` タグによる再公開防止。作者のみがイベントを公開できる。

## リレーの責務

### デフォルト動作

**`["-"]` タグを含むイベントは拒否すべき MUST**

### Protected Events のサポート

サポートする場合：

1. **NIP-42 AUTH を要求**
2. **pubkey の一致確認**: 認証された pubkey とイベントの pubkey が一致する場合のみ受理
3. **不一致の場合は拒否**: `["OK", <event-id>, false, "auth-required: this event may only be published by its author"]`

### kind 22242 の除外

- Protected events に限らず、kind 22242 は配信禁止（NIP-42 の要件）

## 実装ポイント

### フロー例

```
client: ["EVENT", {
  "id": "cb8f...",
  "pubkey": "79be...",
  "tags": [["-"]],
  ...
}]

relay: ["AUTH", "<challenge>"]
relay: ["OK", "cb8f...", false, "auth-required: this event may only be published by its author"]

client: ["AUTH", {...}]  // 認証
relay: ["OK", "abcd...", true, ""]

client: ["EVENT", {...}]  // 再送
relay: ["OK", "cb8f...", true, ""]
```

### 検証ロジック

```go
func canAcceptProtectedEvent(event Event, session *Session) bool {
    hasProtectedTag := false
    for _, tag := range event.Tags {
        if len(tag) == 1 && tag[0] == "-" {
            hasProtectedTag = true
            break
        }
    }

    if !hasProtectedTag {
        return true  // 通常イベント
    }

    // Protected event: 認証済み pubkey と一致するか
    for _, authedPubkey := range session.AuthedPubkeys {
        if authedPubkey == event.Pubkey {
            return true
        }
    }

    return false
}
```

### NIP-11 での advertise

```json
{
  "supported_nips": [42, 70]
}
```

NIP-70 をサポートするには NIP-42 が必須。

## 使用例

### クローズドグループ

- グループメンバーのみが参加できるリレー
- メンバーは自分のイベントに `["-"]` を付ける
- リレーはメンバー以外の再公開を防ぐ

### セミプライベートフィード

- 情報は公開だが、特定のリレーに限定したい
- 無制限な拡散を防ぐ

## 限界

**完全な再公開防止は不可能**:

- グループメンバーが意図的に再公開する可能性
- リレーは「海賊行為」を防ぐ手段を提供するだけ
- 作者の意図を尊重するリレーが協力する

## 他の NIP との関連

- **NIP-42**: 認証（必須）
- **NIP-29**: Relay-based Groups（`["-"]` の使用を推奨）

## 参考

- [NIP-70 原文](https://github.com/nostr-protocol/nips/blob/master/70.md)
