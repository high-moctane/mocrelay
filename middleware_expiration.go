//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"strconv"
	"time"
)

// ExpirationMiddleware implements NIP-40: Expiration Timestamp.
// Events with an "expiration" tag are rejected if expired on receipt,
// and dropped (not delivered) if expired on send.
type ExpirationMiddleware struct {
	now func() time.Time
}

// NewExpirationMiddleware creates a new ExpirationMiddleware.
func NewExpirationMiddleware() Middleware {
	return NewSimpleMiddleware(&ExpirationMiddleware{
		now: time.Now,
	})
}

func (m *ExpirationMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *ExpirationMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *ExpirationMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent || msg.Event == nil {
		return msg, nil, nil
	}

	if m.isExpired(msg.Event) {
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: event has expired")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *ExpirationMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	// Drop expired events from being delivered
	if msg.Type == MsgTypeEvent && msg.Event != nil {
		if m.isExpired(msg.Event) {
			return nil, nil // drop
		}
	}

	return msg, nil
}

// isExpired checks if the event has an expiration tag and is expired.
func (m *ExpirationMiddleware) isExpired(event *Event) bool {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "expiration" {
			expiration, err := strconv.ParseInt(tag[1], 10, 64)
			if err != nil {
				// Invalid expiration tag, treat as not expired
				return false
			}
			return m.now().Unix() > expiration
		}
	}
	return false
}
