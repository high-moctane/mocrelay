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

func sendCtx[T any](ctx context.Context, ch chan<- T, v T) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- v:
		return nil
	}
}

func trySendCtx[T any](ctx context.Context, ch chan<- T, v T) (sent bool, err error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case ch <- v:
		return true, nil
	default:
		return false, nil
	}
}

func sendClientMsgCtx(ctx context.Context, ch chan<- ClientMsg, msg ClientMsg) (sent bool) {
	if isNilClientMsg(msg) {
		return
	}
	return sendCtx(ctx, ch, msg) == nil
}

func sendServerMsgCtx(ctx context.Context, ch chan<- ServerMsg, msg ServerMsg) (sent bool) {
	if isNilServerMsg(msg) {
		return
	}
	return sendCtx(ctx, ch, msg) == nil
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

var errRecvCtxChanClosed = errors.New("channel closed")

func recvCtx[T any](ctx context.Context, ch <-chan T) iter.Seq2[T, error] {
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
						if !yield(zero, errRecvCtxChanClosed) {
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

func pipeCtx[T any](ctx context.Context, send chan<- T, recv <-chan T) error {
	for v, err := range recvCtx(ctx, recv) {
		if err != nil {
			return err
		}
		if err := sendCtx(ctx, send, v); err != nil {
			return err
		}
	}

	panic("unreachable")
}
