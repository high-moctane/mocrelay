# mocrelay リファクタリング・改善計画

## 🎯 プロジェクトの目的

放置していた mocrelay のコード品質を洗練させる。

**背景**：
- 動くけど、奇をてらった実装がある
- 将来的には人におすすめできるクオリティにしたい
- 当面は自分で運用するため

**方針**：
- 奇をてらった実装 → 素直な実装に
- テストを充実
- CI を整備

---

## 📋 現状の問題点（把握済み）

### 1. neo_handler.go（調査完了 ✅）

**問題**：
- Go 1.23 の range over func (iterator) で SimpleHandler を書き直そうとした
- コンパイラが unreachable コードを認識できない問題でイライラ
- iterator は channel + context パターンに不向き
- 命名にも自信がない（`OnStart`, `OnEnd` の hook 的な意図）
- 結果：途中で止まった

**結論**：
- **削除する**
- 既存の SimpleHandler（素直な for + select）を使う
- iterator は本末転倒

---

### 2. インメモリ event store（調査完了 ✅）

**問題**：
- もともと skip list + optimistic locking + CAS で実装しようとした（曲芸！）
- 時間がかかって relay 作れない → 本末転倒
- 急遽 TreeMap (外部ライブラリ) で妥協 → 回りくどい実装になった

**複雑さの原因**：
- REQ メッセージに高速対応したかった
- 素朴な実装の問題：全件ループ → ソート → limit（重い）
- 最新10件欲しいのに1万件ヒットしたら無駄
- 解決策：4つのデータ構造を同期管理（`evs`, `evsCreatedAt`, `evsIndex`, `deleted`）
- Find のロジックが超複雑（intersection で絞り込み）
- type-unsafe (`any`) を使う羽目に

**現状の変化**：
- 当初：メインストレージとして1万件保持 → 最適化が重要
- その後：SQLite 実装が登場 → キャッシュ用途のみに
- 現在：運用重視、気軽に動かせるのが最優先

**結論**：
- **素朴な実装に書き直す**
- キャッシュ用途なら複雑な最適化は不要
- シンプルな map + filter + sort で十分
- 読みやすさ・保守性を優先
- コード行数：450行 → 100行程度に削減見込み

---

### 3. sqlite 実装（調査完了 ✅）

**複雑さの原因**：
- REQ filter の仕様と戦おうとした結果
- すべての filter 条件で index を使える + 時系列順取得を両立
- テーブルフルスキャンを回避（巨大テーブル対応）

**スキーマ設計**：
- `events`: メタデータ（event_key, id, pubkey, created_at, kind）
  - 各条件ごとに複合インデックス（id+created_at, pubkey+created_at, kind+created_at）
- `event_payloads`: ペイロード分離（tags, content, sig）
- `event_tags`: タグ検索用（tag_hash=MD5(key+value), created_at, event_key）
- `deleted_event_keys` & `deleted_event_ids`: Kind 5 削除イベント対応

**クエリ戦略**（query.go）：
- 各 filter 条件ごとにサブクエリを作成
- 同じ events テーブルに何度も JOIN → 各インデックスを活用
- goqu (クエリビルダ) で SQL を動的生成
- 全サブクエリを OR でまとめる

**素晴らしい点**：
- 理論的に正しい設計
- インデックス戦略が優れている
- スケーラビリティを考慮
- SQLite 一本で勝負（VPS 運用で DB サーバー不要）

**問題点**：
- **goqu がすごくつらい**（328行、超複雑）
- コードが読みづらく、保守が大変
- 完璧を目指しすぎた？

**背景**：
- テーブルは巨大になる一方
- VPS 上で動かす → サーバークライアント型 DB は億劫
- SQLite 一本で勝負してみる実験
- スキーマをちゃんと考えた結果

**検討事項**：
- goqu を捨てて生 SQL で書く？
- スキーマを少し緩めてシンプルなクエリに？
- 他の選択肢（KVS, Parquet）？
- でも SQLite は捨てがたい（運用の簡単さ）

---

### 4. event 構造体（調査完了 ✅）

**構造**：
```go
type Event struct {
    ID        string `json:"id"`
    Pubkey    string `json:"pubkey"`
    CreatedAt int64  `json:"created_at"`
    Kind      int64  `json:"kind"`
    Tags      []Tag  `json:"tags"`  // Tag = []string
    Content   string `json:"content"`
    Sig       string `json:"sig"`
}
```

