package mocrelay

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
)

func panicf(format string, a ...any) {
	panic(fmt.Sprintf(format, a...))
}

func toPtr[T any](v T) *T { return &v }

func anySliceAs[T any](sli []any) ([]T, bool) {
	if sli == nil {
		return nil, true
	}

	ret := make([]T, len(sli))
	for i, v := range sli {
		vv, ok := v.(T)
		if !ok {
			return nil, false
		}
		ret[i] = vv
	}

	return ret, true
}

func sliceAllFunc[T any](vs []T, f func(v T) bool) bool {
	return !slices.ContainsFunc(vs, func(v T) bool { return !f(v) })
}

func sendCtx[T any](ctx context.Context, ch chan<- T, v T) (sent bool) {
	select {
	case <-ctx.Done():
		return false
	case ch <- v:
		return true
	}
}

func trySendCtx[T any](ctx context.Context, ch chan<- T, v T) (sent bool) {
	select {
	case <-ctx.Done():
		return false
	case ch <- v:
		return true
	default:
		return false
	}
}

func sendClientMsgCtx(ctx context.Context, ch chan<- ClientMsg, msg ClientMsg) (sent bool) {
	if isNilClientMsg(msg) {
		return
	}
	return sendCtx(ctx, ch, msg)
}

func sendServerMsgCtx(ctx context.Context, ch chan<- ServerMsg, msg ServerMsg) (sent bool) {
	if isNilServerMsg(msg) {
		return
	}
	return sendCtx(ctx, ch, msg)
}

type bufCh[T any] chan T

func newBufCh[T any](items ...T) bufCh[T] {
	ret := make(chan T, len(items))
	for _, item := range items {
		ret <- item
	}
	return ret
}

func newClosedBufCh[T any](items ...T) bufCh[T] {
	ret := newBufCh(items...)
	close(ret)
	return ret
}

func validHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !(('0' <= r && r <= '9') || ('a' <= r && r <= 'f')) {
			return false
		}
	}

	return true
}

var errCtxChClosed = errors.New("ctxCh closed")

func ctxCh[T any](ctx context.Context, ch <-chan T) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T

		for {
			select {
			case <-ctx.Done():
				for {
					if !yield(zero, ctx.Err()) {
						return
					}
				}

			case v, ok := <-ch:
				if !ok {
					for {
						if !yield(zero, errCtxChClosed) {
							return
						}
					}
				}
				if !yield(v, nil) {
					return
				}
			}
		}
	}
}
