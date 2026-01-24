//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockSimpleHandlerBase is a test helper for SimpleHandlerBase.
type mockSimpleHandlerBase struct {
	onStartFunc   func(context.Context) (context.Context, error)
	onEndFunc     func(context.Context) error
	handleMsgFunc func(context.Context, *ClientMsg) (<-chan *ServerMsg, error)

	onStartCalled bool
	onEndCalled   bool
	msgsReceived  []*ClientMsg
}

func (m *mockSimpleHandlerBase) OnStart(ctx context.Context) (context.Context, error) {
	m.onStartCalled = true
	if m.onStartFunc != nil {
		return m.onStartFunc(ctx)
	}
	return ctx, nil
}

func (m *mockSimpleHandlerBase) OnEnd(ctx context.Context) error {
	m.onEndCalled = true
	if m.onEndFunc != nil {
		return m.onEndFunc(ctx)
	}
	return nil
}

func (m *mockSimpleHandlerBase) HandleMsg(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
	m.msgsReceived = append(m.msgsReceived, msg)
	if m.handleMsgFunc != nil {
		return m.handleMsgFunc(ctx, msg)
	}
	return nil, nil
}

func TestSimpleHandler_NormalFlow(t *testing.T) {
	// Test normal message flow: receive message, send response
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
			ch := make(chan *ServerMsg, 1)
			ch <- NewServerOKMsg("test-id", true, "")
			close(ch)
			return ch, nil
		},
	}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	// Send a message and close recv
	recv <- &ClientMsg{Type: MsgTypeEvent}
	close(recv)

	ctx := context.Background()
	err := handler.ServeNostr(ctx, send, recv)

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !mock.onStartCalled {
		t.Error("OnStart was not called")
	}
	if !mock.onEndCalled {
		t.Error("OnEnd was not called")
	}
	if len(mock.msgsReceived) != 1 {
		t.Errorf("expected 1 message received, got %d", len(mock.msgsReceived))
	}
	if len(send) != 1 {
		t.Errorf("expected 1 message sent, got %d", len(send))
	}
}

func TestSimpleHandler_OnStartError(t *testing.T) {
	// Test that OnStart error terminates the handler
	expectedErr := errors.New("onstart error")
	mock := &mockSimpleHandlerBase{
		onStartFunc: func(ctx context.Context) (context.Context, error) {
			return ctx, expectedErr
		},
	}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	ctx := context.Background()
	err := handler.ServeNostr(ctx, send, recv)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	if !mock.onStartCalled {
		t.Error("OnStart was not called")
	}
	// OnEnd should NOT be called if OnStart fails
	if mock.onEndCalled {
		t.Error("OnEnd should not be called when OnStart fails")
	}
}

func TestSimpleHandler_HandleMsgError(t *testing.T) {
	// Test that HandleMsg error terminates the handler but OnEnd is still called
	expectedErr := errors.New("handlemsg error")
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
			return nil, expectedErr
		},
	}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	recv <- &ClientMsg{Type: MsgTypeEvent}

	ctx := context.Background()
	err := handler.ServeNostr(ctx, send, recv)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	if !mock.onEndCalled {
		t.Error("OnEnd should be called even when HandleMsg fails")
	}
}

func TestSimpleHandler_RecvChannelClosed(t *testing.T) {
	// Test that closing recv channel causes normal termination
	mock := &mockSimpleHandlerBase{}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg)

	// Close recv immediately
	close(recv)

	ctx := context.Background()
	err := handler.ServeNostr(ctx, send, recv)

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !mock.onEndCalled {
		t.Error("OnEnd should be called")
	}
}

func TestSimpleHandler_ContextCanceled(t *testing.T) {
	// Test that context cancellation terminates the handler
	mock := &mockSimpleHandlerBase{}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg) // unbuffered, will block

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- handler.ServeNostr(ctx, send, recv)
	}()

	// Cancel context
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Error("handler did not terminate after context cancellation")
	}

	if !mock.onEndCalled {
		t.Error("OnEnd should be called")
	}
}

func TestSimpleHandler_NilResponseChannel(t *testing.T) {
	// Test that nil response channel is handled correctly (no responses sent)
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
			return nil, nil // nil channel
		},
	}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	recv <- &ClientMsg{Type: MsgTypeEvent}
	close(recv)

	ctx := context.Background()
	err := handler.ServeNostr(ctx, send, recv)

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(send) != 0 {
		t.Errorf("expected 0 messages sent, got %d", len(send))
	}
}

func TestSimpleHandler_EmptyResponseChannel(t *testing.T) {
	// Test that empty response channel (immediately closed) is handled correctly
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
			ch := make(chan *ServerMsg)
			close(ch) // immediately closed
			return ch, nil
		},
	}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	recv <- &ClientMsg{Type: MsgTypeEvent}
	close(recv)

	ctx := context.Background()
	err := handler.ServeNostr(ctx, send, recv)

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(send) != 0 {
		t.Errorf("expected 0 messages sent, got %d", len(send))
	}
}

func TestSimpleHandler_MultipleMessages(t *testing.T) {
	// Test handling multiple messages
	responseCount := 0
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (<-chan *ServerMsg, error) {
			responseCount++
			ch := make(chan *ServerMsg, 2)
			ch <- NewServerOKMsg("id1", true, "")
			ch <- NewServerOKMsg("id2", true, "")
			close(ch)
			return ch, nil
		},
	}

	handler := NewSimpleHandler(mock)
	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	recv <- &ClientMsg{Type: MsgTypeEvent}
	recv <- &ClientMsg{Type: MsgTypeEvent}
	recv <- &ClientMsg{Type: MsgTypeEvent}
	close(recv)

	ctx := context.Background()
	err := handler.ServeNostr(ctx, send, recv)

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(mock.msgsReceived) != 3 {
		t.Errorf("expected 3 messages received, got %d", len(mock.msgsReceived))
	}
	if len(send) != 6 { // 3 messages * 2 responses each
		t.Errorf("expected 6 messages sent, got %d", len(send))
	}
}

func TestHandlerFunc(t *testing.T) {
	// Test that HandlerFunc works as Handler
	called := false
	var f HandlerFunc = func(ctx context.Context, send chan<- *ServerMsg, recv <-chan *ClientMsg) error {
		called = true
		return nil
	}

	send := make(chan *ServerMsg)
	recv := make(chan *ClientMsg)
	close(recv)

	err := f.ServeNostr(context.Background(), send, recv)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !called {
		t.Error("HandlerFunc was not called")
	}
}
