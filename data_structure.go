package main

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
