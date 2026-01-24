//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// KindBlacklistMiddleware rejects events with blacklisted kinds.
// This is useful for legal compliance (e.g., rejecting DM-related kinds
// for Japanese telecommunications law compliance).
type KindBlacklistMiddleware struct {
	blacklist map[int64]struct{}
}

// NewKindBlacklistMiddleware creates a new KindBlacklistMiddleware.
// kinds is the list of kind numbers to reject.
func NewKindBlacklistMiddleware(kinds []int64) Middleware {
	blacklist := make(map[int64]struct{}, len(kinds))
	for _, k := range kinds {
		blacklist[k] = struct{}{}
	}
	return NewSimpleMiddleware(&KindBlacklistMiddleware{blacklist: blacklist})
}

func (m *KindBlacklistMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *KindBlacklistMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *KindBlacklistMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event == nil {
		return msg, nil, nil
	}

	if _, blocked := m.blacklist[msg.Event.Kind]; blocked {
		resp := NewServerOKMsg(msg.Event.ID, false, "blocked: kind not allowed")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *KindBlacklistMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
