package nostr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseClientMsg(t *testing.T) {
	type Expect struct {
		MsgType ClientMsgType
		Err     error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ng: invalid utf8",
			Input: []byte{'[', '"', 0xf0, 0x28, 0x8c, 0xbc, '"', ']'},
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ng: empty string",
			Input: []byte(""),
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ng: not a client message",
			Input: []byte(`["INVALID","value"]`),
			Expect: Expect{
				MsgType: ClientMsgTypeUnknown,
				Err:     ErrInvalidClientMsg,
			},
		},
		{
			Name:  "ok: client close message",
			Input: []byte(`["CLOSE","sub_id"]`),
			Expect: Expect{
				MsgType: ClientMsgTypeClose,
				Err:     nil,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				MsgType: ClientMsgTypeClose,
				Err:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientMsg(tt.Input)
			if tt.Expect.Err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.MsgType, msg.MsgType())
		})
	}
}

func TestParseClientCloseMsg(t *testing.T) {
	type Expect struct {
		SubscriptionID string
		Raw            []byte
		Err            error
	}

	tests := []struct {
		Name   string
		Input  []byte
		Expect Expect
	}{
		{
			Name:  "ok: client close message",
			Input: []byte(`["CLOSE","sub_id"]`),
			Expect: Expect{
				SubscriptionID: "sub_id",
				Raw:            []byte(`["CLOSE","sub_id"]`),
				Err:            nil,
			},
		},
		{
			Name:  "ok: client close message with some spaces",
			Input: []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
			Expect: Expect{
				SubscriptionID: "sub_id",
				Raw:            []byte(`[` + "\n" + `  "CLOSE",` + "\n" + `  "sub_id"` + "\n" + `]`),
				Err:            nil,
			},
		},
		{
			Name:  "ng: client close message invalid type",
			Input: []byte(`["CLOSE",3000]`),
			Expect: Expect{
				Err: ErrInvalidClientCloseMsg,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			msg, err := ParseClientCloseMsg(tt.Input)
			if tt.Expect.Err != nil {
				assert.ErrorIs(t, err, tt.Expect.Err)
				return
			}
			if msg == nil {
				t.Errorf("expected non-nil msg but got nil")
				return
			}
			assert.Equal(t, tt.Expect.SubscriptionID, msg.SubscriptionID)
			assert.Equal(t, tt.Expect.Raw, msg.Raw())
		})
	}
}
