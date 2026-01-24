# NIP-13: Proof of Work

**ステータス**: `draft` `optional` `relay`

## 概要

イベント ID の先頭ゼロビット数による PoW。スパム対策として利用可能。

## リレーの責務

### PoW 検証

- **difficulty の計算**: イベント ID の先頭から連続する 0 ビットの数
- **nonce タグの確認**: `["nonce", "<数値>", "<target-difficulty>"]`
- **target difficulty**: コミット済み難易度（第3要素）の確認 SHOULD

### 拒否条件

- `min_pow_difficulty` 未満のイベントを拒否可能
- OK メッセージ: `["OK", <event-id>, false, "pow: insufficient proof of work"]`

## 実装ポイント

### difficulty の計算

**16進数から0ビットを数える**:

```javascript
function countLeadingZeroes(hex) {
  let count = 0;
  for (let i = 0; i < hex.length; i++) {
    const nibble = parseInt(hex[i], 16);
    if (nibble === 0) {
      count += 4;
    } else {
      count += Math.clz32(nibble) - 28;
      break;
    }
  }
  return count;
}
```

### nonce タグの構造

```json
{
  "tags": [
    ["nonce", "776797", "20"]
  ]
}
```

- 第1要素: `"nonce"` (固定)
- 第2要素: マイニングで変更する値
- 第3要素: target difficulty（コミット値）

### target difficulty の重要性

- **なぜ必要**: 低難易度を狙ったスパマーが運良く高難易度を達成してしまうケースを防ぐ
- **検証**: `difficulty >= target` かつ `target >= min_pow_difficulty` を確認

### エッジケース

- **created_at の更新**: マイニング中に `created_at` も更新すべき（推奨）
- **複数の nonce タグ**: 最初のものを使用
- **target なし**: 拒否してもよい MAY

## NIP-11 との連携

```json
{
  "limitation": {
    "min_pow_difficulty": 30
  }
}
```

リレーが要求する最小難易度を advertise する。

## 他の NIP との関連

- **NIP-01**: イベント ID の計算
- **NIP-11**: `min_pow_difficulty` フィールド

## 参考

- [NIP-13 原文](https://github.com/nostr-protocol/nips/blob/master/13.md)
