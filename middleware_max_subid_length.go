package mocrelay

import (
	"context"
)

// NewMaxSubidLengthMiddlewareBase returns a middleware base that limits the
// length of subscription IDs. maxLen must be a positive integer.
func NewMaxSubidLengthMiddlewareBase(maxLen int) SimpleMiddlewareBase {
	if maxLen < 1 {
		panic("maxLen must be positive")
	}
	return &maxSubidLengthMiddleware{maxLen: maxLen}
}

type maxSubidLengthMiddleware struct {
	maxLen int
}

func (m *maxSubidLengthMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *maxSubidLengthMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *maxSubidLengthMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	switch msg.Type {
	case MsgTypeReq, MsgTypeClose:
		if len(msg.SubscriptionID) > m.maxLen {
			logRejection(ctx, "max_subid_length", "subid_too_long",
				"sub_id_length", len(msg.SubscriptionID),
				"limit", m.maxLen,
			)
			resp := NewServerClosedMsg(msg.SubscriptionID, "error: subscription ID too long")
			return nil, resp, nil
		}
	}
	return msg, nil, nil
}

func (m *maxSubidLengthMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
