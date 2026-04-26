package mocrelay

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageHandler_Event_Store(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send an EVENT
		event := makeEvent("event-1", "pubkey01", 1, 100)
		recv <- &ClientMsg{
			Type:  MsgTypeEvent,
			Event: event,
		}

		synctest.Wait()

		// Should receive OK
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeOK, msg.Type)
			assert.Equal(t, "event-1", msg.EventID)
			assert.True(t, msg.Accepted)
		default:
			t.Fatal("expected OK message")
		}

		// Event should be stored
		assert.Equal(t, 1, storage.len())
	})
}

func TestStorageHandler_Event_Duplicate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send same event twice
		event := makeEvent("event-1", "pubkey01", 1, 100)
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		synctest.Wait()
		<-send // First OK

		recv <- &ClientMsg{Type: MsgTypeEvent, Event: event}
		synctest.Wait()

		// Second should also be OK (duplicate is not an error)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeOK, msg.Type)
			assert.True(t, msg.Accepted)
			assert.Contains(t, msg.Message, "duplicate")
		default:
			t.Fatal("expected OK message")
		}

		// Still only 1 event
		assert.Equal(t, 1, storage.len())
	})
}

func TestStorageHandler_Req_Empty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ with empty storage
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive EOSE only (no events)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeEOSE, msg.Type)
			assert.Equal(t, "sub1", msg.SubscriptionID)
		default:
			t.Fatal("expected EOSE message")
		}
	})
}

func TestStorageHandler_Req_WithEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive 3 EVENTs + EOSE
		var events []*ServerMsg
		for i := range 4 {
			select {
			case msg := <-send:
				events = append(events, msg)
			default:
				t.Fatalf("expected message %d", i)
			}
		}

		// First 3 should be EVENT, last should be EOSE
		require.Len(t, events, 4)
		assert.Equal(t, MsgTypeEvent, events[0].Type)
		assert.Equal(t, MsgTypeEvent, events[1].Type)
		assert.Equal(t, MsgTypeEvent, events[2].Type)
		assert.Equal(t, MsgTypeEOSE, events[3].Type)

		// Events should be sorted by created_at DESC
		assert.Equal(t, "event-3", events[0].Event.ID)
		assert.Equal(t, "event-2", events[1].Event.ID)
		assert.Equal(t, "event-1", events[2].Event.ID)
	})
}

func TestStorageHandler_Req_WithLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send REQ with limit
		limit := int64(2)
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Limit: &limit}},
		}

		synctest.Wait()

		// Should receive 2 EVENTs + EOSE
		var events []*ServerMsg
		for i := range 3 {
			select {
			case msg := <-send:
				events = append(events, msg)
			default:
				t.Fatalf("expected message %d", i)
			}
		}

		require.Len(t, events, 3)
		assert.Equal(t, MsgTypeEvent, events[0].Type)
		assert.Equal(t, MsgTypeEvent, events[1].Type)
		assert.Equal(t, MsgTypeEOSE, events[2].Type)

		// Should be the 2 newest
		assert.Equal(t, "event-3", events[0].Event.ID)
		assert.Equal(t, "event-2", events[1].Event.ID)
	})
}

func TestStorageHandler_Close_NoOp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := NewInMemoryStorage()
		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send CLOSE - should be a no-op
		recv <- &ClientMsg{
			Type:           MsgTypeClose,
			SubscriptionID: "sub1",
		}

		synctest.Wait()

		// Should not receive any message
		select {
		case msg := <-send:
			t.Fatalf("unexpected message: %v", msg)
		default:
			// Expected: no message
		}
	})
}

func TestStorageHandler_Count(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 1, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send COUNT
		recv <- &ClientMsg{
			Type:           MsgTypeCount,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		synctest.Wait()

		// Should receive COUNT with 3
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeCount, msg.Type)
			assert.Equal(t, "sub1", msg.SubscriptionID)
			assert.Equal(t, uint64(3), msg.Count)
		default:
			t.Fatal("expected COUNT message")
		}
	})
}

