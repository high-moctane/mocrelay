# NIP-77: Negentropy Syncing

**ステータス**: `draft` `optional` `relay`

## 概要

リレー間同期プロトコル。Range-Based Set Reconciliation で効率的に差分を転送。

## リレーの責務

### メッセージサポート

- **NEG-OPEN**: 同期開始（client → relay）
- **NEG-MSG**: 同期メッセージ（双方向）
- **NEG-ERR**: エラー応答（relay → client）
- **NEG-CLOSE**: 同期終了（client → relay）

### フロー

1. Client: `["NEG-OPEN", <sub_id>, <filter>, <initialMessage>]`
2. Relay: フィルタに一致するイベントセットを準備
3. Relay: `["NEG-MSG", <sub_id>, <message>]` で応答
4. Client: 必要なら `["NEG-MSG", <sub_id>, <message>]` で続行
5. Client: `["NEG-CLOSE", <sub_id>]` で終了

### イベント転送

Negentropy は ID のみを同期:
- Client は必要な ID を学習
- 実際のイベント転送は `EVENT` / `REQ` で行う

## 実装ポイント

### Negentropy Protocol

- **バイナリプロトコル**: hex エンコードして転送
- **Protocol Version 1**: バイト `0x61`
- **Fingerprint**: SHA256 の最初 16 バイト

### エラーハンドリング

```json
["NEG-ERR", <sub_id>, "blocked: this query is too big"]
["NEG-ERR", <sub_id>, "closed: you took too long to respond!"]
```

### リソース管理

- **タイムアウト**: 非アクティブなクエリを閉じる MAY
- **クエリサイズ制限**: 大きすぎるクエリを拒否可能

## 使用例

### リレー間同期

```
relay1: ["NEG-OPEN", "sync_1", {}, "<hex-encoded-message>"]
relay2: ["NEG-MSG", "sync_1", "<hex-encoded-message>"]
relay1: ["NEG-MSG", "sync_1", "<hex-encoded-message>"]
relay2: ["NEG-MSG", "sync_1", "<hex-encoded-message>"]
relay1: ["NEG-CLOSE", "sync_1"]
```

### カウント専用

イベント転送せず、ユニークなイベント数だけ知りたい場合にも使用可能。

## NIP-11 での advertise

```json
{
  "supported_nips": [77]
}
```

## 他の NIP との関連

- **NIP-01**: フィルタ構文

## 参考

- [NIP-77 原文](https://github.com/nostr-protocol/nips/blob/master/77.md)
- [Negentropy Protocol](https://github.com/hoytech/negentropy)
- [Range-Based Set Reconciliation](https://logperiodic.com/rbsr.html)
