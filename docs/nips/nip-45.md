# NIP-45: Event Counts

**ステータス**: `draft` `optional` `relay`

## 概要

COUNT メッセージでイベント数を返す。フォロワー数などの集計に便利。

## リレーの責務

### COUNT リクエストの処理

**リクエスト**:
```json
["COUNT", <query_id>, <filters JSON>...]
```

**レスポンス**:
```json
["COUNT", <query_id>, {"count": <integer>}]
```

または近似値の場合:
```json
["COUNT", <query_id>, {"count": <integer>, "approximate": true}]
```

### フィルタの扱い

- NIP-01 の REQ と同じフィルタ構文
- **複数フィルタは OR**: すべてのフィルタにマッチするイベントの合計数
- **重複除外**: 同じイベントは1回だけカウント

### 拒否

COUNT を拒否する場合は CLOSED で応答:

```json
["CLOSED", <query_id>, "auth-required: cannot count other people's DMs"]
```

## 実装ポイント

### 確率的カウント

- リレーは正確なカウントでなくてもよい MAY（計算コスト削減）
- 近似値の場合は `"approximate": true` を含める SHOULD

### パフォーマンス最適化

```sql
-- 正確なカウント（重い）
SELECT COUNT(*) FROM events WHERE ...

-- 近似カウント（軽い）
SELECT reltuples::bigint FROM pg_class WHERE relname = 'events';
```

### エッジケース

- **空フィルタ**: 全イベント数を返す（リレーが許可する場合）
- **認証が必要なクエリ**: DM など、AUTH なしでは拒否

## 使用例

### フォロワー数

```json
["COUNT", "followers", {"kinds": [3], "#p": ["<pubkey>"]}]
→ ["COUNT", "followers", {"count": 238}]
```

### 投稿数とリアクション数

```json
["COUNT", "stats", {"kinds": [1, 7], "authors": ["<pubkey>"]}]
→ ["COUNT", "stats", {"count": 5}]
```

### 近似カウント

```json
["COUNT", "all_notes", {"kinds": [1]}]
→ ["COUNT", "all_notes", {"count": 93412452, "approximate": true}]
```

## NIP-11 での advertise

```json
{
  "supported_nips": [45]
}
```

## クライアントの利点

**COUNT を使わない場合**:
- フォロワー数を取得するには kind:3 イベントをすべてダウンロード
- 帯域幅の無駄、レイテンシー増加

**COUNT を使う場合**:
- 単一のリクエスト・レスポンスで完結
- 軽量、高速

## 他の NIP との関連

- **NIP-01**: フィルタ構文
- **NIP-42**: 認証（DM のカウントなど）

## 参考

- [NIP-45 原文](https://github.com/nostr-protocol/nips/blob/master/45.md)
