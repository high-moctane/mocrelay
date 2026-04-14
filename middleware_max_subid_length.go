package mocrelay

import (
	"context"
)

// MaxSubidLengthMiddleware limits the length of subscription IDs.
type MaxSubidLengthMiddleware struct {
	maxLen int
}

// NewMaxSubidLengthMiddlewareBase creates a new MaxSubidLengthMiddleware.
// maxLen must be a positive integer.
func NewMaxSubidLengthMiddlewareBase(maxLen int) SimpleMiddlewareBase {
	if maxLen < 1 {
		panic("maxLen must be positive")
	}
	return &MaxSubidLengthMiddleware{maxLen: maxLen}
}

func (m *MaxSubidLengthMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *MaxSubidLengthMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *MaxSubidLengthMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	switch msg.Type {
	case MsgTypeReq, MsgTypeClose:
		if len(msg.SubscriptionID) > m.maxLen {
			resp := NewServerClosedMsg(msg.SubscriptionID, "error: subscription ID too long")
			return nil, resp, nil
		}
	}
	return msg, nil, nil
}

func (m *MaxSubidLengthMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
