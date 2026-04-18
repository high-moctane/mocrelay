package mocrelay

import (
	"context"
)

// NewMaxLimitMiddlewareBase returns a middleware base that clamps the limit
// value in REQ filters and applies a default when unset.
//
// maxLimit is the maximum allowed limit value (must be positive).
// defaultLimit is applied when no limit is specified (must be positive and
// <= maxLimit).
func NewMaxLimitMiddlewareBase(maxLimit, defaultLimit int64) SimpleMiddlewareBase {
	if maxLimit < 1 {
		panic("maxLimit must be positive")
	}
	if defaultLimit < 1 {
		panic("defaultLimit must be positive")
	}
	if defaultLimit > maxLimit {
		panic("defaultLimit must not exceed maxLimit")
	}
	return &maxLimitMiddleware{
		maxLimit:     maxLimit,
		defaultLimit: defaultLimit,
	}
}

type maxLimitMiddleware struct {
	maxLimit     int64
	defaultLimit int64
}

func (m *maxLimitMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	return ctx, nil, nil
}

func (m *maxLimitMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *maxLimitMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
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

func (m *maxLimitMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	return msg, nil
}
