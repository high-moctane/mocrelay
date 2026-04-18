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

// NewRestrictedWritesMiddlewareBase returns a middleware base that restricts
// which pubkeys can write events.
//
// mode determines whether the list is an allowlist or blocklist.
// pubkeys is the list of pubkeys to allow or block.
func NewRestrictedWritesMiddlewareBase(mode RestrictedWritesMode, pubkeys []string) SimpleMiddlewareBase {
	set := make(map[string]struct{}, len(pubkeys))
	for _, pk := range pubkeys {
		set[pk] = struct{}{}
	}
	return &restrictedWritesMiddleware{
		pubkeys: set,
		mode:    mode,
	}
}

type restrictedWritesMiddleware struct {
	pubkeys map[string]struct{}
	mode    RestrictedWritesMode
}

func (m *restrictedWritesMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *restrictedWritesMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *restrictedWritesMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
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

func (m *restrictedWritesMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
