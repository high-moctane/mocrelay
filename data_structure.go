package mocrelay

import "fmt"

type RingBuffer[T any] struct {
	Cap int

	s          []T
	head, tail int
}

func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity <= 0 {
		panic(fmt.Sprintf("capacity must be positive but got %d", capacity))
	}
	return &RingBuffer[T]{
		Cap:  capacity,
		s:    make([]T, capacity),
		head: 0,
		tail: 0,
	}
}

func (rb *RingBuffer[T]) mod(a int) int {
	return a % rb.Cap
}

func (rb *RingBuffer[T]) Len() int {
	return rb.tail - rb.head
}

func (rb *RingBuffer[T]) Enqueue(v T) {
	rb.s[rb.tail] = v
	rb.tail++
}

func (rb *RingBuffer[T]) Dequeue(v T) T {
	var empty T
	old := rb.s[rb.head]
	rb.s[rb.head] = empty
	rb.head++
	return old
}

func (rb *RingBuffer[T]) Do(f func(v T) (done bool)) {
	for i := rb.head; i < rb.tail; i++ {
		idx := rb.mod(i)

		if f(rb.s[idx]) {
			break
		}
	}
}
