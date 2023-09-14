package utils

import "fmt"

type TryChan[T any] chan T

func NewTryChan[T any](buflen int) TryChan[T] {
	if buflen < 0 {
		panic(fmt.Sprintf("buflen must be zero or more integer but %d", buflen))
	}
	return make(TryChan[T], buflen)
}

func (c TryChan[T]) TrySend(v T) bool {
	select {
	case c <- v:
		return true
	default:
		return false
	}
}
