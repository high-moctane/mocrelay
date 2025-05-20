package mocrelay

import (
	"context"
	"testing"
)

type mockNeoSimpleHandlerBase struct {
	neoServeNostrOnStartFunc func(context.Context) (context.Context, error)
	neoServeNostrClientMsg   func(context.Context, ClientMsg) (<-chan ServerMsg, error)
	neoServeNostrOnEnd       func(context.Context, error) error
}

func (m *mockNeoSimpleHandlerBase) NeoServeNostrOnStart(ctx context.Context) (context.Context, error) {
	return m.neoServeNostrOnStartFunc(ctx)
}

func (m *mockNeoSimpleHandlerBase) NeoServeNostrClientMsg(ctx context.Context, msg ClientMsg) (<-chan ServerMsg, error) {
	return m.neoServeNostrClientMsg(ctx, msg)
}

func (m *mockNeoSimpleHandlerBase) NeoServeNostrOnEnd(ctx context.Context, err error) error {
	return m.neoServeNostrOnEnd(ctx, err)
}

func TestNeoSimpleHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		base := &mockNeoSimpleHandlerBase{
			neoServeNostrOnStartFunc: func(ctx context.Context) (context.Context, error) {
				return ctx, nil
			},
			neoServeNostrClientMsg: func(ctx context.Context, msg ClientMsg) (<-chan ServerMsg, error) {
				return newClosedBufCh[ServerMsg](&ServerClosedMsg{}), nil
			},
			neoServeNostrOnEnd: func(ctx context.Context, err error) error {
				return nil
			},
		}

		send := make(chan ServerMsg, 10)
		recv := newClosedBufCh[ClientMsg](&ClientReqMsg{})

		ctx := t.Context()

		handler := NewNeoSimpleHandler(base)

		err := handler.NeoServeNostr(ctx, send, recv)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		select {
		case msg := <-send:
			if _, ok := msg.(*ServerClosedMsg); !ok {
				t.Errorf("expected message to be of type ServerClosedMsg, got %T", msg)
			}
		default:
			t.Error("expected message to be sent, but none was sent")
		}

		select {
		case msg := <-send:
			t.Errorf("got extra message: %v", msg)

		default:
		}
	})
}