func TestStorageHandler_Count_WithFilter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		storage := NewInMemoryStorage()

		// Pre-populate storage with different kinds
		storage.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))
		storage.Store(ctx, makeEvent("event-2", "pubkey01", 2, 200))
		storage.Store(ctx, makeEvent("event-3", "pubkey01", 1, 300))

		handler := NewStorageHandler(storage, nil)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		send := make(chan *ServerMsg, 10)
		recv := make(chan *ClientMsg, 10)

		go handler.ServeNostr(ctx, send, recv)

		// Send COUNT with kind filter
		recv <- &ClientMsg{
			Type:           MsgTypeCount,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{Kinds: []int64{1}}},
		}

		synctest.Wait()

		// Should receive COUNT with 2 (only kind 1)
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeCount, msg.Type)
			assert.Equal(t, uint64(2), msg.Count)
		default:
			t.Fatal("expected COUNT message")
		}
	})
}

// delayedQueryStorage wraps a Storage and sleeps for delay before returning
// from Query — useful to force the slow-query logging path under synctest
// (time.Sleep advances synctest's virtual clock).
type delayedQueryStorage struct {
	inner Storage
	delay time.Duration
}

func (s *delayedQueryStorage) Store(ctx context.Context, event *Event) (bool, error) {
	return s.inner.Store(ctx, event)
}

func (s *delayedQueryStorage) Query(
	ctx context.Context,
	filters []*ReqFilter,
) (iter.Seq[*Event], func() error, func() error) {
	time.Sleep(s.delay)
	return s.inner.Query(ctx, filters)
}

// runSlowQueryServeNostr is a shared fixture for the slow-query tests: it
// wires up a handler, an in-memory storage wrapped in a delay shim, a slog
// text handler into buf, and drives a single ClientMsg through ServeNostr.
// synctest time is advanced by test-goroutine time.Sleep (past advance) so
// the delayed Query path actually resolves before assertions run.
func runSlowQueryServeNostr(
	t *testing.T,
	msg *ClientMsg,
	opts *StorageHandlerOptions,
	delay time.Duration,
	advance time.Duration,
) string {
	t.Helper()
	ctx := context.Background()
	base := NewInMemoryStorage()
	base.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))

	storage := &delayedQueryStorage{inner: base, delay: delay}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	handler := NewStorageHandler(storage, opts)

	ctx = ContextWithLogger(ctx, logger)
	ctx, cancel := context.WithCancel(ctx)

	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	errCh := make(chan error, 1)
	go func() { errCh <- handler.ServeNostr(ctx, send, recv) }()

	recv <- msg

	// time.Sleep (not just synctest.Wait) is required to advance the fake
	// clock — see merge_handler_test.go for the same pattern.
	time.Sleep(advance)
	synctest.Wait()

	cancel()
	synctest.Wait()
	<-errCh
	return buf.String()
}

func TestStorageHandler_SlowQuery_ReqLogsOnThresholdExceeded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := runSlowQueryServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{SlowQueryThreshold: 100 * time.Millisecond},
			500*time.Millisecond,
			600*time.Millisecond,
		)

		for _, want := range []string{
			`level=WARN`,
			`msg="storage: slow REQ"`,
			`subscription_id=sub1`,
			`events_sent=1`,
			`completed=true`,
			`total_ms=`,
			`scan_ms=`,
			`send_wait_ms=`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\nfull output: %s", want, out)
			}
		}
	})
}

func TestStorageHandler_SlowQuery_ReqNoLogBelowThreshold(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Delay is below the default 1s threshold: must not log.
		out := runSlowQueryServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			nil, // default 1s
			500*time.Millisecond,
			600*time.Millisecond,
		)

		if strings.Contains(out, "slow REQ") {
			t.Errorf("slow REQ should not be logged below default 1s threshold:\n%s", out)
		}
	})
}

