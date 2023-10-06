package mocrelay

import (
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