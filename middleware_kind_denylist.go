package mocrelay

import (
	"context"
)

// NewKindDenylistMiddlewareBase returns a middleware base that rejects events
// whose kind is in kinds. This is useful for legal compliance (e.g., rejecting
// DM-related kinds for Japanese telecommunications law compliance).
func NewKindDenylistMiddlewareBase(kinds []int64) SimpleMiddlewareBase {
	denylist := make(map[int64]struct{}, len(kinds))
	for _, k := range kinds {
		denylist[k] = struct{}{}
	}
	return &kindDenylistMiddleware{denylist: denylist}
}

type kindDenylistMiddleware struct {
	denylist map[int64]struct{}
}

func (m *kindDenylistMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *kindDenylistMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *kindDenylistMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event == nil {
		return msg, nil, nil
	}

	if _, denied := m.denylist[msg.Event.Kind]; denied {
		resp := NewServerOKMsg(msg.Event.ID, false, "blocked: kind not allowed")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *kindDenylistMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
