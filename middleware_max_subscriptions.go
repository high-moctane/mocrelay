package mocrelay

import (
	"context"
	"sync"
)

// NewMaxSubscriptionsMiddlewareBase returns a middleware base that limits the
// number of concurrent subscriptions per connection. maxSubs must be a
// positive integer.
func NewMaxSubscriptionsMiddlewareBase(maxSubs int) SimpleMiddlewareBase {
	if maxSubs < 1 {
		panic("maxSubs must be positive")
	}
	return &maxSubscriptionsMiddleware{maxSubs: maxSubs}
}

type maxSubscriptionsMiddleware struct {
	maxSubs int
}

type maxSubsCtxKey struct{}

type maxSubsState struct {
	mu   sync.Mutex
	subs map[string]struct{} // set of subscription IDs
}

func (m *maxSubscriptionsMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	state := &maxSubsState{
		subs: make(map[string]struct{}),
	}
	return context.WithValue(ctx, maxSubsCtxKey{}, state), nil, nil
}

func (m *maxSubscriptionsMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *maxSubscriptionsMiddleware) HandleClientMsg(ctx context.Context, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	state := ctx.Value(maxSubsCtxKey{}).(*maxSubsState)

	switch msg.Type {
	case MsgTypeReq:
		return m.handleReq(state, msg)
	case MsgTypeClose:
		return m.handleClose(state, msg)
	default:
		return msg, nil, nil
	}
}

func (m *maxSubscriptionsMiddleware) handleReq(state *maxSubsState, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	subID := msg.SubscriptionID

	// If this subscription ID already exists, it's a replacement (allowed)
	if _, exists := state.subs[subID]; exists {
		return msg, nil, nil
	}

	// Check if we've reached the limit
	if len(state.subs) >= m.maxSubs {
		// Reject with CLOSED message
		resp := NewServerClosedMsg(subID, "error: too many subscriptions")
		return nil, resp, nil
	}

	// Add the new subscription
	state.subs[subID] = struct{}{}
	return msg, nil, nil
}

func (m *maxSubscriptionsMiddleware) handleClose(state *maxSubsState, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	delete(state.subs, msg.SubscriptionID)
	return msg, nil, nil
}

func (m *maxSubscriptionsMiddleware) HandleServerMsg(ctx context.Context, msg *ServerMsg) (*ServerMsg, error) {
	// Also track CLOSED messages from downstream to update our state
	if msg.Type == MsgTypeClosed {
		state := ctx.Value(maxSubsCtxKey{}).(*maxSubsState)
		state.mu.Lock()
		delete(state.subs, msg.SubscriptionID)
		state.mu.Unlock()
	}
	return msg, nil
}
