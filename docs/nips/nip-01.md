# NIP-01: Basic protocol flow description

**ステータス**: `final` `mandatory` `relay`

## 概要

Nostr の基本プロトコル。すべてのリレーとクライアントが実装すべき必須仕様。

## リレーの責務

### イベントの保存ルール

**Kind の分類**（kind `n` による）:

1. **Regular**（通常イベント）: すべて保存
   - `1000 <= n < 10000 || 4 <= n < 45 || n == 1 || n == 2`

2. **Replaceable**（置き換え可能）: `(pubkey, kind)` ごとに最新のみ保存
   - `10000 <= n < 20000 || n == 0 || n == 3`

3. **Ephemeral**（一時的）: 保存しない、配信のみ
   - `20000 <= n < 30000`

4. **Addressable**（アドレス指定可能）: `(pubkey, kind, d-tag)` ごとに最新のみ保存
   - `30000 <= n < 40000`

### イベント検証

- **ID 検証**: SHA256(serialized event) が `id` フィールドと一致
- **署名検証**: Schnorr 署名（secp256k1, BIP-340）
- **JSON シリアライズ**: 厳密なルール（余分なフィールド不可、特定の文字のみエスケープ）

### フィルタ処理

- **単一文字タグのインデックス**: `{"#e": ["<event-id>"]}` のようなクエリに対応
  - タグ配列の最初の値のみをインデックス化
  - a-z, A-Z のすべての単一文字タグ

- **limit**: 初期クエリにのみ適用、`created_at` 降順でソート

### メッセージ処理

**Client → Relay**:
- `["EVENT", <event>]`: イベント送信 → `OK` で応答
- `["REQ", <sub_id>, <filter>...]`: サブスクリプション開始 → `EVENT`/`EOSE` で応答
- `["CLOSE", <sub_id>]`: サブスクリプション終了

**Relay → Client**:
- `["EVENT", <sub_id>, <event>]`: イベント配信
- `["OK", <event_id>, <bool>, <message>]`: イベント受信結果
- `["EOSE", <sub_id>]`: 保存済みイベント送信完了
- `["CLOSED", <sub_id>, <message>]`: サブスクリプション終了通知
- `["NOTICE", <message>]`: 人間可読メッセージ

## 実装ポイント

### JSON パース

`encoding/json` では不十分。以下の理由で手書き実装が必要：

- 余分なフィールドを拒否
- 特定の文字のみエスケープ（`\n`, `\"`, `\\`, `\r`, `\t`, `\b`, `\f`）
- 空白・改行を含まない厳密なシリアライズ

### OK/CLOSED プレフィックス

標準プレフィックス（NIP-01 定義）:
- `duplicate`: 重複
- `pow`: PoW 不足
- `blocked`: ブロック済み
- `rate-limited`: レート制限
- `invalid`: 無効
- `restricted`: 制限付き
- `mute`: ミュート
- `error`: エラー

### エッジケース

- **created_at**: 未来や過去すぎるタイムスタンプの扱い
- **tags**: 空配列、null 要素の扱い
- **content**: 非常に長い content の制限
- **同時サブスクリプション**: 同じ sub_id の重複リクエスト

## 他の NIP との関連

- **NIP-16**: Event Treatment → NIP-01 に統合済み
- **NIP-33**: Parameterized Replaceable Events → NIP-01 に統合済み（Addressable events）
- **NIP-10**: kind 1（text note）の詳細
- **NIP-42**: AUTH メッセージの追加

## 参考

- [NIP-01 原文](https://github.com/nostr-protocol/nips/blob/master/01.md)
- [BIP-340: Schnorr Signatures](https://bips.xyz/340)
