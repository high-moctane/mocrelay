package mocrelay

import "fmt"

type ringBuffer[T any] struct {
	Cap int

	s          []T
	head, tail int
}

func newRingBuffer[T any](capacity int) *ringBuffer[T] {
	if capacity <= 0 {
		panic(fmt.Sprintf("capacity must be positive but got %d", capacity))
	}
	return &ringBuffer[T]{
		Cap:  capacity,
		s:    make([]T, capacity),
		head: 0,
		tail: 0,
	}
}

func (rb *ringBuffer[T]) mod(a int) int {
	return a % rb.Cap
}

func (rb *ringBuffer[T]) idx(i int) int {
	return rb.mod(rb.tail - 1 - i)
}

func (rb *ringBuffer[T]) At(i int) T {
	return rb.s[rb.idx(i)]
}

func (rb *ringBuffer[T]) Len() int {
	return rb.tail - rb.head
}

func (rb *ringBuffer[T]) Enqueue(v T) {
	rb.s[rb.mod(rb.tail)] = v
	rb.tail++
}

func (rb *ringBuffer[T]) Dequeue() T {
	var empty T
	modhead := rb.mod(rb.head)
	old := rb.s[modhead]
	rb.s[modhead] = empty
	rb.head++
	return old
}

func (rb *ringBuffer[T]) Swap(i, j int) {
	ii := rb.idx(i)
	jj := rb.idx(j)
	rb.s[ii], rb.s[jj] = rb.s[jj], rb.s[ii]
}

type priorityQueue[T any] struct {
	s    []T
	less func(T, T) bool
}
