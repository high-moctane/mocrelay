package mocrelay

import (
	"context"
	"errors"
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

	t.Run("context passing", func(t *testing.T) {
		ctxKey := "testKey"

		want := "testValue"
		var gotServeNostrClientMsg, gotOnEnd string

		base := &mockNeoSimpleHandlerBase{
			neoServeNostrOnStartFunc: func(ctx context.Context) (context.Context, error) {
				return context.WithValue(ctx, ctxKey, "testValue"), nil
			},
			neoServeNostrClientMsg: func(ctx context.Context, msg ClientMsg) (<-chan ServerMsg, error) {
				var ok bool
				gotServeNostrClientMsg, ok = ctx.Value(ctxKey).(string)
				if !ok {
					t.Fatal("expected context to contain key")
				}
				return newClosedBufCh[ServerMsg](&ServerClosedMsg{}), nil
			},
			neoServeNostrOnEnd: func(ctx context.Context, err error) error {
				var ok bool
				gotOnEnd, ok = ctx.Value(ctxKey).(string)
				if !ok {
					t.Fatal("expected context to contain key")
				}
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

		if gotServeNostrClientMsg != want {
			t.Errorf("expected context value to be %v, gotServeNostrClientMsg %v", want, gotServeNostrClientMsg)
		}
		if gotOnEnd != want {
			t.Errorf("expected context value to be %v, gotOnEnd %v", want, gotOnEnd)
		}
	})

	t.Run("error on NeoServeNostrOnStart", func(t *testing.T) {
		onStartErr := errors.New("on start error")
		onEndErr := errors.New("on end error")

		base := &mockNeoSimpleHandlerBase{
			neoServeNostrOnStartFunc: func(ctx context.Context) (context.Context, error) {
				return ctx, onStartErr
			},
			neoServeNostrClientMsg: func(ctx context.Context, msg ClientMsg) (<-chan ServerMsg, error) {
				panic("should not be called")
			},
			neoServeNostrOnEnd: func(ctx context.Context, err error) error {
				if err != onStartErr {
					t.Errorf("expected error to be %v, got %v", onStartErr, err)
				}
				return onEndErr
			},
		}

		send := make(chan ServerMsg, 10)
		recv := newClosedBufCh[ClientMsg](&ClientReqMsg{})

		ctx := t.Context()

		handler := NewNeoSimpleHandler(base)

		err := handler.NeoServeNostr(ctx, send, recv)
		if err != onEndErr {
			t.Errorf("expected error to be %v, got %v", onEndErr, err)
		}

		select {
		case msg := <-send:
			t.Errorf("got extra message: %v", msg)

		default:
		}
	})

	t.Run("error on NeoServeNostrClientMsg", func(t *testing.T) {
		serveErr := errors.New("on serve error")
		onEndErr := errors.New("on end error")

		base := &mockNeoSimpleHandlerBase{
			neoServeNostrOnStartFunc: func(ctx context.Context) (context.Context, error) {
				return ctx, nil
			},
			neoServeNostrClientMsg: func(ctx context.Context, msg ClientMsg) (<-chan ServerMsg, error) {
				return nil, serveErr
			},
			neoServeNostrOnEnd: func(ctx context.Context, err error) error {
				if err != serveErr {
					t.Errorf("expected error to be %v, got %v", serveErr, err)
				}
				return onEndErr
			},
		}

		send := make(chan ServerMsg, 10)
		recv := newClosedBufCh[ClientMsg](&ClientReqMsg{})

		ctx := t.Context()

		handler := NewNeoSimpleHandler(base)

		err := handler.NeoServeNostr(ctx, send, recv)
		if err != onEndErr {
			t.Errorf("expected error to be %v, got %v", onEndErr, err)
		}

		select {
		case msg := <-send:
			t.Errorf("got extra message: %v", msg)

		default:
		}
	})
}
