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

func (rb *ringBuffer[T]) Len() int {
	return rb.tail - rb.head
}

func (rb *ringBuffer[T]) Enqueue(v T) {
	rb.s[rb.tail] = v
	rb.tail++
}

func (rb *ringBuffer[T]) Dequeue() T {
	var empty T
	old := rb.s[rb.head]
	rb.s[rb.head] = empty
	rb.head++
	return old
}

func (rb *ringBuffer[T]) Do(f func(v T) (done bool)) {
	for i := rb.head; i < rb.tail; i++ {
		idx := rb.mod(i)

		if f(rb.s[idx]) {
			break
		}
	}
}

type priorityQueue[T any] struct {
	s    []T
	less func(T, T) bool
}

func newPriorityQueue[T any](less func(T, T) bool) *priorityQueue[T] {
	return &priorityQueue[T]{
		s:    nil,
		less: less,
	}
}

func (q *priorityQueue[T]) Len() int { return len(q.s) }

func (q *priorityQueue[T]) Less(i, j int) bool { return q.less(q.s[i], q.s[j]) }

func (q *priorityQueue[T]) Swap(i, j int) { q.s[i], q.s[j] = q.s[j], q.s[i] }

func (q *priorityQueue[T]) Push(v any) {
	q.TypedPush(v.(T))
}

func (q *priorityQueue[T]) TypedPush(v T) {
	q.s = append(q.s, v)
}

func (q *priorityQueue[T]) Pop() any {
	return q.TypedPop()
}

func (q *priorityQueue[T]) TypedPop() T {
	var empty T
	l := len(q.s)
	v := q.s[l-1]
	q.s[l-1] = empty
	q.s = q.s[:l-1]
	return v
}
