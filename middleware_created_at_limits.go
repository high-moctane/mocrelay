//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"time"
)

// CreatedAtLimitsMiddleware limits the created_at timestamp of events.
type CreatedAtLimitsMiddleware struct {
	lowerLimit int64 // seconds into the past
	upperLimit int64 // seconds into the future
	now        func() time.Time
}

// NewCreatedAtLimitsMiddleware creates a new CreatedAtLimitsMiddleware.
// lowerLimit is the maximum age in seconds (how far into the past is allowed).
// upperLimit is the maximum seconds into the future allowed.
// Both must be non-negative.
func NewCreatedAtLimitsMiddleware(lowerLimit, upperLimit int64) Middleware {
	if lowerLimit < 0 {
		panic("lowerLimit must be non-negative")
	}
	if upperLimit < 0 {
		panic("upperLimit must be non-negative")
	}
	return NewSimpleMiddleware(&CreatedAtLimitsMiddleware{
		lowerLimit: lowerLimit,
		upperLimit: upperLimit,
		now:        time.Now,
	})
}

func (m *CreatedAtLimitsMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *CreatedAtLimitsMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *CreatedAtLimitsMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent {
		return msg, nil, nil
	}

	if msg.Event == nil {
		return msg, nil, nil
	}

	now := m.now()
	createdAt := msg.Event.CreatedAt

	// Check lower limit (too old)
	oldest := now.Add(-time.Duration(m.lowerLimit) * time.Second)
	if createdAt.Before(oldest) {
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: created_at too old")
		return nil, resp, nil
	}

	// Check upper limit (too far in the future)
	newest := now.Add(time.Duration(m.upperLimit) * time.Second)
	if createdAt.After(newest) {
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: created_at too far in the future")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *CreatedAtLimitsMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
