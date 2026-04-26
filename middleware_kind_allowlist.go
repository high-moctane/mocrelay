package mocrelay

import (
	"context"
)

// NewKindAllowlistMiddlewareBase returns a middleware base that rejects events
// whose kind is NOT in kinds. This is useful for restricting a relay to a
// well-known set of kinds (e.g., the kinds documented in the NIPs README).
//
// An empty allowlist rejects all events. Range-style kinds (e.g., NIP-90 Job
// Request 5000-5999) are expanded into individual entries by the caller:
//
//	kinds := []int64{0, 1, 3}
//	for k := int64(5000); k <= 5999; k++ {
//		kinds = append(kinds, k)
//	}
//	mw := NewKindAllowlistMiddlewareBase(kinds)
func NewKindAllowlistMiddlewareBase(kinds []int64) SimpleMiddlewareBase {
	allowlist := make(map[int64]struct{}, len(kinds))
	for _, k := range kinds {
		allowlist[k] = struct{}{}
	}
	return &kindAllowlistMiddleware{allowlist: allowlist}
}

type kindAllowlistMiddleware struct {
	allowlist map[int64]struct{}
}

func (m *kindAllowlistMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *kindAllowlistMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *kindAllowlistMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event == nil {
		return msg, nil, nil
	}

	if _, allowed := m.allowlist[msg.Event.Kind]; !allowed {
		logRejection(ctx, "kind_allowlist", "kind_blocked",
			"event_id", msg.Event.ID,
			"kind", msg.Event.Kind,
		)
		resp := NewServerOKMsg(msg.Event.ID, false, "blocked: kind not allowed")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *kindAllowlistMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
