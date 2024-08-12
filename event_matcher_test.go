package mocrelay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatcher(t *testing.T) {
	filters := []*ReqFilter{{Limit: toPtr(int64(2))}, {Limit: toPtr(int64(3))}}
	event := &Event{
		ID:        "d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
		Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
		CreatedAt: 1693156107,
		Kind:      1,
		Tags:      []Tag{},
		Content:   "ぽわ〜",
		Sig:       "47f04052e5b6b3d9a0ca6493494af10618af35e00aeb30cdc86c2a33aca01738a3267f6ff5e06c0270eb0f4e25ba051782e8d7bba61706b857a66c4c17c88eee",
	}

	m := NewReqFiltersEventLimitMatcher(filters)

	match := m.LimitMatch(event)
	assert.True(t, match)
	assert.False(t, m.Done())

	match = m.LimitMatch(event)
	assert.True(t, match)
	assert.False(t, m.Done())

	match = m.LimitMatch(event)
	assert.True(t, match)
	assert.True(t, m.Done())

	match = m.LimitMatch(event)
	assert.True(t, match)
	assert.True(t, m.Done())
}

func TestReqFilterMatcher_Match(t *testing.T) {
	tests := []struct {
		name   string
		input  Event
		filter ReqFilter
		want   bool
	}{
		{
			name: "empty filter",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{},
			want:   true,
		},
		{
			name: "empty ids",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				IDs: []string{},
			},
			want: false,
		},
		{
			name: "ids match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				IDs: []string{
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
				},
			},
			want: true,
		},
		{
			name: "ids not match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				IDs: []string{"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c"},
			},
			want: false,
		},
		{
			name: "empty authors",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Authors: []string{},
			},
			want: false,
		},
		{
			name: "authors match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Authors: []string{
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					"fbfcea07d1a1764d9b20167ee5b96d3881a9e908a75dd93de3f16150fb69084c",
				},
			},
			want: true,
		},
		{
			name: "authors not match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Authors: []string{
					"fbfcea07d1a1764d9b20167ee5b96d3881a9e908a75dd93de3f16150fb69084c",
				},
			},
			want: false,
		},
		{
			name: "empty kinds",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Kinds: []int64{},
			},
			want: false,
		},
		{
			name: "kind match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Kinds: []int64{0, 1, 2, 3},
			},
			want: true,
		},
		{
			name: "kind not match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Kinds: []int64{0, 2, 3},
			},
			want: false,
		},
		{
			name: "empty #e tags",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Tags: map[string][]string{
					"e": {},
				},
			},
			want: false,
		},
		{
			name: "match #e tag",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Tags: map[string][]string{
					"e": {
						"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					},
				},
			},
			want: true,
		},
		{
			name: "not match #e tag",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Tags: map[string][]string{
					"e": {"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2"},
				},
			},
			want: false,
		},
		{
			name: "match #e tag, empty #p tag",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Tags: map[string][]string{
					"e": {"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2"},
					"p": {},
				},
			},
			want: false,
		},
		{
			name: "match #e tag, match #p tag",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Tags: map[string][]string{
					"e": {
						"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					},
					"p": {
						"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
						"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
					},
				},
			},
			want: true,
		},
		{
			name: "match #e tag, not match #p tag",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Tags: map[string][]string{
					"e": {"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2"},
					"p": {"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2"},
				},
			},
			want: false,
		},
		{
			name: "since match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Since: toPtr(int64(1693157791)),
			},
			want: true,
		},
		{
			name: "since not match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Since: toPtr(int64(1693157792)),
			},
			want: false,
		},
		{
			name: "until match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Until: toPtr(int64(1693157791)),
			},
			want: true,
		},
		{
			name: "until not match",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				Until: toPtr(int64(1693157790)),
			},
			want: false,
		},
		{
			name: "all",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				IDs: []string{
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				},
				Authors: []string{
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				Kinds: []int64{1},
				Tags: map[string][]string{
					"e": {"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c"},
					"p": {"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
				},
				Since: toPtr(int64(1693157791)),
				Until: toPtr(int64(1693157791)),
			},
			want: true,
		},
		{
			name: "all: not match #e tag",
			input: Event{
				ID:        "49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				Pubkey:    "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				CreatedAt: 1693157791,
				Kind:      1,
				Tags: []Tag{{
					"e",
					"d2ea747b6e3a35d2a8b759857b73fcaba5e9f3cfb6f38d317e034bddc0bf0d1c",
					"",
					"root",
				}, {
					"p",
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				},
				Content: "powa",
				Sig:     "795e51656e8b863805c41b3a6e1195ed63bf8c5df1fc3a4078cd45aaf0d8838f2dc57b802819443364e8e38c0f35c97e409181680bfff83e58949500f5a8f0c8",
			},
			filter: ReqFilter{
				IDs: []string{
					"49d58222bd85ddabfc19b8052d35bcce2bad8f1f3030c0bc7dc9f10dba82a8a2",
				},
				Authors: []string{
					"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
				},
				Kinds: []int64{1},
				Tags: map[string][]string{
					"e": {"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
					"p": {"dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e"},
				},
				Since: toPtr(int64(1693157791)),
				Until: toPtr(int64(1693157791)),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewReqFilterMatcher(&tt.filter)
			got := m.Match(&tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
