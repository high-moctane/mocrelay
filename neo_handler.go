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
