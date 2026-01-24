# NIP-28: Public Chat

**ステータス**: `draft` `optional`

## 概要

kind 40-44 によるパブリックチャット。チャンネル作成、メタデータ、メッセージ、モデレーション。

## リレーの責務

### イベント保存

- **kind 40**: Channel create（regular）
- **kind 41**: Channel metadata（replaceable、`e` タグごとに最新のみ）
- **kind 42**: Channel message（regular）
- **kind 43**: Hide message（regular）
- **kind 44**: Mute user（regular）

### 特殊な処理

基本的に NIP-01 の通常のルールに従う。

- kind 41 は replaceable に近いが、`e` タグの値ごとに最新を保つ
- モデレーション（kind 43, 44）はクライアント側で処理

## イベント構造

### kind 40: チャンネル作成

```json
{
  "kind": 40,
  "content": "{\"name\": \"Demo Channel\", \"about\": \"A test channel.\", ...}"
}
```

### kind 41: メタデータ更新

```json
{
  "kind": 41,
  "tags": [
    ["e", "<channel_create_event_id>", "<relay-url>", "root"],
    ["t", "<category>"]
  ],
  "content": "{\"name\": \"Updated Demo Channel\", ...}"
}
```

### kind 42: メッセージ

```json
{
  "kind": 42,
  "tags": [
    ["e", "<kind_40_event_id>", "<relay-url>", "root"]
  ],
  "content": "Hello!"
}
```

## 実装ポイント

### kind 41 の保存

- `e` タグの値ごとに最新の kind 41 のみ保存
- 異なる `e` タグ値なら、複数の kind 41 を保存可能

### モデレーション

リレーは kind 43/44 を保存するだけ。クライアントが解釈：

- **kind 43**: 特定メッセージを非表示
- **kind 44**: 特定ユーザーをミュート

## 他の NIP との関連

- **NIP-10**: `e` タグのマーカー（root, reply）
- **NIP-29**: Relay-based Groups（よりリレー中心の設計）

## 参考

- [NIP-28 原文](https://github.com/nostr-protocol/nips/blob/master/28.md)
