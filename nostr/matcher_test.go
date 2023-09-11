package nostr

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/high-moctane/mocrelay/utils"
)

func TestMatcher(t *testing.T) {
	filters := []*Filter{{Limit: utils.ToRef(int64(2))}, {Limit: utils.ToRef(int64(3))}}
	event := &Event{
		ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
		Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
		CreatedAt: 1693156107,
		Kind:      1,
		Tags:      []Tag{},
		Content:   "ぽわ〜",
		Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
	}

	m := NewFiltersMatcher(filters)

	match := m.CountMatch(event)
	assert.True(t, match)
	assert.False(t, m.Done())

	match = m.CountMatch(event)
	assert.True(t, match)
	assert.False(t, m.Done())

	match = m.CountMatch(event)
	assert.True(t, match)
	assert.True(t, m.Done())

	match = m.CountMatch(event)
	assert.True(t, match)
	assert.True(t, m.Done())
}
