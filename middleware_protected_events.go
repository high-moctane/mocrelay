//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// ProtectedEventsMiddleware implements NIP-70: Protected Events.
// Events with a ["-"] tag can only be published by their authenticated author.
//
// This middleware requires NIP-42 authentication to be enabled upstream.
// If no authentication state is found in the context, protected events are always rejected.
type ProtectedEventsMiddleware struct{}

// NewProtectedEventsMiddleware creates a new ProtectedEventsMiddleware.
func NewProtectedEventsMiddleware() Middleware {
	return NewSimpleMiddleware(&ProtectedEventsMiddleware{})
}

func (m *ProtectedEventsMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *ProtectedEventsMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *ProtectedEventsMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent || msg.Event == nil {
		return msg, nil, nil
	}

	if !hasProtectedTag(msg.Event) {
		return msg, nil, nil
	}

	// Check if the authenticated pubkey matches the event pubkey
	if !isAuthedAsPubkey(ctx, msg.Event.Pubkey) {
		resp := NewServerOKMsg(
			msg.Event.ID,
			false,
			"auth-required: this event may only be published by its author",
		)
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *ProtectedEventsMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}

// hasProtectedTag checks if the event has a ["-"] tag (NIP-70).
func hasProtectedTag(event *Event) bool {
	for _, tag := range event.Tags {
		if len(tag) == 1 && tag[0] == "-" {
			return true
		}
	}
	return false
}

// isAuthedAsPubkey checks if the given pubkey is authenticated in the current session.
// This looks for the auth state set by AuthMiddleware (NIP-42).
func isAuthedAsPubkey(ctx context.Context, pubkey string) bool {
	state, ok := ctx.Value(authCtxKey{}).(*authState)
	if !ok || state == nil {
		// No auth state in context - NIP-42 is not enabled or not authenticated
		return false
	}

	state.mu.RLock()
	defer state.mu.RUnlock()

	_, authed := state.authedPubkeys[pubkey]
	return authed
}