func TestStorageHandler_SlowQuery_Disabled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Large delay that would trip any positive threshold, but a
		// negative threshold disables slow-query logging entirely.
		out := runSlowQueryServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{SlowQueryThreshold: -1},
			5*time.Second,
			6*time.Second,
		)

		if strings.Contains(out, "slow REQ") || strings.Contains(out, "slow COUNT") {
			t.Errorf("slow-query logging should be disabled with negative threshold:\n%s", out)
		}
	})
}

func TestStorageHandler_SlowQuery_CountLogsOnThresholdExceeded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := runSlowQueryServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeCount,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{SlowQueryThreshold: 100 * time.Millisecond},
			500*time.Millisecond,
			600*time.Millisecond,
		)

		for _, want := range []string{
			`level=WARN`,
			`msg="storage: slow COUNT"`,
			`subscription_id=sub1`,
			`count=1`,
			`total_ms=`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\nfull output: %s", want, out)
			}
		}
	})
}

// runQueryTimeoutServeNostr mirrors runSlowQueryServeNostr but also drains the
// send channel after cancel, so tests can assert which terminal ServerMsg
// (CLOSED on timeout, EOSE / COUNT on normal completion) was emitted.
func runQueryTimeoutServeNostr(
	t *testing.T,
	msg *ClientMsg,
	opts *StorageHandlerOptions,
	delay time.Duration,
	advance time.Duration,
) (string, []*ServerMsg) {
	t.Helper()
	ctx := context.Background()
	base := NewInMemoryStorage()
	base.Store(ctx, makeEvent("event-1", "pubkey01", 1, 100))

	storage := &delayedQueryStorage{inner: base, delay: delay}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	handler := NewStorageHandler(storage, opts)

	ctx = ContextWithLogger(ctx, logger)
	ctx, cancel := context.WithCancel(ctx)

	send := make(chan *ServerMsg, 10)
	recv := make(chan *ClientMsg, 10)

	errCh := make(chan error, 1)
	go func() { errCh <- handler.ServeNostr(ctx, send, recv) }()

	recv <- msg

	time.Sleep(advance)
	synctest.Wait()

	cancel()
	synctest.Wait()
	<-errCh

	var msgs []*ServerMsg
drain:
	for {
		select {
		case m := <-send:
			msgs = append(msgs, m)
		default:
			break drain
		}
	}
	return buf.String(), msgs
}

func TestStorageHandler_QueryTimeout_ReqAbortsWithClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out, msgs := runQueryTimeoutServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{
				SlowQueryThreshold: 50 * time.Millisecond,
				QueryTimeout:       100 * time.Millisecond,
			},
			500*time.Millisecond,
			600*time.Millisecond,
		)

		// Must have sent exactly one CLOSED, no EVENT or EOSE.
		require.Len(t, msgs, 1)
		assert.Equal(t, MsgTypeClosed, msgs[0].Type)
		assert.Equal(t, "sub1", msgs[0].SubscriptionID)
		assert.Equal(t, "error: query timeout", msgs[0].Message)

		// Slow-query log should fire with completed=false (aborted).
		for _, want := range []string{
			`msg="storage: slow REQ"`,
			`completed=false`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\nfull output: %s", want, out)
			}
		}
	})
}

func TestStorageHandler_QueryTimeout_CountAbortsWithClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, msgs := runQueryTimeoutServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeCount,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{QueryTimeout: 100 * time.Millisecond},
			500*time.Millisecond,
			600*time.Millisecond,
		)

		require.Len(t, msgs, 1)
		assert.Equal(t, MsgTypeClosed, msgs[0].Type)
		assert.Equal(t, "sub1", msgs[0].SubscriptionID)
		assert.Equal(t, "error: query timeout", msgs[0].Message)
	})
}

