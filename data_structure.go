package main

import (
	"fmt"
)

type TryChan[T any] chan T

func NewTryChan[T any](size int) TryChan[T] {
	return make(chan T, size)
}

func (ch TryChan[T]) TrySend(v T) bool {
	select {
	case ch <- v:
		return true
	default:
		return false
	}
}

type KeyValueCache[K comparable, V any] struct {
	size int

	m   map[K]int
	ks  []K
	vs  []V
	ptr int
}

func NewKeyValueCache[K comparable, V any](size int) *KeyValueCache[K, V] {
	if size < 1 {
		print(fmt.Sprintf("event cache size must be a positive number but %v", size))
	}

	return &KeyValueCache[K, V]{
		size: size,
		m:    make(map[K]int, size),
		ks:   make([]K, 0, size),
		vs:   make([]V, 0, size),
		ptr:  0,
	}
}

func (c *KeyValueCache[K, V]) Push(k K, v V) (exist bool) {
	if _, exist = c.m[k]; exist {
		return
	}

	if len(c.ks) < c.size {
		c.ks = append(c.ks, k)
		c.vs = append(c.vs, v)
	} else {
		delete(c.m, c.ks[c.ptr])
		c.ks[c.ptr] = k
		c.vs[c.ptr] = v
	}

	c.m[k] = c.ptr
	c.ptr = (c.size + c.ptr + 1) % c.size

	return
}

func (c *KeyValueCache[K, V]) Find(k K) (v V, found bool) {
	var idx int
	idx, found = c.m[k]
	if !found {
		return
	}
	return c.vs[idx], true
}

func (c *KeyValueCache[K, V]) Delete(k K) (found bool) {
	if _, found = c.m[k]; !found {
		return
	}

	if found {
		delete(c.m, k)
	}

	return
}

func (c *KeyValueCache[K, V]) DeleteAll(cond func(K, V) bool) (found bool) {
	var willDelete []K

	for k, idx := range c.m {
		if cond(c.ks[idx], c.vs[idx]) {
			found = true
			willDelete = append(willDelete, k)
		}
	}

	for _, k := range willDelete {
		delete(c.m, k)
	}

	return
}

func (c *KeyValueCache[K, V]) Loop(do func(K, V) bool) {
	mod := func(n int) int { return (len(c.ks) + n) % len(c.ks) }

	idx := mod(c.ptr - 1)
	for {
		if _, ok := c.m[c.ks[idx]]; ok {
			if next := do(c.ks[idx], c.vs[idx]); !next {
				break
			}
		}

		if idx == mod(c.ptr) {
			break
		}
		idx = mod(idx - 1)
	}
}