**特徴**：
- Nostr プロトコルの標準的な Event 構造そのまま
- シンプルで分かりやすい
- カスタム UnmarshalJSON で厳密にフィールド数チェック（7個）
- 必要なメソッド完備（EventType, Valid, Verify, Serialize 等）

**結論**：
- **問題なし！** 💯
- 直感通り「そんなに問題ない」
- 変な奇をてらった実装もない
- このままでOK

---

### 5. handler/middleware 全般（調査完了 ✅）

**Handler たち**：
- SimpleHandler（36-91行）- 素直、問題なし
- RouterHandler（132-280行）- pub/sub、問題なし
- CacheHandler（282-392行）- EventCache を使用、Handler 自体は素直
- MergeHandler（394-694行）- 複雑だけど、性質上仕方ない、理にかなってる

**Middleware たち**（910-1978行、大量）：
- NIP-11 の limitation に対応する標準的な実装
- MaxSubscriptions, MaxReqFilters, MaxLimit, MaxEventTags, MaxContentLength
- CreatedAtLowerLimit, CreatedAtUpperLimit
- RecvEventUniqueFilter, SendEventUniqueFilter
- RecvEventAllowFilter, RecvEventDenyFilter
- LoggingMiddleware
- どれもシンプルで素直

**safeMap**（data_structure.go, 49行）：
- sync.RWMutex で保護された concurrent-safe map
- Get, TryGet, Add, Delete, Loop のメソッド
- 素直な実装

**結論**：
- **特に奇をてらった実装はなし！** 💯
- どれも素直で読みやすい
- MergeHandler だけ複雑だけど、理にかなってる

---

### 6. その他

**検討事項**：
- Makefile の整備
- CI の整備
- testing/synctest の活用可能性

---

## 🗓️ 作業ログ

### 2025-10-27（続き）

#### MergeHandler の詳細調査（完了 ✅）

**調査内容**：
- パインちゃんに MergeHandler の実装を詳しく読んでもらった
- handler.go の 394-909行（515行）を徹底分析

**発見**：
1. **channel を mutex として使用**（Goのアンチパターン）
   - `s := <-ss.okStat; defer func() { ss.okStat <- s }()` パターン
   - 読みにくい、`sync.Mutex` の方が意図が明確

2. **再帰的goroutine生成**
   - `runHandlers` と `mergeSend` が再帰的にN-1個のgoroutineを生成
   - 理解しにくい、デバッグ困難

3. **35行の巨大関数** `IsSendableEventMsg`
   - 5つの異なる責務を1つの関数に詰め込みすぎ
   - 小さな関数に分割すべき

4. **マージソートの欠如**
   - 各ハンドラーが降順で送る前提、ハンドラー間の順序保証が不十分

5. **複雑な状態マシン**
   - 3つの独立した状態（okStat, reqStat, countStat）をchannelで管理
   - 全体的に**状態管理がすごくつらい**

**ユーザーの記憶**：
- 「mutex でいいのになぜか chan を使ってるのが変だった」
- 「全体的に状態管理がすごくつらかった」
- パインちゃんの分析と完全一致！

**改善案**：
- 優先度高：channel → Mutex、関数分割
- 優先度中：ドキュメント追加
- 優先度低：マージソート実装、インターフェース見直し

**決定**：
- Phase 2 に MergeHandler のリファクタリングを追加
- 段階的に改善していく方針

---

### 2025-10-27

#### neo_handler.go の調査（完了 ✅）

**調査内容**：
- neo_handler.go を読んだ
- 既存の SimpleHandler と比較
- `recvCtx` (utils.go) の実装を確認

**発見**：
- Go 1.23 の range over func (iterator) を使った実装
- 既存の冗長な for + select を簡潔にしたかった
- でも、コンパイラが unreachable コードを認識できない問題
- エラーハンドリングも複雑になった（無限 yield ループ）
- 結果：iterator は channel + context パターンに不向き

**決定**：
- neo_handler.go を削除する
- 既存の SimpleHandler を使い続ける

---

#### インメモリ event store の調査（完了 ✅）

**調査内容**：
- `event_cache.go` (450行) を読んだ
- データ構造を確認：4つのデータ構造を同期管理
  - `evs`: map[string]*Event（メインストレージ）
  - `evsCreatedAt`: TreeMap（時系列ソート維持）
  - `evsIndex`: 独自マルチインデックス（ID, Author, Kind, Tag）
  - `deleted`: Kind 5 削除追跡

**発見した回りくどさ**：
1. **Find のロジックが超複雑**（381-449行）
   - keysFromReqFilter → 各 key slice → map[*Event]bool 作成
   - サイズでソート → intersection で絞り込み → limit 処理
   - 普通に書けばもっとシンプル

