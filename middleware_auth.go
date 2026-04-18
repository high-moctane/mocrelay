package mocrelay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// AuthMiddlewareOptions configures the middleware returned by
// [NewAuthMiddlewareBase]. All fields are optional; the zero value gives
// sensible defaults.
type AuthMiddlewareOptions struct {
	// CreatedAtTolerance is the maximum age of auth events.
	// Default: 10 minutes (as recommended by NIP-42)
	CreatedAtTolerance time.Duration

	// Metrics is the Prometheus metrics collector for authentication.
	// If nil, no metrics are collected.
	Metrics *AuthMetrics
}

// NewAuthMiddlewareBase creates a middleware base that requires NIP-42
// authentication.
//
// relayURL is the URL of this relay (used for relay tag validation).
// opts can be nil for default options.
func NewAuthMiddlewareBase(relayURL string, opts *AuthMiddlewareOptions) SimpleMiddlewareBase {
	if opts == nil {
		opts = &AuthMiddlewareOptions{}
	}

	tolerance := opts.CreatedAtTolerance
	if tolerance == 0 {
		tolerance = 10 * time.Minute
	}

	return &authMiddleware{
		relayURL:           relayURL,
		createdAtTolerance: tolerance,
		metrics:            opts.Metrics,
	}
}

type authMiddleware struct {
	relayURL           string
	createdAtTolerance time.Duration
	metrics            *AuthMetrics
}

// authState holds per-connection authentication state.
type authState struct {
	mu            sync.RWMutex
	challenge     string
	authedPubkeys map[string]struct{}
	authenticated bool // true after first successful auth (for gauge tracking)
}

type authCtxKey struct{}

func (m *authMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	// Generate random challenge
	challenge, err := generateChallenge()
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to generate challenge: %w", err)
	}

	state := &authState{
		challenge:     challenge,
		authedPubkeys: make(map[string]struct{}),
	}
	ctx = context.WithValue(ctx, authCtxKey{}, state)

	// Send AUTH challenge
	return ctx, NewServerAuthMsg(challenge), nil
}

func (m *authMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	if m.metrics != nil {
		state := ctx.Value(authCtxKey{}).(*authState)
		state.mu.RLock()
		authenticated := state.authenticated
		state.mu.RUnlock()

		if authenticated {
			m.metrics.AuthenticatedConnectionsCurrent.Dec()
		}
	}

	return nil, nil
}

func (m *authMiddleware) HandleClientMsg(
	ctx context.Context,
	msg *ClientMsg,
) (*ClientMsg, *ServerMsg, error) {
	state := ctx.Value(authCtxKey{}).(*authState)

	switch msg.Type {
	case MsgTypeAuth:
		return m.handleAuth(state, msg)

	case MsgTypeEvent:
		if !m.isAuthedPubkey(state, msg.Event.Pubkey) {
			if m.metrics != nil {
				m.metrics.RejectionsTotal.WithLabelValues("event_unauthenticated").Inc()
			}
			return nil, NewServerOKMsg(
				msg.Event.ID,
				false,
				"auth-required: authentication required to publish events",
			), nil
		}
		return msg, nil, nil

	case MsgTypeReq:
		if !m.isAuthed(state) {
			if m.metrics != nil {
				m.metrics.RejectionsTotal.WithLabelValues("req_unauthenticated").Inc()
			}
			return nil, NewServerClosedMsg(
				msg.SubscriptionID,
				"auth-required: authentication required to subscribe",
			), nil
		}
		return msg, nil, nil

	default:
		// CLOSE and other messages pass through
		return msg, nil, nil
	}
}

func (m *authMiddleware) HandleServerMsg(
	ctx context.Context,
	msg *ServerMsg,
) (*ServerMsg, error) {
	return msg, nil
}

func (m *authMiddleware) handleAuth(state *authState, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Event == nil {
		return nil, NewServerOKMsg("", false, "invalid: missing auth event"), nil
	}

	event := msg.Event

	// Validate kind
	if event.Kind != 22242 {
		if m.metrics != nil {
			m.metrics.AuthTotal.WithLabelValues("invalid_kind").Inc()
		}
		return nil, NewServerOKMsg(event.ID, false, "invalid: auth event must be kind 22242"), nil
	}

	// Validate created_at (within tolerance)
	now := time.Now()
	if event.CreatedAt.Before(now.Add(-m.createdAtTolerance)) || event.CreatedAt.After(now.Add(m.createdAtTolerance)) {
		if m.metrics != nil {
			m.metrics.AuthTotal.WithLabelValues("expired").Inc()
		}
		return nil, NewServerOKMsg(event.ID, false, "invalid: auth event created_at out of range"), nil
	}

	// Validate challenge tag
	challengeTag := findTagValue(event.Tags, "challenge")
	state.mu.RLock()
	expectedChallenge := state.challenge
	state.mu.RUnlock()

	if challengeTag != expectedChallenge {
		if m.metrics != nil {
			m.metrics.AuthTotal.WithLabelValues("challenge_mismatch").Inc()
		}
		return nil, NewServerOKMsg(event.ID, false, "invalid: challenge mismatch"), nil
	}

	// Validate relay tag (simple domain check)
	relayTag := findTagValue(event.Tags, "relay")
	if m.relayURL != "" && relayTag != m.relayURL {
		// Simple check: just verify it's not empty for now
		// More sophisticated URL normalization could be added
		if relayTag == "" {
			if m.metrics != nil {
				m.metrics.AuthTotal.WithLabelValues("invalid_relay").Inc()
			}
			return nil, NewServerOKMsg(event.ID, false, "invalid: missing relay tag"), nil
		}
	}

	// Authentication successful
	if m.metrics != nil {
		m.metrics.AuthTotal.WithLabelValues("success").Inc()
	}

	state.mu.Lock()
	state.authedPubkeys[event.Pubkey] = struct{}{}
	if m.metrics != nil && !state.authenticated {
		state.authenticated = true
		m.metrics.AuthenticatedConnectionsCurrent.Inc()
	}
	state.mu.Unlock()

	return nil, NewServerOKMsg(event.ID, true, ""), nil
}

func (m *authMiddleware) isAuthed(state *authState) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return len(state.authedPubkeys) > 0
}

func (m *authMiddleware) isAuthedPubkey(state *authState, pubkey string) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	_, ok := state.authedPubkeys[pubkey]
	return ok
}

// findTagValue finds the first tag with the given name and returns its value.
// Returns empty string if not found.
func findTagValue(tags []Tag, name string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == name {
			return tag[1]
		}
	}
	return ""
}

// generateChallenge generates a random hex challenge string.
func generateChallenge() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
