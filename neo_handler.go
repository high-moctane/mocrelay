package mocrelay

import (
	"context"
)

type NeoHandler interface {
	NeoServeNostr(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) error
}

type NeoHandlerFunc func(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) error

func (f NeoHandlerFunc) NeoServeNostr(
	ctx context.Context,
	send chan<- ServerMsg,
	recv <-chan ClientMsg,
) error {
	return f(ctx, send, recv)
}

type NeoSimpleHandlerBase interface {
	NeoServeNostrOnStart(ctx context.Context) (newCtx context.Context, err error)
	NeoServeNostrClientMsg(ctx context.Context, msg ClientMsg) (<-chan ServerMsg, error)
	NeoServeNostrOnEnd(ctx context.Context, serveErr error) error
}

type NeoSimpleHandler NeoHandlerFunc

func NewNeoSimpleHandler(base NeoSimpleHandlerBase) NeoHandler {
	return NeoHandlerFunc(func(ctx context.Context, send chan<- ServerMsg, recv <-chan ClientMsg) (err error) {
		defer func() {
			err = base.NeoServeNostrOnEnd(ctx, err)
		}()

		ctx, err = base.NeoServeNostrOnStart(ctx)
		if err != nil {
			return
		}

		err = func() error {
			for cmsg, err := range recvCtx(ctx, recv) {
				if err != nil {
					return err
				}

				smsgCh, err := base.NeoServeNostrClientMsg(ctx, cmsg)
				if err != nil {
					return err
				}
				if smsgCh == nil {
					continue
				}

				for smsg, err := range recvCtx(ctx, smsgCh) {
					if err != nil {
						if err == errRecvCtxChanClosed {
							break
						}
						return err
					}
					if err := sendCtx(ctx, send, smsg); err != nil {
						return err
					}
				}
			}

			panic("unreachable")
		}()

		if err == errRecvCtxChanClosed {
			err = ErrRecvClosed
		}
		return
	})
}
