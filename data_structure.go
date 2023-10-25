package mocrelay

import (
	"math/bits"
	"math/rand"
	"sync"
)

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

const skipListMaxHeight = 16

type skipList[K any, V any] struct {
	Len  int
	Cmp  func(K, K) int
	Head *skipListNode[K, V]
	rnd  *rand.Rand
}

func newSkipList[K any, V any](cmp func(K, K) int) *skipList[K, V] {
	return &skipList[K, V]{
		Cmp:  cmp,
		Head: &skipListNode[K, V]{Nexts: make([]*skipListNode[K, V], skipListMaxHeight)},
		rnd:  rand.New(rand.NewSource(rand.Int63())),
	}
}

func (l *skipList[K, V]) Find(k K) (v V, ok bool) {
	node := l.Head
	for h := skipListMaxHeight - 1; h >= 0; h-- {
		for node.Nexts[h] != nil && l.Cmp(node.Nexts[h].K, k) <= 0 {
			node = node.Nexts[h]
		}
		if l.Cmp(node.K, k) == 0 {
			if node == l.Head {
				return
			}
			return node.V, true
		}
	}

	return
}

func (l *skipList[K, V]) Add(k K, v V) (added bool) {
	switched := make([]*skipListNode[K, V], skipListMaxHeight)

	node := l.Head
	for h := skipListMaxHeight - 1; h >= 0; h-- {
		for node.Nexts[h] != nil && l.Cmp(node.Nexts[h].K, k) < 0 {
			node = node.Nexts[h]
		}
		if node.Nexts[h] != nil && l.Cmp(node.Nexts[h].K, k) == 0 {
			return
		}
		switched[h] = node
	}

	newNode := skipListNode[K, V]{
		K:     k,
		V:     v,
		Nexts: make([]*skipListNode[K, V], l.newHeight()),
	}

	for h := 0; h < len(newNode.Nexts); h++ {
		newNode.Nexts[h] = switched[h].Nexts[h]
		switched[h].Nexts[h] = &newNode
	}

	l.Len++
	return true
}

func (l *skipList[K, V]) newHeight() int {
	n := l.rnd.Uint32()
	return bits.LeadingZeros16(uint16(n|1)) + 1
}

func (l *skipList[K, V]) Delete(k K) (removed bool) {
	node := l.Head
	for h := skipListMaxHeight - 1; h >= 0; h-- {
		for node.Nexts[h] != nil && l.Cmp(node.Nexts[h].K, k) < 0 {
			node = node.Nexts[h]
		}
		if node.Nexts[h] != nil && l.Cmp(node.Nexts[h].K, k) == 0 {
			removed = true
			node.Nexts[h] = node.Nexts[h].Nexts[h]
		}
	}

	if removed {
		l.Len--
	}
	return
}

type skipListNode[K any, V any] struct {
	Mu    sync.RWMutex
	K     K
	V     V
	Nexts []*skipListNode[K, V]
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
