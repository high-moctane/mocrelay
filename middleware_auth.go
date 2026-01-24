//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// AuthMiddleware is a middleware that requires NIP-42 authentication.
type AuthMiddleware struct {
	// RelayURL is the URL of this relay (used for validation).
	// Example: "wss://relay.example.com/"
	RelayURL string

	// CreatedAtTolerance is the maximum age of auth events.
	// Default: 10 minutes (as recommended by NIP-42)
	CreatedAtTolerance time.Duration
}

// authState holds per-connection authentication state.
type authState struct {
	mu            sync.RWMutex
	challenge     string
	authedPubkeys map[string]struct{}
}

type authCtxKey struct{}

func (m *AuthMiddleware) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
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

func (m *AuthMiddleware) OnEnd(ctx context.Context) (*ServerMsg, error) {
	return nil, nil
}

func (m *AuthMiddleware) HandleClientMsg(
	ctx context.Context,
	msg *ClientMsg,
) (*ClientMsg, *ServerMsg, error) {
	state := ctx.Value(authCtxKey{}).(*authState)

	switch msg.Type {
	case MsgTypeAuth:
		return m.handleAuth(state, msg)

	case MsgTypeEvent:
		if !m.isAuthed(state) {
			return nil, NewServerOKMsg(
				msg.Event.ID,
				false,
				"auth-required: authentication required to publish events",
			), nil
		}
		return msg, nil, nil

	case MsgTypeReq:
		if !m.isAuthed(state) {
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

func (m *AuthMiddleware) HandleServerMsg(
	ctx context.Context,
	msg *ServerMsg,
) (*ServerMsg, error) {
	return msg, nil
}

func (m *AuthMiddleware) handleAuth(state *authState, msg *ClientMsg) (*ClientMsg, *ServerMsg, error) {
	if msg.Event == nil {
		return nil, NewServerOKMsg("", false, "invalid: missing auth event"), nil
	}

	event := msg.Event

	// Validate kind
	if event.Kind != 22242 {
		return nil, NewServerOKMsg(event.ID, false, "invalid: auth event must be kind 22242"), nil
	}

	// Validate created_at (within tolerance)
	tolerance := m.CreatedAtTolerance
	if tolerance == 0 {
		tolerance = 10 * time.Minute
	}
	now := time.Now()
	if event.CreatedAt.Before(now.Add(-tolerance)) || event.CreatedAt.After(now.Add(tolerance)) {
		return nil, NewServerOKMsg(event.ID, false, "invalid: auth event created_at out of range"), nil
	}

	// Validate challenge tag
	challengeTag := findTagValue(event.Tags, "challenge")
	state.mu.RLock()
	expectedChallenge := state.challenge
	state.mu.RUnlock()

	if challengeTag != expectedChallenge {
		return nil, NewServerOKMsg(event.ID, false, "invalid: challenge mismatch"), nil
	}

	// Validate relay tag (simple domain check)
	relayTag := findTagValue(event.Tags, "relay")
	if m.RelayURL != "" && relayTag != m.RelayURL {
		// Simple check: just verify it's not empty for now
		// More sophisticated URL normalization could be added
		if relayTag == "" {
			return nil, NewServerOKMsg(event.ID, false, "invalid: missing relay tag"), nil
		}
	}

	// Authentication successful
	state.mu.Lock()
	state.authedPubkeys[event.Pubkey] = struct{}{}
	state.mu.Unlock()

	return nil, NewServerOKMsg(event.ID, true, ""), nil
}

func (m *AuthMiddleware) isAuthed(state *authState) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return len(state.authedPubkeys) > 0
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

// NewAuthMiddleware creates a middleware that requires NIP-42 authentication.
//
// Parameters:
//   - relayURL: the URL of this relay (used for relay tag validation)
func NewAuthMiddleware(relayURL string) Middleware {
	return NewSimpleMiddleware(&AuthMiddleware{
		RelayURL:           relayURL,
		CreatedAtTolerance: 10 * time.Minute,
	})
}
