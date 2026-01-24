//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// MaxEventTagsMiddleware limits the number of tags in an event.
type MaxEventTagsMiddleware struct {
	maxTags int
}

// NewMaxEventTagsMiddleware creates a new MaxEventTagsMiddleware.
// maxTags must be a positive integer.
func NewMaxEventTagsMiddleware(maxTags int) Middleware {
	if maxTags < 1 {
		panic("maxTags must be positive")
	}
	return NewSimpleMiddleware(&MaxEventTagsMiddleware{maxTags: maxTags})
}

func (m *MaxEventTagsMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *MaxEventTagsMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *MaxEventTagsMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event != nil && len(msg.Event.Tags) > m.maxTags {
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: too many tags")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *MaxEventTagsMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
