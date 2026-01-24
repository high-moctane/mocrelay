//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestMinPowDifficultyMiddleware(t *testing.T) {
	testTime := time.Unix(1651794653, 0)

	// Event with 36 leading zero bits (from NIP-13 example)
	// ID: 000000000e9d97a1ab09fc381030b346cdd7a142ad57e6df0b46dc9bef6c7e2d
	highPowEvent := &Event{
		ID:        "000000000e9d97a1ab09fc381030b346cdd7a142ad57e6df0b46dc9bef6c7e2d",
		Pubkey:    "a48380f4cfcc1ad5378294fcac36439770f9c878dd880ffa94bb74ea54a6f243",
		CreatedAt: testTime,
		Kind:      1,
		Tags:      []Tag{{"nonce", "776797", "20"}},
		Content:   "It's just me mining my own business",
		Sig:       "284622fc0a3f4f1303455d5175f7ba962a3300d136085b9566801bc2e0699de0c7e31e44c81fb40ad9049173742e904713c3594a1da0fc5d2382a25c11aba977",
	}

	// Event with no PoW (starts with non-zero)
	noPowEvent := &Event{
		ID:        "a48380f4cfcc1ad5378294fcac36439770f9c878dd880ffa94bb74ea54a6f243",
		Pubkey:    "a48380f4cfcc1ad5378294fcac36439770f9c878dd880ffa94bb74ea54a6f243",
		CreatedAt: testTime,
		Kind:      1,
		Tags:      []Tag{},
		Content:   "No PoW here",
		Sig:       "284622fc0a3f4f1303455d5175f7ba962a3300d136085b9566801bc2e0699de0c7e31e44c81fb40ad9049173742e904713c3594a1da0fc5d2382a25c11aba977",
	}

	// Event with high PoW but low committed target
	luckyEvent := &Event{
		ID:        "000000000e9d97a1ab09fc381030b346cdd7a142ad57e6df0b46dc9bef6c7e2d",
		Pubkey:    "a48380f4cfcc1ad5378294fcac36439770f9c878dd880ffa94bb74ea54a6f243",
		CreatedAt: testTime,
		Kind:      1,
		Tags:      []Tag{{"nonce", "123456", "10"}}, // Committed to only 10
		Content:   "Got lucky!",
		Sig:       "284622fc0a3f4f1303455d5175f7ba962a3300d136085b9566801bc2e0699de0c7e31e44c81fb40ad9049173742e904713c3594a1da0fc5d2382a25c11aba977",
	}

	// Event with no nonce tag
	noNonceEvent := &Event{
		ID:        "000000000e9d97a1ab09fc381030b346cdd7a142ad57e6df0b46dc9bef6c7e2d",
		Pubkey:    "a48380f4cfcc1ad5378294fcac36439770f9c878dd880ffa94bb74ea54a6f243",
		CreatedAt: testTime,
		Kind:      1,
		Tags:      []Tag{},
		Content:   "No nonce tag",
		Sig:       "284622fc0a3f4f1303455d5175f7ba962a3300d136085b9566801bc2e0699de0c7e31e44c81fb40ad9049173742e904713c3594a1da0fc5d2382a25c11aba977",
	}

	tests := []struct {
		name            string
		minDifficulty   int
		checkCommitment bool
		event           *Event
		wantAccepted    bool
		wantMsgContains string
	}{
		{
			name:            "sufficient PoW",
			minDifficulty:   20,
			checkCommitment: false,
			event:           highPowEvent,
			wantAccepted:    true,
		},
		{
			name:            "insufficient PoW",
			minDifficulty:   40,
			checkCommitment: false,
			event:           highPowEvent, // Has 36, need 40
			wantAccepted:    false,
			wantMsgContains: "pow: difficulty 36 is less than 40 required",
		},
		{
			name:            "no PoW at all",
			minDifficulty:   10,
			checkCommitment: false,
			event:           noPowEvent,
			wantAccepted:    false,
			wantMsgContains: "pow: difficulty 0 is less than 10 required",
		},
		{
			name:            "lucky event without commitment check",
			minDifficulty:   20,
			checkCommitment: false,
			event:           luckyEvent, // Has 36, committed to 10, need 20
			wantAccepted:    true,       // Passes because we don't check commitment
		},
		{
			name:            "lucky event with commitment check",
			minDifficulty:   20,
			checkCommitment: true,
			event:           luckyEvent, // Has 36, committed to 10, need 20
			wantAccepted:    false,      // Fails because committed target is too low
			wantMsgContains: "pow: committed target 10 is less than 20 required",
		},
		{
			name:            "no nonce tag with commitment check",
			minDifficulty:   20,
			checkCommitment: true,
			event:           noNonceEvent, // Has 36, no nonce tag
			wantAccepted:    true,         // Passes because no commitment to check
		},
		{
			name:            "exact difficulty match",
			minDifficulty:   36,
			checkCommitment: false,
			event:           highPowEvent,
			wantAccepted:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				downstream := NewNopHandler()
				middleware := NewMinPowDifficultyMiddleware(tt.minDifficulty, tt.checkCommitment)
				handler := middleware(downstream)

				recv := make(chan *ClientMsg, 1)
				send := make(chan *ServerMsg, 10)

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go handler.ServeNostr(ctx, send, recv)

				// Send EVENT message
				recv <- &ClientMsg{
					Type:  MsgTypeEvent,
					Event: tt.event,
				}

				synctest.Wait()

				if tt.wantAccepted {
					// Should pass through to downstream (NopHandler sends OK true)
					select {
					case msg := <-send:
						if msg.Type != MsgTypeOK {
							t.Errorf("expected OK message, got %v", msg.Type)
						}
						if !msg.Accepted {
							t.Errorf("expected accepted=true, got false with message: %s", msg.Message)
						}
					default:
						t.Error("expected OK message, got nothing")
					}
				} else {
					// Should be rejected by middleware
					select {
					case msg := <-send:
						if msg.Type != MsgTypeOK {
							t.Errorf("expected OK message, got %v", msg.Type)
						}
						if msg.Accepted {
							t.Error("expected accepted=false, got true")
						}
						if tt.wantMsgContains != "" && msg.Message != tt.wantMsgContains {
							t.Errorf("expected message %q, got %q", tt.wantMsgContains, msg.Message)
						}
					default:
						t.Error("expected OK message, got nothing")
					}
				}

				cancel()
			})
		})
	}
}

