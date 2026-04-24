package mocrelay

import (
	"context"
	"errors"
	"iter"
	"testing"
	"testing/synctest"
	"time"
)

// mockSimpleHandlerBase is a test helper for SimpleHandlerBase.
type mockSimpleHandlerBase struct {
	onStartFunc   func(context.Context) (context.Context, *ServerMsg, error)
	onEndFunc     func(context.Context) (*ServerMsg, error)
	handleMsgFunc func(context.Context, *ClientMsg) (iter.Seq[*ServerMsg], error)

	onStartCalled bool
	onEndCalled   bool
	msgsReceived  []*ClientMsg
}

func (m *mockSimpleHandlerBase) OnStart(ctx context.Context) (context.Context, *ServerMsg, error) {
	m.onStartCalled = true
	if m.onStartFunc != nil {
		return m.onStartFunc(ctx)
	}
	return ctx, nil, nil
}

func (m *mockSimpleHandlerBase) OnEnd(ctx context.Context) (*ServerMsg, error) {
	m.onEndCalled = true
	if m.onEndFunc != nil {
		return m.onEndFunc(ctx)
	}
	return nil, nil
}

func (m *mockSimpleHandlerBase) HandleMsg(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
	m.msgsReceived = append(m.msgsReceived, msg)
	if m.handleMsgFunc != nil {
		return m.handleMsgFunc(ctx, msg)
	}
	return nil, nil
}

func TestSimpleHandler_NormalFlow(t *testing.T) {
	// Test normal message flow: receive message, send response
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
			return func(yield func(*ServerMsg) bool) {
				yield(NewServerOKMsg("test-id", true, ""))
			}, nil
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
		onStartFunc: func(ctx context.Context) (context.Context, *ServerMsg, error) {
			return ctx, nil, expectedErr
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
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
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

func TestSimpleHandler_NilResponseIterator(t *testing.T) {
	// Test that nil response iterator is handled correctly (no responses sent)
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
			return nil, nil // nil iterator
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

func TestSimpleHandler_EmptyResponseIterator(t *testing.T) {
	// Test that empty response iterator (yields nothing) is handled correctly
	mock := &mockSimpleHandlerBase{
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
			return func(yield func(*ServerMsg) bool) {
				// yields nothing
			}, nil
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
		handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
			responseCount++
			return func(yield func(*ServerMsg) bool) {
				if !yield(NewServerOKMsg("id1", true, "")) {
					return
				}
				yield(NewServerOKMsg("id2", true, ""))
			}, nil
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

// TestSimpleHandler_DrainsRecvWhileYieldBlocks asserts that simpleHandler keeps
// draining its recv channel while HandleMsg's response iterator is blocked on
// sending. Without concurrent recv drain, a slow downstream (unread `send`)
// stalls the handler itself and any upstream broadcaster (e.g. MergeHandler)
// wedges on a full child-recv buffer — the "deadlock spring" shape observed at
// production scale (see problem D, diary 2026-04-24).
//
// Today this test is EXPECTED TO FAIL on main: simpleHandler consumes
// HandleMsg's iter.Seq synchronously on the same goroutine that reads recv, so
// a blocked `send <- msg` pauses recv drain until the receiver unblocks.
//
// The structural fix (later commits in this branch) is to decouple recv drain
// from response forwarding — e.g. a dedicated recv-drain goroutine feeding an
// internal queue. Once that lands, this test should PASS as-written.
func TestSimpleHandler_DrainsRecvWhileYieldBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		handledMsgs := make(chan *ClientMsg, 10)
		mock := &mockSimpleHandlerBase{
			handleMsgFunc: func(ctx context.Context, msg *ClientMsg) (iter.Seq[*ServerMsg], error) {
				handledMsgs <- msg
				return func(yield func(*ServerMsg) bool) {
					yield(NewServerOKMsg("dummy", true, ""))
				}, nil
			},
		}

		handler := NewSimpleHandler(mock)
		send := make(chan *ServerMsg) // unbuffered, intentionally never drained
		recv := make(chan *ClientMsg) // unbuffered, so drain pauses are observable

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- handler.ServeNostr(ctx, send, recv)
		}()

		// First message: handled, yields one response, then blocks on
		// `send <- msg` because `send` has no reader. simpleHandler's
		// goroutine is parked on that send from here on.
		recv <- &ClientMsg{Type: MsgTypeEvent}
		<-handledMsgs
		synctest.Wait()

		// Second message: with concurrent recv drain (the desired behaviour),
		// the push below completes even though the first yield's send is
		// still blocked. We do not observe HandleMsg being invoked a second
		// time — the main loop is still parked on the first response's
		// send — we only observe that recv itself keeps flowing. Push from
		// a helper goroutine so that the test can distinguish "push
		// succeeded" from "push is still blocked"; make it ctx-aware so the
		// final cancel cleanup reliably reaps the helper.
		delivered := make(chan struct{})
		go func() {
			select {
			case recv <- &ClientMsg{Type: MsgTypeEvent}:
			case <-ctx.Done():
			}
			close(delivered)
		}()
		synctest.Wait()

		var drainStalled bool
		select {
		case <-delivered:
			t.Log("simpleHandler drained recv while yield was blocked (ideal)")
		default:
			drainStalled = true
		}

		cancel()
		synctest.Wait()
		<-done

		if drainStalled {
			t.Fatal("simpleHandler stopped draining recv while HandleMsg's " +
				"yield was blocked on send — this is the 'deadlock spring' " +
				"shape that wedges MergeHandler childRecvs at production scale.")
		}
	})
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