2. **type-unsafe な eventCacheEvsIndexKey**
   - `What int8` + `Value any` で型を区別
   - interface{} を使用、type-safe じゃない

3. **TreeMap (外部ライブラリ) の使用**
   - skip list の代わりに Red-Black Tree で妥協
   - 常に時系列ソート維持、ソート不要にしたかった

**背景理解**：
- REQ メッセージ高速化のための最適化
- 当初はメインストレージ（1万件）として使う想定
- SQLite 登場後、キャッシュ用途のみに変化
- 現在は運用重視、気軽に動かせるのが最優先

**決定**：
- 素朴な実装（map + filter + sort）に書き直す
- 450行 → 100行程度に削減見込み
- 読みやすさ・保守性を優先

---

#### sqlite 実装の調査（完了 ✅）

**調査内容**：
- `handler/sqlite/handler.go` (236行) を読んだ
- `handler/sqlite/query.go` (328行) を読んだ
- `handler/sqlite/migrate.go` (113行) を読んだ
- スキーマとクエリ戦略を確認

**発見**：
1. **Bulk Insert 戦略**（handler.go）
   - goroutine でバッファリング → まとまったら INSERT
   - LRU で重複排除、3回リトライ
   - xxHash seed: Hash Flooding 攻撃対策
   - WAL checkpoint: 定期実行でパフォーマンス維持

2. **複雑なクエリ構築**（query.go）
   - goqu (クエリビルダ) で SQL を動的生成
   - 各 filter 条件ごとにサブクエリ作成
   - 同じ events テーブルに何度も JOIN → 各インデックス活用
   - **328行、超複雑、読みづらい**

3. **スキーマ設計**（migrate.go）
   - 正規化：events（メタデータ） + event_payloads（ペイロード）
   - 複合インデックス：id+created_at, pubkey+created_at, kind+created_at
   - event_tags: タグを MD5 ハッシュ化して保存
   - 削除イベント用に2テーブル（event_key 用、id 用）

**背景理解**：
- REQ filter の仕様と戦おうとした
- すべての条件で index を使える + 時系列順取得を両立
- テーブルフルスキャンを回避（巨大テーブル対応）
- SQLite 一本で勝負（VPS 運用、DB サーバー不要）

**問題点**：
- **goqu がすごくつらい**
- 完璧を目指しすぎた
- 運用重視の今なら、もっとシンプルでいいかも

**決定（検討事項）**：
- goqu を捨てて生 SQL？
- スキーマを少し緩める？
- SQLite は捨てがたい（運用の簡単さ）

---

## 📝 メモ・気づき

- 英語の命名センスに自信がない → レビュー時に命名も見直す
- 奇をてらった実装の典型パターン：野心的な設計 → 時間かかる → 妥協 → 変な形に
- でも SQLite 実装は「妥協」ではなく「完璧を目指しすぎた」
- 素直な実装が一番！でも、理論的に正しい設計も捨てがたい

---

## 🔍 次のステップ

1. ✅ neo_handler.go 調査完了
2. ✅ インメモリ event store 調査完了
3. ✅ sqlite 実装調査完了
4. ✅ event 構造体確認
5. ✅ handler/middleware レビュー
6. ✅ **MergeHandler 詳細調査完了**（パインちゃんによる徹底分析）
7. ✅ 改善計画のファイナライズ（このセクションで完了）
8. ⏳ 実装開始（次回以降）

---

## 📊 調査結果のまとめ（2025-10-27）

### 🔴 要改善（奇をてらった実装）

**1. neo_handler.go（76行）**
- 問題：Go 1.23 iterator、コンパイラの問題、本末転倒
- 対応：**削除**（既存の SimpleHandler を使う）
- 優先度：⭐️⭐️（すぐ削除できる）

**2. event_cache.go（450行）**
- 問題：早すぎる最適化、4つのデータ構造を同期管理、超複雑
- 対応：**素朴な実装に書き直し**（map + filter + sort）
- 見込み：450行 → 100行
- 優先度：⭐️⭐️⭐️（キャッシュ用途なら簡単に）

**3. handler/sqlite/query.go（328行）**
- 問題：goqu がすごくつらい、超複雑、読みづらい
- 背景：理論的には正しい設計、完璧を目指しすぎた
- 対応案：
  - A. goqu を捨てて生 SQL で書く
  - B. スキーマを少し緩めてシンプルなクエリに
  - C. そのまま（運用で問題なければ）
