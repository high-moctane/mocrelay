# NIP-29: Relay-based Groups

**ステータス**: `draft` `optional` `relay`

## 概要

リレーが管理するクローズドグループ。メンバーシップ管理、モデレーション、グループメタデータをリレーが提供。

## リレーの責務

### グループ管理

- **グループ ID**: ランダムな文字列（`a-z0-9-_`）
- **グループ識別子**: `<host>'<group-id>`（例: `groups.nostr.com'abcdef`）
- **リレーが署名**: グループメタデータイベントはリレーの鍵で署名

### メンバーシップ管理

- **kind 9000**: ユーザー追加（`put-user`）
- **kind 9001**: ユーザー削除（`remove-user`）
- **kind 9021**: 参加リクエスト（ユーザーが送信）
- **kind 9022**: 退出リクエスト（ユーザーが送信）

### グループメタデータ（リレーが署名）

- **kind 39000**: グループメタデータ（`d` タグ = group-id）
- **kind 39001**: グループ管理者リスト
- **kind 39002**: グループメンバーリスト
- **kind 39003**: グループロール定義

### イベント検証

- **`h` タグ必須**: グループに送信されるイベントは `["h", "<group-id>"]` を含む MUST
- **メンバーシップチェック**: メンバーのみがイベントを投稿可能
- **timeline references**: `previous` タグで過去イベントを参照（コンテキスト外での使用を防ぐ）

## 実装ポイント

### グループの作成

- **kind 9007**: create-group（管理者が送信）
- リレーは kind 39000 を生成・署名

### モデレーションイベント

| kind | 名前 | 引数 |
|------|------|------|
| 9000 | put-user | `p` タグ（pubkey とロール） |
| 9001 | remove-user | `p` タグ（pubkey） |
| 9002 | edit-metadata | メタデータフィールド |
| 9005 | delete-event | `e` タグ（event id） |
| 9007 | create-group | なし |
| 9008 | delete-group | なし |
| 9009 | create-invite | `code` タグ |

### unmanaged グループ

- NIP-29 非対応リレーでも使用可能
- 全員がメンバー扱い
- モデレーションなし

## エッジケース

### Late publication

- 過去のタイムスタンプのイベントを拒否すべき SHOULD
- 例外: グループを別リレーから移行する場合

### Timeline references

- `previous` タグに存在しないイベント ID → 拒否
- クライアントもこれをチェックしてリレーの誠実性を検証すべき

## 他の NIP との関連

- **NIP-70**: Protected Events（`["-"]` タグの使用を推奨）
- **NIP-42**: 認証（メンバーシップ確認）
- **NIP-51**: kind 10009（グループリスト）

## 参考

- [NIP-29 原文](https://github.com/nostr-protocol/nips/blob/master/29.md)