func TestStorageHandler_QueryTimeout_ReqWithinTimeoutNormalEose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, msgs := runQueryTimeoutServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{QueryTimeout: 5 * time.Second},
			50*time.Millisecond,
			100*time.Millisecond,
		)

		// Normal completion: 1 EVENT + EOSE, no CLOSED.
		require.Len(t, msgs, 2)
		assert.Equal(t, MsgTypeEvent, msgs[0].Type)
		assert.Equal(t, MsgTypeEOSE, msgs[1].Type)
	})
}

func TestStorageHandler_QueryTimeout_CountWithinTimeoutNormalCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, msgs := runQueryTimeoutServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeCount,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{QueryTimeout: 5 * time.Second},
			50*time.Millisecond,
			100*time.Millisecond,
		)

		require.Len(t, msgs, 1)
		assert.Equal(t, MsgTypeCount, msgs[0].Type)
		assert.Equal(t, uint64(1), msgs[0].Count)
	})
}

func TestStorageHandler_QueryTimeout_ZeroDisables(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Large delay that would trip any positive timeout, but zero
		// QueryTimeout means disabled.
		_, msgs := runQueryTimeoutServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{
				SlowQueryThreshold: -1, // silence the slow-query log
				QueryTimeout:       0,  // explicitly disabled
			},
			500*time.Millisecond,
			600*time.Millisecond,
		)

		// Normal completion despite long delay: EVENT + EOSE.
		require.Len(t, msgs, 2)
		assert.Equal(t, MsgTypeEvent, msgs[0].Type)
		assert.Equal(t, MsgTypeEOSE, msgs[1].Type)
	})
}

func TestStorageHandler_QueryTimeout_NegativeDisables(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, msgs := runQueryTimeoutServeNostr(t,
			&ClientMsg{
				Type:           MsgTypeReq,
				SubscriptionID: "sub1",
				Filters:        []*ReqFilter{{}},
			},
			&StorageHandlerOptions{
				SlowQueryThreshold: -1,
				QueryTimeout:       -1 * time.Second,
			},
			500*time.Millisecond,
			600*time.Millisecond,
		)

		require.Len(t, msgs, 2)
		assert.Equal(t, MsgTypeEvent, msgs[0].Type)
		assert.Equal(t, MsgTypeEOSE, msgs[1].Type)
	})
}

// TestStorageHandler_QueryTimeout_ReqAbortsOnStalledConsumer exercises the
// "live but slow client" class of stall — the motivating case that
// QueryTimeout was introduced for. Query returns fast but the receiver
// stops draining send, so the handler's yield blocks on send<-msg.
// QueryTimeout must fire at the *send boundary* too: a ctx.WithTimeout
// scoped to Query alone lets the stall continue indefinitely once yield
// is blocked (scan is already done; the scan loop's ctx.Err check never
// re-runs).
func TestStorageHandler_QueryTimeout_ReqAbortsOnStalledConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		base := NewInMemoryStorage()
		// Populate more than one event so there is a second yield that
		// actually stalls on send<-msg (the first fits in the
		// receiver-side handoff that the test performs below).
		for i := range 5 {
			base.Store(ctx, makeEvent(
				fmt.Sprintf("event-%02d", i),
				"pubkey01",
				1,
				int64(100+i),
			))
		}

		handler := NewStorageHandler(base, &StorageHandlerOptions{
			QueryTimeout:       100 * time.Millisecond,
			SlowQueryThreshold: 50 * time.Millisecond,
		})

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		send := make(chan *ServerMsg) // unbuffered: 2nd yield stalls
		recv := make(chan *ClientMsg, 1)
		errCh := make(chan error, 1)
		go func() { errCh <- handler.ServeNostr(ctx, send, recv) }()

		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}

		// Receive exactly one EVENT, then stop draining so the next yield
		// blocks. The handler is now parked on send<-msg for event #2.
		first := <-send
		require.Equal(t, MsgTypeEvent, first.Type)

		// Advance the fake clock well past QueryTimeout. A correctly
		// scoped QueryTimeout must unblock the stalled send and fall
		// through to the CLOSED abort path.
		time.Sleep(300 * time.Millisecond)
		synctest.Wait()

		// The next message the handler sends must be CLOSED, not another
		// EVENT — time has moved past the abort deadline.
		select {
		case msg := <-send:
			assert.Equal(t, MsgTypeClosed, msg.Type)
			assert.Equal(t, "sub1", msg.SubscriptionID)
			assert.Equal(t, queryTimeoutCLOSEDReason, msg.Message)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("handler did not send CLOSED after QueryTimeout — send-side stall is not honoring the timeout")
		}

		cancel()
		synctest.Wait()
		<-errCh
	})
}

