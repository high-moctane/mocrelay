package mocrelay

import (
	"math"
	"math/bits"
	"math/rand"
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

const skipListMaxHeight = 10

type skipList[K, V any] struct {
	MaxHeight int
	CmpFunc   func(K, K) int

	Head *skipListNode[K, V]

	lenMu sync.RWMutex
	len   int

	rndMu sync.Mutex
	rnd   *rand.Rand
}

func newSkipList[K, V any](cmpFunc func(K, K) int) *skipList[K, V] {
	var k K
	var v V

	return &skipList[K, V]{
		MaxHeight: skipListMaxHeight,
		CmpFunc:   cmpFunc,
		Head:      newSkipListNode(k, v, skipListMaxHeight),
		rnd:       rand.New(rand.NewSource(rand.Int63())),
	}
}

func (l *skipList[K, V]) Len() int {
	l.lenMu.RLock()
	defer l.lenMu.RUnlock()
	return l.len
}

func (l *skipList[K, V]) lenInc() {
	l.lenMu.Lock()
	defer l.lenMu.Unlock()
	l.len++
}

func (l *skipList[K, V]) lenDec() {
	l.lenMu.Lock()
	defer l.lenMu.Unlock()
	l.len--
}

func (l *skipList[K, V]) Find(k K) (v V, ok bool) {
	node := l.Head

	for h := l.MaxHeight - 1; h >= 0; h-- {
		for {
			node.nexts[h].mu.RLock()
			next := node.nexts[h].node
			node.nexts[h].mu.RUnlock()

			if next == nil || l.CmpFunc(next.K, k) > 0 {
				break
			}
			node = next
		}

		if node != l.Head && l.CmpFunc(node.K, k) == 0 {
			return node.V, true
		}
	}

	return
}

func (l *skipList[K, V]) FindPre(k K) (v V, ok bool) {
	node := l.Head

	for h := l.MaxHeight - 1; h >= 0; h-- {
		for {
			node.nexts[h].mu.RLock()
			next := node.nexts[h].node
			node.nexts[h].mu.RUnlock()

			if next == nil {
				break
			}
			if c := l.CmpFunc(next.K, k); c >= 0 {
				if h == 0 && node != l.Head && c == 0 {
					return node.V, true
				}
				break
			}

			node = next
		}
	}

	return
}

func (l *skipList[K, V]) findFirstNodeFunc(f func(K) int) *skipListNode[K, V] {
	node := l.Head

	for h := l.MaxHeight - 1; h >= 0; h-- {
		for {
			node.nexts[h].mu.RLock()
			next := node.nexts[h].node
			node.nexts[h].mu.RUnlock()

			if next == nil || f(next.K) >= 0 {
				break
			}
			node = next
		}
	}

	node.nexts[0].mu.RLock()
	next := node.nexts[0].node
	node.nexts[0].mu.RUnlock()

	if next == nil || f(next.K) != 0 {
		return nil
	}
	return next
}

func (l *skipList[K, V]) FindAll(f func(K) int) []V {
	var ret []V
	node := l.findFirstNodeFunc(f)
	for node != nil && f(node.K) == 0 {
		ret = append(ret, node.V)

		node.nexts[0].mu.RLock()
		next := node.nexts[0].node
		node.nexts[0].mu.RUnlock()

		node = next
	}
	return ret
}

func (l *skipList[K, V]) Add(k K, v V) (added bool) {
	var ok bool
	for {
		if added, ok = l.tryAdd(k, v); ok {
			if added {
				l.lenInc()
			}
			return
		}
	}
}

func (l *skipList[K, V]) tryAdd(k K, v V) (added, ok bool) {
	type traceElem[K, V any] struct {
		Node, Next *skipListNode[K, V]
	}

	height := l.randHeight()

	// Get trace
	trace := make([]traceElem[K, V], height)
	node := l.Head
	for h := l.MaxHeight - 1; h >= 0; h-- {
		var next *skipListNode[K, V]
		for {
			node.nexts[h].mu.RLock()
			next = node.nexts[h].node
			node.nexts[h].mu.RUnlock()

			if next == nil || l.CmpFunc(next.K, k) > 0 {
				break
			}

			node = next
		}

		if node != l.Head && l.CmpFunc(node.K, k) == 0 {
			ok = true
			return
		}

		if h < height {
			trace[h] = traceElem[K, V]{
				Node: node,
				Next: next,
			}
		}
	}

	// Try insert
	newNode := newSkipListNode(k, v, height)
	for h := height - 1; h >= 0; h-- {
		trace[h].Node.nexts[h].mu.Lock()
		defer trace[h].Node.nexts[h].mu.Unlock()

		if trace[h].Node.nexts[h].node != trace[h].Next {
			return
		}
	}

	for h := height - 1; h >= 0; h-- {
		newNode.nexts[h].node = trace[h].Next
		trace[h].Node.nexts[h].node = newNode
	}

	return true, true
}

func (l *skipList[K, V]) randHeight() int {
	l.rndMu.Lock()
	n := l.rnd.Uint32()
	l.rndMu.Unlock()
	return bits.LeadingZeros16(uint16(n)|math.MaxUint16>>(l.MaxHeight-1)) + 1
}

func (l *skipList[K, V]) Delete(k K) (deleted bool) {
	var ok bool
	for {
		if deleted, ok = l.tryDelete(k); ok {
			if deleted {
				l.lenDec()
			}
			return
		}
	}
}

func (l *skipList[K, V]) tryDelete(k K) (deleted, ok bool) {
	var trace []*skipListNode[K, V]
	node := l.Head
	var target *skipListNode[K, V]
	for h := l.MaxHeight - 1; h >= 0; h-- {
		c := -100
		for {
			node.nexts[h].mu.RLock()
			next := node.nexts[h].node
			node.nexts[h].mu.RUnlock()

			if next == nil {
				break
			}
			if c = l.CmpFunc(next.K, k); c >= 0 {
				if c == 0 {
					node.nexts[h].mu.Lock()
					defer node.nexts[h].mu.Unlock()

					if node.nexts[h].node != next {
						return
					}

					if target == nil {
						trace = make([]*skipListNode[K, V], h+1)
						target = next
					} else if next != target {
						ok = true
						return
					}

					trace[h] = node
				}
				break
			}

			node = next
		}
		if trace != nil && trace[h] == nil {
			ok = true
			return
		}
	}
	if target == nil {
		ok = true
		return
	}

	for h := len(trace) - 1; h >= 0; h-- {
		target.nexts[h].mu.Lock()
		defer target.nexts[h].mu.Unlock()
	}
	for h := len(trace) - 1; h >= 0; h-- {
		trace[h].nexts[h].node = target.nexts[h].node
	}

	return true, true
}

type skipListNode[K, V any] struct {
	K K
	V V

	nexts []struct {
		mu   sync.RWMutex
		node *skipListNode[K, V]
	}
}

func newSkipListNode[K, V any](k K, v V, height int) *skipListNode[K, V] {
	if height <= 0 {
		panicf("skipListNode height must be positive integer but got %d", height)
	}

	return &skipListNode[K, V]{
		K: k,
		V: v,
		nexts: make([]struct {
			mu   sync.RWMutex
			node *skipListNode[K, V]
		}, height),
	}
}

func (nd *skipListNode[K, V]) Next() *skipListNode[K, V] {
	if nd == nil {
		return nil
	}
	nd.nexts[0].mu.RLock()
	defer nd.nexts[0].mu.RUnlock()
	return nd.nexts[0].node
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
