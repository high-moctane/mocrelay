package mocrelay

type ringBuffer[T any] struct {
	Cap int

	s          []T
	head, tail int
}

func newRingBuffer[T any](capacity int) *ringBuffer[T] {
	if capacity <= 0 {
		panicf("capacity must be positive but got %d", capacity)
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
	if i < 0 || rb.Len() <= i {
		panicf("index out of range [%d]", i)
	}
	return rb.mod(rb.tail - 1 - i)
}

func (rb *ringBuffer[T]) At(i int) T {
	return rb.s[rb.idx(i)]
}

func (rb *ringBuffer[T]) Len() int {
	return rb.tail - rb.head
}

func (rb *ringBuffer[T]) Enqueue(v T) {
	if rb.Len() == rb.Cap {
		panic("enqueue into full ring buffer")
	}

	rb.s[rb.mod(rb.tail)] = v
	rb.tail++
}

func (rb *ringBuffer[T]) Dequeue() T {
	if rb.Len() == 0 {
		panic("dequeue from empty ring buffer")
	}

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

func (rb *ringBuffer[T]) IdxFunc(f func(v T) bool) int {
	for i := 0; i < rb.Len(); i++ {
		if f(rb.At(i)) {
			return i
		}
	}
	return -1
}

type typedHeap[T any] struct {
	S        []T
	LessFunc func(T, T) bool
}

func (h typedHeap[T]) Len() int { return len(h.S) }

func (h typedHeap[T]) Less(i, j int) bool { return h.LessFunc(h.S[i], h.S[j]) }

func (h typedHeap[T]) Swap(i, j int) { h.S[i], h.S[j] = h.S[j], h.S[i] }

func (h *typedHeap[T]) Push(x any) { h.PushT(x.(T)) }

func (h *typedHeap[T]) PushT(x T) { h.S = append(h.S, x) }

func (h *typedHeap[T]) Pop() any { return h.Pop() }

func (h *typedHeap[T]) PopT() T {
	ret := h.S[len(h.S)-1]
	h.S = h.S[:len(h.S)-1]
	return ret
}

type randCache[K comparable, V any] struct {
	Cap int
	c   map[K]V
}

func newRandCache[K comparable, V any](capacity int) *randCache[K, V] {
	return &randCache[K, V]{
		Cap: capacity,
		c:   make(map[K]V, capacity),
	}
}

func (c *randCache[K, V]) Get(key K) (v V, found bool) {
	v, found = c.c[key]
	return
}

func (c *randCache[K, V]) Set(key K, value V) (added bool) {
	if _, ok := c.c[key]; ok {
		return false
	}

	if len(c.c) >= c.Cap {
		var k K
		for k = range c.c {
			break
		}
		delete(c.c, k)
	}

	c.c[key] = value

	return true
}