// TestStorageHandler_DrainsRecvWhileYieldBlocks asserts that storageHandler
// keeps draining its recv channel while a REQ response is parked on
// send<-msg. The motivating concern is the "deadlock spring" shape recorded
// in CLAUDE.md: when a Handler implements [Handler] directly (as
// storageHandler does, since #76) and consumes its iter.Seq response on the
// same goroutine that reads recv, a stalled send pauses recv drain — which
// in turn wedges upstream broadcasters such as MergeHandler on their own
// child-recv buffers (childRecvs cap 10) until BroadcastTimeout retires the
// child.
//
// simpleHandler addressed this in #78 by spawning a dedicated recv-drain
// goroutine fed by an internal cap-10 queue. The CLAUDE.md rule "Handlers
// that implement Handler directly and consume iter.Seq responses must
// follow the same shape" applies to storageHandler too: this test exercises
// that contract.
func TestStorageHandler_DrainsRecvWhileYieldBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := NewInMemoryStorage()
		// Two events: the first event is delivered into the unbuffered send
		// below, the second yield then blocks on send<-msg. That blocked
		// send is the condition we want to verify recv drain survives.
		for i := range 2 {
			base.Store(t.Context(), makeEvent(
				fmt.Sprintf("event-%02d", i),
				"pubkey01",
				1,
				int64(100+i),
			))
		}

		// QueryTimeout left at zero (disabled) on purpose: the test must
		// observe recv drain survival on the indefinite-stall path that
		// QueryTimeout would otherwise mask.
		handler := NewStorageHandler(base, nil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		send := make(chan *ServerMsg) // unbuffered: 2nd yield stalls
		recv := make(chan *ClientMsg) // unbuffered: drain pauses are observable

		done := make(chan error, 1)
		go func() { done <- handler.ServeNostr(ctx, send, recv) }()

		// Push a REQ. The handler will yield event-00, drain it via the
		// receive on `send` below, then yield event-01 — at which point
		// the main loop is parked on send<-msg with `send` having no
		// further reader.
		recv <- &ClientMsg{
			Type:           MsgTypeReq,
			SubscriptionID: "sub1",
			Filters:        []*ReqFilter{{}},
		}
		first := <-send
		require.Equal(t, MsgTypeEvent, first.Type)
		synctest.Wait()

		// Push a second client message. With concurrent recv drain (the
		// desired behaviour), this push completes even though the main
		// loop is still parked on the first response's stalled send. We
		// don't need the handler to *act on* the second message — just to
		// keep flowing recv. Push from a helper goroutine so we can
		// distinguish "push succeeded" from "push is still blocked"; make
		// it ctx-aware so the final cancel cleanup reliably reaps it.
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
			t.Log("storageHandler drained recv while yield was blocked (ideal)")
		default:
			drainStalled = true
		}

		cancel()
		synctest.Wait()
		<-done

		if drainStalled {
			t.Fatal("storageHandler stopped draining recv while a REQ " +
				"response was blocked on send — this is the 'deadlock " +
				"spring' shape that wedges MergeHandler childRecvs at " +
				"production scale (CLAUDE.md iter.Seq recv-drain rule).")
		}
	})
}
