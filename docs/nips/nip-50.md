# NIP-50: Search Capability

**ステータス**: `draft` `optional` `relay`

## 概要

REQ フィルタに `search` フィールドを追加し、全文検索を提供。

## リレーの責務

### search フィルタの処理

**リクエスト**:
```json
["REQ", "search_1", {
  "search": "best nostr apps",
  "kinds": [1],
  "limit": 20
}]
```

### 検索対象

- **必須**: `content` フィールド SHOULD
- **オプション**: 他のフィールド（kind に応じて） MAY
  - 例: kind 0 の metadata では `name`, `about` なども検索対象にできる

### ソート順

- **検索スコア降順** SHOULD（`created_at` 降順ではない）
- `limit` はソート後に適用

### 拡張構文

`key:value` 形式の拡張をサポート可能（オプション）:

- `include:spam`: スパムフィルタを無効化
- `domain:<domain>`: NIP-05 ドメインでフィルタ
- `language:<ISO 639-1>`: 言語でフィルタ（例: `language:en`）
- `sentiment:<negative/neutral/positive>`: 感情分析
- `nsfw:<true/false>`: NSFW コンテンツの扱い

### スパムフィルタ

- デフォルトでスパムを除外すべき SHOULD
- `include:spam` で無効化可能

## 実装ポイント

### 検索エンジンの選択

**オプション**:
1. **PostgreSQL Full-Text Search**: 中規模リレー向け
2. **Elasticsearch / OpenSearch**: 大規模リレー向け
3. **SQLite FTS5**: 小規模リレー向け
4. **単純な LIKE**: 最小実装（非推奨、遅い）

### PostgreSQL 例

```sql
-- tsvector カラムの追加
ALTER TABLE events ADD COLUMN content_tsv tsvector;
CREATE INDEX idx_content_tsv ON events USING gin(content_tsv);

-- 検索クエリ
SELECT * FROM events
WHERE content_tsv @@ to_tsquery('best & nostr & apps')
ORDER BY ts_rank(content_tsv, to_tsquery('best & nostr & apps')) DESC
LIMIT 20;
```

### 検索スコア

実装依存だが、以下を考慮:
- キーワードの出現頻度
- 位置（タイトル vs 本文）
- 鮮度（新しいイベントを優先する場合）

### エッジケース

- **空の検索文字列**: すべてのイベントを返す（limit 適用）
- **特殊文字**: エスケープ処理
- **非常に長いクエリ**: 制限を設ける

## NIP-11 での advertise

```json
{
  "supported_nips": [50]
}
```

クライアントは `supported_nips` を確認してから search を送るべき SHOULD。

## クライアントのベストプラクティス

- **複数リレーにクエリ**: 検索実装はリレーごとに異なるため
- **結果の検証**: リレーの検索品質を評価し、低品質なリレーを除外 MAY
- **検索結果の重複除外**: 複数リレーからの結果をマージ

## 他の NIP との関連

- **NIP-01**: フィルタ構文
- **NIP-05**: `domain:` 拡張

## 参考

- [NIP-50 原文](https://github.com/nostr-protocol/nips/blob/master/50.md)
