//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
)

// MaxLimitMiddleware clamps the limit value in REQ filters and applies a default.
type MaxLimitMiddleware struct {
	maxLimit     int64
	defaultLimit int64
}

// NewMaxLimitMiddleware creates a new MaxLimitMiddleware.
// maxLimit is the maximum allowed limit value (must be positive).
// defaultLimit is applied when no limit is specified (must be positive and <= maxLimit).
func NewMaxLimitMiddleware(maxLimit, defaultLimit int64) Middleware {
	if maxLimit < 1 {
		panic("maxLimit must be positive")
	}
	if defaultLimit < 1 {
		panic("defaultLimit must be positive")
	}
	if defaultLimit > maxLimit {
		panic("defaultLimit must not exceed maxLimit")
	}
	return NewSimpleMiddleware(&MaxLimitMiddleware{
		maxLimit:     maxLimit,
		defaultLimit: defaultLimit,
	})
}

func (m *MaxLimitMiddleware) OnStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *MaxLimitMiddleware) OnEnd(ctx context.Context) error {
	return nil
}

func (m *MaxLimitMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeReq {
		return msg, nil, nil
	}

	// Clamp or apply default to each filter's limit
	for _, filter := range msg.Filters {
		if filter.Limit == nil || *filter.Limit == 0 {
			// Apply default limit
			filter.Limit = &m.defaultLimit
		} else if *filter.Limit > m.maxLimit {
			// Clamp to max
			filter.Limit = &m.maxLimit
		}
	}

	return msg, nil, nil
}

func (m *MaxLimitMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
