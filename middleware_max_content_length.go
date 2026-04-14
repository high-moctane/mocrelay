package mocrelay

import (
	"context"
	"unicode/utf8"
)

// MaxContentLengthMiddleware limits the number of Unicode characters in event content.
type MaxContentLengthMiddleware struct {
	maxLen int
}

// NewMaxContentLengthMiddlewareBase creates a new MaxContentLengthMiddleware.
// maxLen is the maximum number of Unicode characters (not bytes).
// maxLen must be a positive integer.
func NewMaxContentLengthMiddlewareBase(maxLen int) SimpleMiddlewareBase {
	if maxLen < 1 {
		panic("maxLen must be positive")
	}
	return &MaxContentLengthMiddleware{maxLen: maxLen}
}

func (m *MaxContentLengthMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *MaxContentLengthMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *MaxContentLengthMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event != nil && utf8.RuneCountInString(msg.Event.Content) > m.maxLen {
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: content too long")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *MaxContentLengthMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
