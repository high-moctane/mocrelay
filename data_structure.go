package mocrelay

import (
	"container/heap"
	"sync"
)

type safeMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func newSafeMap[K comparable, V any]() *safeMap[K, V] {
	return &safeMap[K, V]{m: make(map[K]V)}
}

func (m *safeMap[K, V]) Get(k K) V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.m[k]
}

func (m *safeMap[K, V]) TryGet(k K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.m[k]
	return v, ok
}

func (m *safeMap[K, V]) Add(k K, v V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[k] = v
}

func (m *safeMap[K, V]) Delete(k K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, k)
}

func (m *safeMap[K, V]) Loop(f func(k K, v V)) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for key, val := range m.m {
		f(key, val)
	}
}

type typedHeap[T any] struct {
	S        []T
	LessFunc func(T, T) bool
}

func newTypedHeap[T any](less func(T, T) bool) *typedHeap[T] {
	return &typedHeap[T]{LessFunc: less}
}

func (h *typedHeap[T]) Len() int {
	return len(h.S)
}

func (h *typedHeap[T]) Less(i, j int) bool {
	return h.LessFunc(h.S[i], h.S[j])
}

func (h *typedHeap[T]) Swap(i, j int) {
	h.S[i], h.S[j] = h.S[j], h.S[i]
}

func (h *typedHeap[T]) Push(x interface{}) {
	h.S = append(h.S, x.(T))
}

func (h *typedHeap[T]) Pop() interface{} {
	old := h.S
	n := len(old)
	x := old[n-1]
	h.S = old[0 : n-1]
	return x
}

func (h *typedHeap[T]) HeapInit() {
	heap.Init(h)
}

func (h *typedHeap[T]) HeapPush(x T) {
	heap.Push(h, x)
}

func (h *typedHeap[T]) HeapPop() T {
	return heap.Pop(h).(T)
}

func (h *typedHeap[T]) HeapPushPop(x T) T {
	old := h.S[0]
	h.S[0] = x
	heap.Fix(h, 0)
	return old
}

func (h *typedHeap[T]) HeapPeek() T {
	return h.S[0]
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
