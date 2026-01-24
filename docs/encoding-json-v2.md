# encoding/json/v2 ガイド（Nostr リレー向け）

**最終更新**: 2026-01-24

Go 1.25 で experimental として導入された `encoding/json/v2` を Nostr リレー実装で活用するためのガイド。

## 概要

`encoding/json/v2` は Nostr の厳密な JSON 要件にほぼ完璧に対応できる：

| Nostr の要件 | json/v2 での実現方法 |
|-------------|---------------------|
| 余分なフィールドを拒否 | `RejectUnknownMembers(true)` |
| フィールド不足を検出 | `UnmarshalerFrom` でカウント |
| unix timestamp ↔ time.Time | `format:unix` タグ |
| 数値の精度保持（int64） | デフォルトで OK |
| ケースセンシティブマッチング | デフォルトで OK |
| 重複メンバー名を拒否 | 自動検出 |
| 無効な UTF-8 を拒否 | RFC 8259 準拠 |

## 有効化方法

```bash
# 環境変数で有効化
GOEXPERIMENT=jsonv2 go build ./...
GOEXPERIMENT=jsonv2 go test ./...
```

または、ビルドタグを使用：

```go
//go:build goexperiment.jsonv2
```

## 基本的な使い方

### Import

```go
import (
    "encoding/json/v2"
    "encoding/json/jsontext"
)
```

### Marshal / Unmarshal

```go
// Unmarshal（余分なフィールドを拒否）
err := json.Unmarshal(data, &value, json.RejectUnknownMembers(true))

// Marshal
out, err := json.Marshal(&value)
```

## Nostr Event での使用例

### 基本的な Event 構造体

```go
type Event struct {
    ID        string     `json:"id"`
    Pubkey    string     `json:"pubkey"`
    CreatedAt time.Time  `json:"created_at,format:unix"`
    Kind      int64      `json:"kind"`
    Tags      [][]string `json:"tags"`
    Content   string     `json:"content"`
    Sig       string     `json:"sig"`
}
```

**ポイント**:
- `format:unix` で unix timestamp を `time.Time` として扱える
- Marshal 時も自動的に数値に戻る

### フィールド数チェック付き Event

Nostr の Event は必ず 7 フィールドでなければならない。
`UnmarshalerFrom` を実装してフィールド数をカウントする：

```go
func (e *Event) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
    // jsontext.Value として読み取り
    val, err := dec.ReadValue()
    if err != nil {
        return err
    }

    // フィールド数をカウント
    count := 0
    tempDec := jsontext.NewDecoder(bytes.NewReader(val))
    tok, _ := tempDec.ReadToken() // {
    if tok.Kind() != '{' {
        return fmt.Errorf("expected object, got %v", tok.Kind())
    }
    for {
        tok, err := tempDec.ReadToken()
        if err != nil {
            return err
        }
        if tok.Kind() == '}' {
            break
        }
        count++
        tempDec.SkipValue() // 値をスキップ
    }

    // 7 フィールドでなければエラー
    if count != 7 {
        return fmt.Errorf("event must have exactly 7 fields, got %d", count)
    }

    // 通常の unmarshal を実行
    type EventAlias Event // 無限ループ防止
    alias := (*EventAlias)(e)
    return json.Unmarshal(val, alias, json.RejectUnknownMembers(true))
}
```

## 主要な Options

### RejectUnknownMembers

未知のフィールドを拒否する：

```go
err := json.Unmarshal(data, &value, json.RejectUnknownMembers(true))
// エラー例: json: cannot unmarshal JSON string into Go main.Event: unknown object member name "extra"
```

### StringifyNumbers

数値を文字列として扱う（精度保持が必要な場合）：

```go
// Marshal 時: 数値を文字列化
b, err := json.Marshal(&value, json.StringifyNumbers(true))

// Unmarshal 時: 文字列内の数値をパース
err = json.Unmarshal(b, &value, json.StringifyNumbers(true))
```

## 主要な Struct タグ

| タグ | 説明 | 例 |
|------|------|-----|
| `format:unix` | unix timestamp として扱う | `json:"created_at,format:unix"` |
| `format:unixmilli` | ミリ秒 unix timestamp | `json:"ts,format:unixmilli"` |
| `omitempty` | 空値なら省略 | `json:"name,omitempty"` |
| `omitzero` | ゼロ値なら省略 | `json:"count,omitzero"` |
| `string` | 数値を文字列として扱う | `json:"id,string"` |
| `inline` | 埋め込みフィールドを展開 | `json:",inline"` |
| `case:ignore` | 大文字小文字を無視 | `json:"name,case:ignore"` |

## カスタム Marshal/Unmarshal

### UnmarshalerFrom（推奨）

ストリーミング処理で高パフォーマンス：

```go
type UnmarshalerFrom interface {
    UnmarshalJSONFrom(*jsontext.Decoder) error
}
```

### MarshalerTo（推奨）

```go
type MarshalerTo interface {
    MarshalJSONTo(*jsontext.Encoder) error
}
```

### 従来の Marshaler/Unmarshaler

互換性のため引き続きサポート：

```go
type Marshaler interface {
    MarshalJSON() ([]byte, error)
}

type Unmarshaler interface {
    UnmarshalJSON([]byte) error
}
```

## v1 との違い

| 項目 | v1 | v2 |
|------|-----|-----|
| 未知フィールド | 無視 | `RejectUnknownMembers` で拒否可能 |
| 重複メンバー名 | 最後の値を使用 | エラー |
| ケースマッチング | 大文字小文字を無視 | 厳密（デフォルト） |
| 無効な UTF-8 | 許容 | エラー |
| nil スライス | `null` | `[]` |
| nil マップ | `null` | `{}` |

## 注意事項

### Experimental

```
This package (encoding/json/v2) is experimental, and not subject to the Go 1
compatibility promise.
```

- API は将来変更される可能性がある
- `GOEXPERIMENT=jsonv2` が必要
- ただし、複数の大規模プロジェクトで本番使用実績あり

### Serialize（署名検証用）

Event の署名検証には、特定の順序でフィールドをシリアライズする必要がある：

```go
[0, pubkey, created_at, kind, tags, content]
```

これは Marshal では実現できないため、手書きが必要。
ただし、`jsontext.Encoder` を使えば比較的簡単に実装可能。

## 参考資料

- [encoding/json/v2 - Go Packages](https://pkg.go.dev/encoding/json/v2)
- [A new experimental Go API for JSON - Go Blog](https://go.dev/blog/jsonv2-exp)
- [encoding/json/v2 Discussion #63397](https://github.com/golang/go/discussions/63397)

## go doc コマンド

```bash
# パッケージ概要
GOEXPERIMENT=jsonv2 go doc encoding/json/v2

# 特定の関数/型
GOEXPERIMENT=jsonv2 go doc encoding/json/v2.RejectUnknownMembers
GOEXPERIMENT=jsonv2 go doc encoding/json/v2.UnmarshalerFrom
GOEXPERIMENT=jsonv2 go doc encoding/json/jsontext.Decoder

# 全ての API
GOEXPERIMENT=jsonv2 go doc -all encoding/json/v2
```
