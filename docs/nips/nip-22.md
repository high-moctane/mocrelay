# NIP-22: Comment

**ステータス**: `draft` `optional`

## 概要

kind 1111 によるコメント機能。ブログ記事、ファイル、URL などにコメント可能。

## リレーの責務

### イベント保存

- **kind 1111 を regular イベントとして保存**
- NIP-01 の通常のルールに従う

### 特殊な処理は不要

- リレーは kind 1111 を特別扱いする必要なし
- クライアント側でタグを解釈してスレッド構造を構築

## イベント構造

### タグの規則

- **大文字タグ**: ルートスコープ（`E`, `A`, `I`, `K`, `P`）
- **小文字タグ**: 親アイテム（`e`, `a`, `i`, `k`, `p`）
- **K と k**: kind を明示（必須）

### 例: ブログ記事へのコメント

```json
{
  "kind": 1111,
  "content": "Great blog post!",
  "tags": [
    ["A", "30023:3c98...1289:f9347ca7", "wss://example.relay"],
    ["K", "30023"],
    ["P", "3c98...1289", "wss://example.relay"],
    ["a", "30023:3c98...1289:f9347ca7", "wss://example.relay"],
    ["e", "5b4f...651", "wss://example.relay"],
    ["k", "30023"],
    ["p", "3c98...1289", "wss://example.relay"]
  ]
}
```

## 実装ポイント

### kind 1 との違い

**kind 1111 を kind 1 の返信に使ってはいけない MUST NOT**

- kind 1 へのリプライは NIP-10 に従う
- kind 1111 は他のコンテンツタイプ用

### インデックス

通常の単一文字タグのインデックス（`#e`, `#a`, `#p` など）で十分。

## 他の NIP との関連

- **NIP-10**: kind 1 のスレッド（使い分け）
- **NIP-73**: External Content IDs（`I` タグ）
- **NIP-23**: Long-form Content（コメント対象）

## 参考

- [NIP-22 原文](https://github.com/nostr-protocol/nips/blob/master/22.md)
