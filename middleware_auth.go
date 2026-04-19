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
//
// Metrics are not part of this struct. [AuthMetrics] is injected via the
// request context by [Relay] (see [RelayOptions.AuthMetrics]) and read by
// the auth middleware via [AuthMetricsFromContext], mirroring the
// [RejectionMetrics] / Logger pattern.
type AuthMiddlewareOptions struct {
	// CreatedAtTolerance is the maximum age of auth events.
	// Default: 10 minutes (as recommended by NIP-42)
	CreatedAtTolerance time.Duration
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
	}
}

type authMiddleware struct {
	relayURL           string
	createdAtTolerance time.Duration
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
	if metrics := AuthMetricsFromContext(ctx); metrics != nil {
		state := ctx.Value(authCtxKey{}).(*authState)
		state.mu.RLock()
		authenticated := state.authenticated
		state.mu.RUnlock()

		if authenticated {
			metrics.AuthenticatedConnectionsCurrent.Dec()
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
		return m.handleAuth(ctx, state, msg)

	case MsgTypeEvent:
		if !m.isAuthedPubkey(state, msg.Event.Pubkey) {
			logRejection(ctx, "auth", "event_unauthenticated",
				"event_id", msg.Event.ID,
				"pubkey", msg.Event.Pubkey,
			)
			return nil, NewServerOKMsg(
				msg.Event.ID,
				false,
				"auth-required: authentication required to publish events",
			), nil
		}
		return msg, nil, nil

	case MsgTypeReq:
		if !m.isAuthed(state) {
			logRejection(ctx, "auth", "req_unauthenticated",
				"sub_id", msg.SubscriptionID,
			)
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

func (m *authMiddleware) handleAuth(ctx context.Context, state *authState, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	metrics := AuthMetricsFromContext(ctx)

	if msg.Event == nil {
		logRejection(ctx, "auth", "missing_auth_event")
		return nil, NewServerOKMsg("", false, "invalid: missing auth event"), nil
	}

	event := msg.Event

	// Validate kind
	if event.Kind != 22242 {
		if metrics != nil {
			metrics.AuthTotal.WithLabelValues("invalid_kind").Inc()
		}
		logRejection(ctx, "auth", "invalid_kind",
			"event_id", event.ID,
			"kind", event.Kind,
		)
		return nil, NewServerOKMsg(event.ID, false, "invalid: auth event must be kind 22242"), nil
	}

	// Validate created_at (within tolerance)
	now := time.Now()
	if event.CreatedAt.Before(now.Add(-m.createdAtTolerance)) || event.CreatedAt.After(now.Add(m.createdAtTolerance)) {
		if metrics != nil {
			metrics.AuthTotal.WithLabelValues("expired").Inc()
		}
		logRejection(ctx, "auth", "expired",
			"event_id", event.ID,
			"created_at", event.CreatedAt.Unix(),
		)
		return nil, NewServerOKMsg(event.ID, false, "invalid: auth event created_at out of range"), nil
	}

	// Validate challenge tag
	challengeTag := findTagValue(event.Tags, "challenge")
	state.mu.RLock()
	expectedChallenge := state.challenge
	state.mu.RUnlock()

	if challengeTag != expectedChallenge {
		if metrics != nil {
			metrics.AuthTotal.WithLabelValues("challenge_mismatch").Inc()
		}
		logRejection(ctx, "auth", "challenge_mismatch",
			"event_id", event.ID,
		)
		return nil, NewServerOKMsg(event.ID, false, "invalid: challenge mismatch"), nil
	}

	// Validate relay tag (simple domain check)
	relayTag := findTagValue(event.Tags, "relay")
	if m.relayURL != "" && relayTag != m.relayURL {
		// Simple check: just verify it's not empty for now
		// More sophisticated URL normalization could be added
		if relayTag == "" {
			if metrics != nil {
				metrics.AuthTotal.WithLabelValues("invalid_relay").Inc()
			}
			logRejection(ctx, "auth", "missing_relay_tag",
				"event_id", event.ID,
			)
			return nil, NewServerOKMsg(event.ID, false, "invalid: missing relay tag"), nil
		}
	}

	// Authentication successful
	if metrics != nil {
		metrics.AuthTotal.WithLabelValues("success").Inc()
	}

	state.mu.Lock()
	state.authedPubkeys[event.Pubkey] = struct{}{}
	if metrics != nil && !state.authenticated {
		state.authenticated = true
		metrics.AuthenticatedConnectionsCurrent.Inc()
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
