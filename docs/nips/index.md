# NIP 調査レポート

**最終更新**: 2026-01-24

## 概要

- **調査した NIP の総数**: 約 90 個
- **リレー実装に関係する NIP**: 16 個
  - 必須 (MUST): 1 個
  - 推奨 (SHOULD): 8 個
  - 任意 (MAY): 7 個

## リレー実装に必要な NIP

### 必須 (MUST)

リレーとして最低限動作するために必要。

| NIP | タイトル | 説明 |
|-----|---------|------|
| [NIP-01](nip-01.md) | Basic protocol flow description | Nostr の基本プロトコル。イベント構造、署名検証、メッセージ型、フィルタ、Kind の分類（regular/replaceable/ephemeral/addressable） |

### 推奨 (SHOULD)

実用的なリレーとして実装すべき。

| NIP | タイトル | 説明 |
|-----|---------|------|
| [NIP-09](nip-09.md) | Event Deletion Request | kind 5 による削除リクエスト。リレーは同一 pubkey のイベントを削除すべき |
| [NIP-11](nip-11.md) | Relay Information Document | リレー情報を HTTP で提供（metadata, limitations, retention, fees など） |
| [NIP-13](nip-13.md) | Proof of Work | PoW によるスパム対策。`nonce` タグと leading zero bits |
| [NIP-40](nip-40.md) | Expiration Timestamp | `expiration` タグによる自動削除。リレーは期限切れイベントを配信すべきでない |
| [NIP-42](nip-42.md) | Authentication of clients to relays | AUTH メッセージによる認証。kind 22242 イベント |
| [NIP-45](nip-45.md) | Event Counts | COUNT メッセージでイベント数を返す。高価なクエリの代替 |
| [NIP-50](nip-50.md) | Search Capability | REQ の `search` フィールドによる全文検索 |
| [NIP-70](nip-70.md) | Protected Events | `["-"]` タグによる再公開防止。認証した作者のみが公開可能 |

### 任意 (MAY)

あると便利だが必須ではない。

| NIP | タイトル | 説明 |
|-----|---------|------|
| [NIP-22](nip-22.md) | Comment | kind 1111 コメント機能。ブログ記事・ファイル・URL へのコメント |
| [NIP-28](nip-28.md) | Public Chat | kind 40-44 によるパブリックチャット。チャンネル作成・メタデータ・メッセージ・モデレーション |
| [NIP-29](nip-29.md) | Relay-based Groups | リレーが管理するクローズドグループ。メンバーシップ管理、モデレーション |
| [NIP-43](nip-43.md) | Relay Access Metadata and Requests | リレーのメンバーリスト（kind 13534）と参加リクエスト（kind 28934-28936） |
| [NIP-77](nip-77.md) | Negentropy Syncing | リレー間同期プロトコル。Range-Based Set Reconciliation で効率的に差分転送 |
| [NIP-86](nip-86.md) | Relay Management API | リレー管理 API（JSON-RPC over HTTP）。ban/allow, メタデータ変更など |

## クライアント専用・対象外の NIP

以下の NIP は主にクライアント側の実装や、リレーが単に保存するだけのイベント種別の定義です。

### クライアント実装

- **NIP-02**: Follow List（kind 3）
- **NIP-03**: OpenTimestamps Attestations
- **NIP-04**: Encrypted Direct Message（deprecated → NIP-17）
- **NIP-05**: DNS-based identifier mapping
- **NIP-06**: Key derivation from mnemonic
- **NIP-07**: window.nostr browser extension
- **NIP-08**: Handling Mentions（deprecated → NIP-27）
- **NIP-10**: Text Notes and Threads
- **NIP-14**: Subject tag
- **NIP-17**: Private Direct Messages
- **NIP-18**: Reposts
- **NIP-19**: bech32-encoded entities
- **NIP-21**: nostr: URI scheme
- **NIP-23**: Long-form Content
- **NIP-24**: Extra metadata fields
- **NIP-25**: Reactions
- **NIP-26**: Delegated Event Signing（unrecommended）
- **NIP-27**: Text Note References
- **NIP-30**: Custom Emoji
- **NIP-31**: Dealing with Unknown Events
- **NIP-32**: Labeling
- **NIP-34**: git stuff
- **NIP-35**: Torrents
- **NIP-36**: Sensitive Content
- **NIP-37**: Draft Events
- **NIP-38**: User Statuses
- **NIP-39**: External Identities in Profiles
- **NIP-44**: Encrypted Payloads
- **NIP-46**: Nostr Remote Signing
- **NIP-47**: Nostr Wallet Connect
- **NIP-48**: Proxy Tags
- **NIP-49**: Private Key Encryption
- **NIP-51**: Lists（kind 10000-10096, 30000-30078 など）
- **NIP-52**: Calendar Events
- **NIP-53**: Live Activities
- **NIP-54**: Wiki
- **NIP-55**: Android Signer Application
- **NIP-56**: Reporting
- **NIP-57**: Lightning Zaps
- **NIP-58**: Badges
- **NIP-59**: Gift Wrap
- **NIP-60**: Cashu Wallet
- **NIP-61**: Nutzaps
- **NIP-62**: Request to Vanish
- **NIP-64**: Chess (PGN)
- **NIP-65**: Relay List Metadata（kind 10002）
- **NIP-66**: Relay Discovery and Liveness Monitoring
- **NIP-68**: Picture-first feeds
- **NIP-69**: Peer-to-peer Order events
- **NIP-71**: Video Events
- **NIP-72**: Moderated Communities
- **NIP-73**: External Content IDs
- **NIP-75**: Zap Goals
- **NIP-78**: Application-specific data
- **NIP-7D**: Threads
- **NIP-84**: Highlights
- **NIP-85**: Ecash Mint Discoverability
- **NIP-87**: Ecash Mint Discoverability
- **NIP-88**: Polls
- **NIP-89**: Recommended Application Handlers
- **NIP-90**: Data Vending Machines
- **NIP-92**: Media Attachments
- **NIP-94**: File Metadata
- **NIP-96**: HTTP File Storage Integration（deprecated）
- **NIP-98**: HTTP Auth
- **NIP-99**: Classified Listings
- **NIP-A0**: Voice Messages
- **NIP-A4**: Public Messages
- **NIP-B0**: Web Bookmarks
- **NIP-B7**: Blossom
- **NIP-BE**: Nostr BLE Communications Protocol
- **NIP-C0**: Code Snippets
- **NIP-C7**: Chats
- **NIP-EE**: E2EE Messaging using MLS Protocol（unrecommended）

### 統合済み・移動済み

- **NIP-16**: Event Treatment → NIP-01 に統合
- **NIP-33**: Parameterized Replaceable Events → NIP-01 に統合（Addressable events に改名）

## 実装の優先順位

### Phase 1: 最小限の動作（NIP-01 のみ）

- NIP-01 の完全実装
- イベントの保存・配信
- Kind による分類（regular/replaceable/ephemeral/addressable）

### Phase 2: 実用的なリレー

- NIP-11: リレー情報の提供
- NIP-09: 削除リクエスト対応
- NIP-40: Expiration 対応
- NIP-13: PoW 検証（スパム対策）

### Phase 3: 高度な機能

- NIP-42: 認証
- NIP-45: COUNT
- NIP-50: 検索
- NIP-70: Protected Events

### Phase 4: 特殊機能（必要に応じて）

- NIP-29: Groups
- NIP-77: Negentropy
- NIP-86: Management API
- その他の任意機能

## 参考資料

- [NIP リポジトリ](https://github.com/nostr-protocol/nips)
- [Nostr Protocol](https://github.com/nostr-protocol/nostr)