func TestMinPowDifficultyMiddleware_NonEventMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream := NewNopHandler()
		middleware := NewMinPowDifficultyMiddleware(20, true)
		handler := middleware(downstream)

		recv := make(chan *ClientMsg, 1)
		send := make(chan *ServerMsg, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ message (should pass through)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should get EOSE (REQ passed through to NopHandler)
		select {
		case msg := <-send:
			if msg.Type != MsgTypeEOSE {
				t.Errorf("expected EOSE message, got %v", msg.Type)
			}
		default:
			t.Error("expected EOSE message, got nothing")
		}

		cancel()
	})
}

func TestGetCommittedPowTarget(t *testing.T) {
	tests := []struct {
		name string
		tags []Tag
		want int
	}{
		{
			name: "valid nonce tag",
			tags: []Tag{{"nonce", "776797", "20"}},
			want: 20,
		},
		{
			name: "no nonce tag",
			tags: []Tag{{"e", "event_id"}},
			want: -1,
		},
		{
			name: "nonce tag without target",
			tags: []Tag{{"nonce", "776797"}},
			want: -1,
		},
		{
			name: "nonce tag with invalid target",
			tags: []Tag{{"nonce", "776797", "invalid"}},
			want: -1,
		},
		{
			name: "empty tags",
			tags: []Tag{},
			want: -1,
		},
		{
			name: "multiple tags with nonce",
			tags: []Tag{{"e", "id"}, {"nonce", "123", "30"}, {"p", "pubkey"}},
			want: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &Event{Tags: tt.tags}
			got := getCommittedPowTarget(event)
			if got != tt.want {
				t.Errorf("getCommittedPowTarget() = %d, want %d", got, tt.want)
			}
		})
	}
}
