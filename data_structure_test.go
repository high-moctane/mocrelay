package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyValueCache(t *testing.T) {
	c := NewKeyValueCache[string, int](4)

	c.Push("1", 1)
	c.Push("2", 2)
	c.Push("3", 3)

	var ks []string
	var vs []int
	do := func(k string, v int) bool {
		ks = append(ks, k)
		vs = append(vs, v)
		return true
	}
	c.Loop(do)

	assert.Equal(t, []string{"3", "2", "1"}, ks)
	assert.Equal(t, []int{3, 2, 1}, vs)

	ks = nil
	vs = nil

	c.Push("4", 4)
	c.Push("5", 5)
	c.Push("6", 6)
	c.Push("7", 7)
	c.Push("8", 8)
	c.Push("9", 9)

	c.Loop(do)

	assert.Equal(t, []string{"9", "8", "7", "6"}, ks)
	assert.Equal(t, []int{9, 8, 7, 6}, vs)

	ks = nil
	vs = nil
	c.Loop(func(k string, v int) bool {
		ks = append(ks, k)
		vs = append(vs, v)
		return false
	})

	assert.Equal(t, []string{"9"}, ks)
	assert.Equal(t, []int{9}, vs)

	v, ok := c.Find("9")
	assert.Equal(t, 9, v)
	assert.True(t, ok)
	v, ok = c.Find("8")
	assert.Equal(t, 8, v)
	assert.True(t, ok)
	v, ok = c.Find("7")
	assert.Equal(t, 7, v)
	assert.True(t, ok)
	v, ok = c.Find("6")
	assert.Equal(t, 6, v)
	assert.True(t, ok)
	v, ok = c.Find("5")
	assert.Equal(t, 0, v)
	assert.False(t, ok)
	v, ok = c.Find("4")
	assert.Equal(t, 0, v)
	assert.False(t, ok)

	assert.True(t, c.Delete("7"))
	v, ok = c.Find("7")
	assert.Equal(t, 0, v)
	assert.False(t, ok)

	ks = nil
	vs = nil
	c.Loop(func(k string, v int) bool {
		ks = append(ks, k)
		vs = append(vs, v)
		return true
	})
	assert.Equal(t, []string{"9", "8", "6"}, ks)
	assert.Equal(t, []int{9, 8, 6}, vs)

	c.DeleteAll(func(k string, v int) bool {
		return v%2 == 0
	})

	ks = nil
	vs = nil
	c.Loop(func(k string, v int) bool {
		ks = append(ks, k)
		vs = append(vs, v)
		return true
	})
	assert.Equal(t, []string{"9"}, ks)
	assert.Equal(t, []int{9}, vs)
}
