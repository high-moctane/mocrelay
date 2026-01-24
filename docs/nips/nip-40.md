# NIP-40: Expiration Timestamp

**ステータス**: `draft` `optional` `relay`

## 概要

`expiration` タグによるイベントの自動削除。Unix タイムスタンプで期限を指定。

## リレーの責務

### 期限切れイベントの扱い

- **配信停止**: 期限切れイベントを配信すべきでない SHOULD NOT
- **受信拒否**: 期限切れイベントを受信時に拒否すべき SHOULD
- **削除**: 即座に削除しなくてもよい MAY NOT、無期限保存してもよい MAY
  - ただし、配信はしない

### エッジケース

- **Ephemeral イベント**: expiration は影響しない（もともと保存しない）
- **過去の expiration**: 受信時に既に期限切れの場合は即座に拒否

## 実装ポイント

### expiration タグの構造

```json
{
  "tags": [
    ["expiration", "1600000000"]
  ]
}
```

- Unix タイムスタンプ（秒）、文字列形式
- `created_at` と同じフォーマット

### バリデーション

```go
func isExpired(event Event) bool {
    for _, tag := range event.Tags {
        if len(tag) >= 2 && tag[0] == "expiration" {
            expiration, err := strconv.ParseInt(tag[1], 10, 64)
            if err != nil {
                return false
            }
            return time.Now().Unix() > expiration
        }
    }
    return false
}
```

### NIP-11 での advertise

```json
{
  "supported_nips": [40]
}
```

クライアントは `supported_nips` を確認してから expiration イベントを送るべき SHOULD。

## 削除のタイミング

実装の選択肢：

1. **即座削除**: 期限到達時に削除（リソース効率的）
2. **遅延削除**: 配信はしないが、削除は後で（実装簡単）
3. **保存継続**: 配信しないが保存は続ける（デバッグ用）

推奨: (2) 遅延削除が実装しやすい。

## セキュリティ上の注意

**expiration は秘密保持の手段ではない**:

- イベントは公開されている間、第三者がダウンロード可能
- 期限到達前にコピーされる可能性
- 削除されてもすべてのリレーから消えるとは限らない

## 他の NIP との関連

- **NIP-09**: 削除リクエスト（手動削除、理由付き）
- **NIP-01**: Ephemeral イベント（保存しない）

## 参考

- [NIP-40 原文](https://github.com/nostr-protocol/nips/blob/master/40.md)
