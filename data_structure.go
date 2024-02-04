package mocrelay

import (
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
