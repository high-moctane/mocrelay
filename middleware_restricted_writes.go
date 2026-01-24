//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// RestrictedWritesMode determines how the pubkey list is interpreted.
type RestrictedWritesMode int

const (
	// RestrictedWritesModeAllowlist only allows pubkeys in the list.
	RestrictedWritesModeAllowlist RestrictedWritesMode = iota
	// RestrictedWritesModeBlocklist blocks pubkeys in the list.
	RestrictedWritesModeBlocklist
)

// RestrictedWritesMiddleware restricts which pubkeys can write events.
type RestrictedWritesMiddleware struct {
	pubkeys map[string]struct{}
	mode    RestrictedWritesMode
}

// NewRestrictedWritesMiddleware creates a new RestrictedWritesMiddleware.
// mode determines whether the list is an allowlist or blocklist.
// pubkeys is the list of pubkeys to allow or block.
func NewRestrictedWritesMiddleware(mode RestrictedWritesMode, pubkeys []string) Middleware {
	set := make(map[string]struct{}, len(pubkeys))
	for _, pk := range pubkeys {
		set[pk] = struct{}{}
	}
	return NewSimpleMiddleware(&RestrictedWritesMiddleware{
		pubkeys: set,
		mode:    mode,
	})
}

func (m *RestrictedWritesMiddleware) OnStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *RestrictedWritesMiddleware) OnEnd(ctx context.Context) error {
	return nil
}

func (m *RestrictedWritesMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event == nil {
		return msg, nil, nil
	}

	_, inList := m.pubkeys[msg.Event.Pubkey]

	switch m.mode {
	case RestrictedWritesModeAllowlist:
		if !inList {
			resp := NewServerOKMsg(msg.Event.ID, false, "restricted: pubkey not allowed")
			return nil, resp, nil
		}
	case RestrictedWritesModeBlocklist:
		if inList {
			resp := NewServerOKMsg(msg.Event.ID, false, "restricted: pubkey blocked")
			return nil, resp, nil
		}
	}

	return msg, nil, nil
}

func (m *RestrictedWritesMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
