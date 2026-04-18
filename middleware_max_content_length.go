package mocrelay

import (
	"context"
	"unicode/utf8"
)

// NewMaxContentLengthMiddlewareBase returns a middleware base that limits the
// number of Unicode characters (not bytes) in event content.
//
// maxLen must be a positive integer.
func NewMaxContentLengthMiddlewareBase(maxLen int) SimpleMiddlewareBase {
	if maxLen < 1 {
		panic("maxLen must be positive")
	}
	return &maxContentLengthMiddleware{maxLen: maxLen}
}

type maxContentLengthMiddleware struct {
	maxLen int
}

func (m *maxContentLengthMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *maxContentLengthMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *maxContentLengthMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event != nil && utf8.RuneCountInString(msg.Event.Content) > m.maxLen {
		logRejection(ctx, "max_content_length", "content_too_long",
			"event_id", msg.Event.ID,
			"length", utf8.RuneCountInString(msg.Event.Content),
			"limit", m.maxLen,
		)
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: content too long")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *maxContentLengthMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
