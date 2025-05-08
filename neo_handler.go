package mocrelay

import "context"

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