- 優先度：⭐️（SQLite は捨てがたい、慎重に判断）

**4. MergeHandler（handler.go, 515行: 394-909行）**
- 問題：**状態管理がすごくつらい**、「奇をてらった実装」がある
- 具体的な問題点：
  - **channel を mutex として使用**（❌ Goのアンチパターン）
    - `s := <-ss.okStat; defer func() { ss.okStat <- s }()` パターン
    - `sync.Mutex` を使うべき、その方が意図が明確
  - **再帰的goroutine生成**（理解しにくい、デバッグ困難）
  - **35行の巨大関数** `IsSendableEventMsg`（5つの責務を詰め込みすぎ）
  - **マージソートの欠如**（各ハンドラーが降順で送る前提、順序保証が不十分）
  - **複雑な状態マシン**（3つの独立した状態をchannelで管理）
- 背景：複数ハンドラーのイベントマージは本質的に難しい
- 対応案（優先度順）：
  - A. **channel → Mutex へ置き換え**（高優先度、可読性↑、リスク低）
  - B. **`IsSendableEventMsg` を小さな関数に分割**（高優先度、可読性↑）
  - C. ドキュメントとコメント追加（中優先度、メンテナンス性↑）
  - D. マージソートの明示的実装（検討、正確性↑、複雑性↑）
  - E. インターフェース設計の見直し（低優先度、大規模変更）
- 優先度：⭐️⭐️（改善の効果は大きいが、慎重に進める）
- 参考：パインちゃんの詳細分析レポート（2025-10-27）

---

### 🟢 問題なし（そのまま）

**5. Event 構造体（message.go）**
- ✅ Nostr プロトコル標準、シンプルで分かりやすい
- ✅ 必要なメソッド完備
- ✅ このままでOK

**6. 他の Handler/Middleware（handler.go）**
- ✅ SimpleHandler, RouterHandler, CacheHandler: 素直
- ✅ Middleware: NIP-11 対応、標準的（14種類）
- ✅ safeMap: 素直な concurrent-safe map
- ✅ 特に奇をてらった実装なし

---

### 🎯 優先順位

**Phase 1：すぐできる改善**
1. neo_handler.go を削除（76行削減）
2. recvCtx (utils.go) も削除？（使われてなければ）

**Phase 2：大きな改善**
3. event_cache.go を素朴な実装に（450行 → 100行）
   - テストも書き直し
   - 既存の CacheHandler との統合確認

4. MergeHandler をリファクタリング（段階的に）
   - 4a. channel → Mutex へ置き換え（低リスク、可読性向上）
   - 4b. `IsSendableEventMsg` を小さな関数に分割（低リスク、可読性向上）
   - 4c. ドキュメントとコメント追加（メンテナンス性向上）

**Phase 3：慎重に判断**
5. SQLite 実装の見直し（query.go, 328行）
   - 運用で問題が出てから検討
   - goqu → 生 SQL？
   - スキーマ変更？

6. MergeHandler の深い改善（検討）
   - マージソートの明示的実装（正確性↑、複雑性↑）
   - インターフェース設計の見直し（大規模変更）

**その他の検討事項**：
- Makefile の整備
- CI の整備
- testing/synctest の活用可能性

---

### 💡 今日の気づき

- **記憶を掘り起こすのが大事**：当時の考えを思い出すことで、全体像が見えた
- **「奇をてらった実装」の3パターン**：
  - A. 野心的な設計 → 妥協 → 変な形（EventCache, neo_handler）
  - B. 完璧を目指しすぎ → 複雑すぎ（SQLite）
  - C. **状態管理で奇をてらう → つらい実装**（MergeHandler）
    - channel を mutex として使用（Goのアンチパターン）
    - 「状態管理がすごくつらい」という実感は正しかった
- **素直な実装が一番**：でも、理論的に正しい設計も捨てがたい
- **運用重視**：気軽に動かせるのが最優先、完璧主義を捨てる
- **丁寧なレビューの重要性**：前回は「さらっと」終わらせてしまっていた
  - パインちゃんに詳しく読んでもらうことで、深い問題が見つかった
  - 残りトークンを気にしすぎず、丁寧にレビューすることが大事

---

### 🚀 次回以降の作戦

**次回（実装開始）**：
1. neo_handler.go 削除（サクッと）
2. event_cache.go 書き直し（本命）

**長期的**：
- SQLite の見直しは保留（運用で様子見）
- Makefile, CI の整備
- テストの充実
