package mocrelay

import (
	"context"
)

// KindDenylistMiddleware rejects events with denied kinds.
// This is useful for legal compliance (e.g., rejecting DM-related kinds
// for Japanese telecommunications law compliance).
type KindDenylistMiddleware struct {
	denylist map[int64]struct{}
}

// NewKindDenylistMiddlewareBase creates a new KindDenylistMiddleware.
// kinds is the list of kind numbers to reject.
func NewKindDenylistMiddlewareBase(kinds []int64) SimpleMiddlewareBase {
	denylist := make(map[int64]struct{}, len(kinds))
	for _, k := range kinds {
		denylist[k] = struct{}{}
	}
	return &KindDenylistMiddleware{denylist: denylist}
}

// OnStart implements [SimpleMiddlewareBase].
func (m *KindDenylistMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

// OnEnd implements [SimpleMiddlewareBase].
func (m *KindDenylistMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

// HandleClientMsg implements [SimpleMiddlewareBase].
func (m *KindDenylistMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
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

// HandleServerMsg implements [SimpleMiddlewareBase].
func (m *KindDenylistMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
