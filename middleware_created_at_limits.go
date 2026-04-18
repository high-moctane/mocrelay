package mocrelay

import (
	"context"
	"time"
)

// NewCreatedAtLimitsMiddlewareBase returns a middleware base that limits the
// created_at timestamp of events.
//
// lowerLimit is the maximum age in seconds (how far into the past is allowed).
// upperLimit is the maximum seconds into the future allowed. Both must be
// non-negative.
func NewCreatedAtLimitsMiddlewareBase(lowerLimit, upperLimit int64) SimpleMiddlewareBase {
	if lowerLimit < 0 {
		panic("lowerLimit must be non-negative")
	}
	if upperLimit < 0 {
		panic("upperLimit must be non-negative")
	}
	return &createdAtLimitsMiddleware{
		lowerLimit: lowerLimit,
		upperLimit: upperLimit,
		now:        time.Now,
	}
}

type createdAtLimitsMiddleware struct {
	lowerLimit int64 // seconds into the past
	upperLimit int64 // seconds into the future
	now        func() time.Time
}

func (m *createdAtLimitsMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *createdAtLimitsMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *createdAtLimitsMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
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
		logRejection(ctx, "created_at_limits", "too_old",
			"event_id", msg.Event.ID,
			"created_at", createdAt.Unix(),
			"lower_limit_sec", m.lowerLimit,
		)
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: created_at too old")
		return nil, resp, nil
	}

	// Check upper limit (too far in the future)
	newest := now.Add(time.Duration(m.upperLimit) * time.Second)
	if createdAt.After(newest) {
		logRejection(ctx, "created_at_limits", "too_far_future",
			"event_id", msg.Event.ID,
			"created_at", createdAt.Unix(),
			"upper_limit_sec", m.upperLimit,
		)
		resp := NewServerOKMsg(msg.Event.ID, false, "invalid: created_at too far in the future")
		return nil, resp, nil
	}

	return msg, nil, nil
}

func (m *createdAtLimitsMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
