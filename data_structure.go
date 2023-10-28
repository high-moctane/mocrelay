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

const skipListMaxHeight = 16 + 1

type skipList[K any, V any] struct {
	Cmp  func(K, K) int
	Head *skipListNode[K, V]

	lenMu sync.RWMutex
	len   int

	rndMu sync.Mutex
	rnd   *rand.Rand
}

func newSkipList[K, V any](cmp func(K, K) int) *skipList[K, V] {
	return &skipList[K, V]{
		Cmp:  cmp,
		Head: &skipListNode[K, V]{Height: skipListMaxHeight},
		rnd:  rand.New(rand.NewSource(rand.Int63())),
	}
}

func (l *skipList[K, V]) Len() int {
	l.lenMu.RLock()
	defer l.lenMu.RUnlock()
	return l.len
}

func (l *skipList[K, V]) Find(k K) (v V, ok bool) {
	node := l.Head
	for h := skipListMaxHeight - 1; h >= 0; h-- {
		for {
			node.NextsMu.RLock()
			next := node.Nexts[h]
			node.NextsMu.RUnlock()

			if next == nil || l.Cmp(next.K, k) > 0 {
				break
			}
			node = next
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

type skipListStackEntry[K, V any] struct {
	node *skipListNode[K, V]
	next *skipListNode[K, V]
}

func (l *skipList[K, V]) Add(k K, v V) (added bool) {
	var ok bool
	for {
		if added, ok = l.tryAdd(k, v); ok {
			if added {
				l.lenMu.Lock()
				defer l.lenMu.Unlock()
				l.len++
			}
			return
		}
	}
}

func (l *skipList[K, V]) tryAdd(k K, v V) (added, ok bool) {
	var switched [skipListMaxHeight]skipListStackEntry[K, V]

	var next *skipListNode[K, V]
	node := l.Head
	for h := skipListMaxHeight - 1; h >= 0; h-- {
		for {
			node.NextsMu.RLock()
			next = node.Nexts[h]
			node.NextsMu.RUnlock()

			if next == nil || l.Cmp(next.K, k) >= 0 {
				break
			}
			node = next
		}

		if next != nil && l.Cmp(next.K, k) == 0 {
			return false, true
		}

		switched[h] = skipListStackEntry[K, V]{
			node: node,
			next: next,
		}
	}

	height := l.newHeight()
	newNode := skipListNode[K, V]{
		K:      k,
		V:      v,
		Height: height,
	}

	return true, l.tryAddInsert(&newNode, switched)
}

func (l *skipList[K, V]) tryAddInsert(
	newNode *skipListNode[K, V],
	switched [skipListMaxHeight]skipListStackEntry[K, V],
) (ok bool) {
	var pre *skipListNode[K, V]
	for h := newNode.Height - 1; h >= 0; h-- {
		node := switched[h].node

		if node != pre {
			node.NextsMu.Lock()
			defer node.NextsMu.Unlock()
			pre = node
		}

		if node.Nexts[h] != switched[h].next {
			return
		}
	}

	for h := newNode.Height - 1; h >= 0; h-- {
		newNode.Nexts[h] = switched[h].node.Nexts[h]
		switched[h].node.Nexts[h] = newNode
	}

	return true
}

func (l *skipList[K, V]) newHeight() int {
	l.rndMu.Lock()
	n := l.rnd.Uint32()
	l.rndMu.Unlock()
	return bits.LeadingZeros16(uint16(n)) + 1
}

func (l *skipList[K, V]) Delete(k K) (deleted bool) {
	var ok bool
	for {
		if deleted, ok = l.tryDelete(k); ok {
			if deleted {
				l.lenMu.Lock()
				defer l.lenMu.Unlock()
				l.len--
			}
			return
		}
	}
}

func (l *skipList[K, V]) tryDelete(k K) (deleted, ok bool) {
	var switched [skipListMaxHeight]skipListStackEntry[K, V]
	switchedH := -1
	var willDelete bool

	var next *skipListNode[K, V]
	node := l.Head
	for h := skipListMaxHeight - 1; h >= 0; h-- {
		for {
			node.NextsMu.RLock()
			next = node.Nexts[h]
			node.NextsMu.RUnlock()

			if next == nil || l.Cmp(next.K, k) >= 0 {
				break
			}
			node = next
		}

		if node == nil {
			return false, true
		}

		switched[h] = skipListStackEntry[K, V]{
			node: node,
			next: next,
		}
		if next != nil {
			switchedH = max(switchedH, h)
			willDelete = willDelete || l.Cmp(next.K, k) == 0
		}
	}

	if willDelete {
		return true, l.tryDeleteRemove(switched, switchedH)
	}

	return false, true
}

func (l *skipList[K, V]) tryDeleteRemove(
	switched [skipListMaxHeight]skipListStackEntry[K, V],
	switchedH int,
) (ok bool) {
	var pre *skipListNode[K, V]
	for h := switchedH; h >= 0; h-- {
		node := switched[h].node

		if node != pre {
			node.NextsMu.Lock()
			defer node.NextsMu.Unlock()
			pre = node
		}

		if node.Nexts[h] != switched[h].next {
			return
		}
	}

	for h := switchedH; h >= 0; h-- {
		if switched[h].next == nil {
			continue
		}

		switched[h].node.Nexts[h] = switched[h].node.Nexts[h].Nexts[h]
	}

	return true
}

type skipListNode[K, V any] struct {
	K K
	V V

	Height int

	NextsMu sync.RWMutex
	Nexts   [skipListMaxHeight]*skipListNode[K, V]
}

func (nd *skipListNode[K, V]) Next() *skipListNode[K, V] {
	if nd == nil {
		return nil
	}

	nd.NextsMu.RLock()
	defer nd.NextsMu.RUnlock()
	return nd.Nexts[0]
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
