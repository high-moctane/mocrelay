package mocrelay

import (
	"cmp"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingBuffer(t *testing.T) {
	b := newRingBuffer[int](3)

	collect := func() []int {
		var ret []int
		for i := 0; i < b.Len(); i++ {
			ret = append(ret, b.At(i))
		}
		return ret
	}

	for i := 0; i < 3; i++ {
		assert.EqualValues(t, []int(nil), collect())
		b.Enqueue(1)
		assert.EqualValues(t, []int{1}, collect())
		b.Enqueue(2)
		b.Enqueue(3)
		assert.EqualValues(t, []int{3, 2, 1}, collect())
		assert.Equal(t, 1, b.Dequeue())
		assert.EqualValues(t, []int{3, 2}, collect())
		assert.Equal(t, 2, b.Dequeue())
		assert.Equal(t, 3, b.Dequeue())
		assert.Panics(t, func() { b.Dequeue() })
		b.Enqueue(4)
		b.Enqueue(5)
		b.Enqueue(6)
		assert.Panics(t, func() { b.Enqueue(7) })
		assert.EqualValues(t, []int{6, 5, 4}, collect())
		assert.Equal(t, 6, b.At(0))
		assert.Equal(t, 5, b.At(1))
		assert.Equal(t, 4, b.At(2))
		assert.Panics(t, func() { b.At(3) })
		b.Swap(0, 1)
		assert.EqualValues(t, []int{5, 6, 4}, collect())
		assert.Equal(t, 4, b.Dequeue())
		assert.Equal(t, 6, b.Dequeue())
		assert.Equal(t, 5, b.Dequeue())
		assert.Panics(t, func() { b.Dequeue() })
	}
}

func TestRingBuffer_IdxFunc(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		f    func(v int) bool
		want int
	}{
		{
			"ok",
			[]int{1, 2, 3},
			func(v int) bool { return v < 2 },
			2,
		},
		{
			"ok: empty",
			nil,
			func(v int) bool { return v < 2 },
			-1,
		},
		{
			"not found",
			[]int{1, 2, 3},
			func(v int) bool { return v < 0 },
			-1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newRingBuffer[int](3)
			for _, v := range tt.in {
				b.Enqueue(v)
			}
			got := b.IdxFunc(tt.f)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSkipList_Find(t *testing.T) {
	type entry struct{ k, v int }

	tests := []struct {
		name   string
		cmp    func(int, int) int
		input  []entry
		target int
		found  bool
	}{
		{
			name:   "empty",
			cmp:    cmp.Compare[int],
			input:  nil,
			target: 2,
			found:  false,
		},
		{
			name:   "one: found",
			cmp:    cmp.Compare[int],
			input:  []entry{{1, 1}},
			target: 1,
			found:  true,
		},
		{
			name:   "one: not found: too large",
			cmp:    cmp.Compare[int],
			input:  []entry{{1, 1}},
			target: 3,
			found:  false,
		},
		{
			name:   "one: not found: too small",
			cmp:    cmp.Compare[int],
			input:  []entry{{1, 1}},
			target: -5,
			found:  false,
		},
		{
			name: "odd: found",
			cmp:  cmp.Compare[int],
			input: func() []entry {
				var ret []entry
				for i := 1; i < 100; i += 2 {
					ret = append(ret, entry{i, i})
				}
				return ret
			}(),
			target: 31,
			found:  true,
		},
		{
			name: "odd: not found",
			cmp:  cmp.Compare[int],
			input: func() []entry {
				var ret []entry
				for i := 1; i < 100; i += 2 {
					ret = append(ret, entry{i, i})
				}
				return ret
			}(),
			target: 48,
			found:  false,
		},
		{
			name: "rand odd: found",
			cmp:  cmp.Compare[int],
			input: func() []entry {
				ret := make([]entry, 100)
				for i := 1; i < 100; i += 2 {
					ret[i] = entry{i, i}
				}
				rand.Shuffle(len(ret), func(i, j int) { ret[i], ret[j] = ret[j], ret[i] })
				return ret
			}(),
			target: 73,
			found:  true,
		},
		{
			name: "rand odd: not found",
			cmp:  cmp.Compare[int],
			input: func() []entry {
				ret := make([]entry, 100)
				for i := 1; i < 100; i += 2 {
					ret[i] = entry{i, i}
				}
				rand.Shuffle(len(ret), func(i, j int) { ret[i], ret[j] = ret[j], ret[i] })
				return ret
			}(),
			target: 64,
			found:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newSkipList[int, int](10, tt.cmp)

			for _, item := range tt.input {
				l.Add(item.k, item.v)
			}

			got, ok := l.Find(tt.target)
			assert.Equal(t, tt.found, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.target, got)
		})
	}
}

func TestSkipList_Add(t *testing.T) {
	type entry struct{ k, v int }

	tests := []struct {
		name  string
		cmp   func(int, int) int
		input []entry
		want  []int
	}{
		{
			name:  "empty",
			cmp:   cmp.Compare[int],
			input: nil,
			want:  nil,
		},
		{
			name:  "one",
			cmp:   cmp.Compare[int],
			input: []entry{{1, 1}},
			want:  []int{1},
		},
		{
			name: "succ",
			cmp:  cmp.Compare[int],
			input: func() []entry {
				ret := make([]entry, 100)
				for i := 0; i < 100; i++ {
					ret[i] = entry{i, i}
				}
				return ret
			}(),
			want: func() []int {
				ret := make([]int, 100)
				for i := 0; i < 100; i++ {
					ret[i] = i
				}
				return ret
			}(),
		},
		{
			name: "reverse",
			cmp:  cmp.Compare[int],
			input: func() []entry {
				ret := make([]entry, 100)
				for i := 0; i < 100; i++ {
					ret[99-i] = entry{i, i}
				}
				return ret
			}(),
			want: func() []int {
				ret := make([]int, 100)
				for i := 0; i < 100; i++ {
					ret[i] = i
				}
				return ret
			}(),
		},
		{
			name: "rand",
			cmp:  cmp.Compare[int],
			input: func() []entry {
				ret := make([]entry, 100)
				for i := 0; i < 100; i++ {
					ret[i] = entry{i, i}
				}
				rand.Shuffle(100, func(i, j int) { ret[i], ret[j] = ret[j], ret[i] })
				return ret
			}(),
			want: func() []int {
				ret := make([]int, 100)
				for i := 0; i < 100; i++ {
					ret[i] = i
				}
				return ret
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newSkipList[int, int](10, tt.cmp)

			for _, item := range tt.input {
				l.Add(item.k, item.v)
			}

			var got []int
			for node := l.Head.Nexts[0]; node != nil; node = node.Nexts[0] {
				got = append(got, node.V)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSkipList_Delete(t *testing.T) {
	type entry struct{ k, v int }

	tests := []struct {
		name    string
		cmp     func(int, int) int
		input   []entry
		target  int
		deleted bool
		want    []int
	}{
		{
			name:    "empty",
			cmp:     cmp.Compare[int],
			input:   nil,
			target:  0,
			deleted: false,
			want:    nil,
		},
		{
			name:    "one",
			cmp:     cmp.Compare[int],
			input:   []entry{{1, 1}},
			target:  1,
			deleted: true,
			want:    nil,
		},
		{
			name:    "first",
			cmp:     cmp.Compare[int],
			input:   []entry{{1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}},
			target:  1,
			deleted: true,
			want:    []int{2, 3, 4, 5},
		},
		{
			name:    "last",
			cmp:     cmp.Compare[int],
			input:   []entry{{1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}},
			target:  5,
			deleted: true,
			want:    []int{1, 2, 3, 4},
		},
		{
			name:    "3",
			cmp:     cmp.Compare[int],
			input:   []entry{{1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}},
			target:  3,
			deleted: true,
			want:    []int{1, 2, 4, 5},
		},
		{
			name:    "not found",
			cmp:     cmp.Compare[int],
			input:   []entry{{1, 1}, {3, 3}, {4, 4}, {5, 5}},
			target:  2,
			deleted: false,
			want:    []int{1, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newSkipList[int, int](10, tt.cmp)

			for _, item := range tt.input {
				l.Add(item.k, item.v)
			}

			deleted := l.Delete(tt.target)
			assert.Equal(t, tt.deleted, deleted)

			var got []int
			for node := l.Head.Nexts[0]; node != nil; node = node.Nexts[0] {
				got = append(got, node.V)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
