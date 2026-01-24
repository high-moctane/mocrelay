//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"fmt"
	"strconv"
)

// MinPowDifficultyMiddleware is a middleware that validates Proof of Work (NIP-13).
type MinPowDifficultyMiddleware struct {
	// MinDifficulty is the minimum number of leading zero bits required.
	MinDifficulty int

	// CheckCommitment controls whether to check the nonce tag's target difficulty.
	// If true, events with a committed target difficulty lower than MinDifficulty
	// will be rejected, even if the actual difficulty is sufficient.
	// This prevents spammers from getting lucky with low-target mining.
	CheckCommitment bool
}

func (m *MinPowDifficultyMiddleware) OnStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *MinPowDifficultyMiddleware) OnEnd(ctx context.Context) error {
	return nil
}

func (m *MinPowDifficultyMiddleware) HandleClientMsg(
	ctx context.Context,
	msg *ClientMsg,
) (*ClientMsg, *ServerMsg, error) {
	if msg.Type != MsgTypeEvent || msg.Event == nil {
		return msg, nil, nil
	}

	event := msg.Event

	// Check actual difficulty
	actualDifficulty := CountLeadingZeroBits(event.ID)
	if actualDifficulty < m.MinDifficulty {
		return nil, NewServerOKMsg(
			event.ID,
			false,
			fmt.Sprintf("pow: difficulty %d is less than %d required", actualDifficulty, m.MinDifficulty),
		), nil
	}

	// Check committed target difficulty (if enabled)
	if m.CheckCommitment {
		committedTarget := getCommittedPowTarget(event)
		if committedTarget >= 0 && committedTarget < m.MinDifficulty {
			return nil, NewServerOKMsg(
				event.ID,
				false,
				fmt.Sprintf("pow: committed target %d is less than %d required", committedTarget, m.MinDifficulty),
			), nil
		}
	}

	return msg, nil, nil
}

func (m *MinPowDifficultyMiddleware) HandleServerMsg(
	ctx context.Context,
	msg *ServerMsg,
) (*ServerMsg, error) {
	return msg, nil
}

// getCommittedPowTarget extracts the target difficulty from the nonce tag.
// Returns -1 if no nonce tag or no target is specified.
//
// Example: ["nonce", "776797", "20"] -> returns 20
func getCommittedPowTarget(event *Event) int {
	for _, tag := range event.Tags {
		if len(tag) >= 3 && tag[0] == "nonce" {
			target, err := strconv.Atoi(tag[2])
			if err != nil {
				return -1
			}
			return target
		}
	}
	return -1
}

// NewMinPowDifficultyMiddleware creates a middleware that validates Proof of Work.
//
// Parameters:
//   - minDifficulty: minimum number of leading zero bits required in event ID
//   - checkCommitment: if true, also validates the nonce tag's target difficulty
func NewMinPowDifficultyMiddleware(minDifficulty int, checkCommitment bool) Middleware {
	return NewSimpleMiddleware(&MinPowDifficultyMiddleware{
		MinDifficulty:   minDifficulty,
		CheckCommitment: checkCommitment,
	})
}
