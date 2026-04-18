package mocrelay

import (
	"context"
)

// NewMaxEventTagsMiddlewareBase returns a middleware base that limits the
// number of tags in an event. maxTags must be a positive integer.
func NewMaxEventTagsMiddlewareBase(maxTags int) SimpleMiddlewareBase {
	if maxTags < 1 {
		panic("maxTags must be positive")
	}
	return &maxEventTagsMiddleware{maxTags: maxTags}
}

type maxEventTagsMiddleware struct {
	maxTags int
}

func (m *maxEventTagsMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *maxEventTagsMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *maxEventTagsMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event != nil && len(msg.Event.Tags) > m.maxTags {
		logRejection(ctx, "max_event_tags", "too_many_tags",
			"event_id", msg.Event.ID,
			"tags", len(msg.Event.Tags),
			"limit", m.maxTags,
		)
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: too many tags")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *maxEventTagsMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
