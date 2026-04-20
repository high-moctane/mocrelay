package mocrelay

import (
	"context"
	"errors"
	"iter"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

// errStorage is a Storage double that fails every Store and reports a Query
// error via errFn. Used to exercise MetricsStorage's StoreErrors / QueryErrors
// counters without standing up Pebble.
type errStorage struct {
	storeErr error
	queryErr error
}

func (s *errStorage) Store(ctx context.Context, event *Event) (bool, error) {
	return false, s.storeErr
}

func (s *errStorage) Query(ctx context.Context, filters []*ReqFilter) (iter.Seq[*Event], func() error, func() error) {
	empty := func(yield func(*Event) bool) {}
	errFn := func() error { return s.queryErr }
	closeFn := func() error { return nil }
	return empty, errFn, closeFn
}

// TestMetrics_RelayWSParseErrors drives the read path with malformed inputs
// and asserts that each rejection lands on its labeled WSParseErrors counter.
func TestMetrics_RelayWSParseErrors(t *testing.T) {
	reg := prometheus.NewRegistry()

	relay := NewRelay(NewNopHandler(), &RelayOptions{Registerer: reg})
	server := httptest.NewServer(relay)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// 1) binary frame → binary_message
	if err := conn.Write(ctx, websocket.MessageBinary, []byte{0x00, 0x01}); err != nil {
		t.Fatalf("binary write: %v", err)
	}
	// 2) invalid UTF-8 (as a text frame, the server rejects after read)
	if err := conn.Write(ctx, websocket.MessageText, []byte{0xff, 0xfe}); err != nil {
		t.Fatalf("invalid utf8 write: %v", err)
	}
	// 3) malformed JSON text → parse_error
	if err := conn.Write(ctx, websocket.MessageText, []byte("not-json")); err != nil {
		t.Fatalf("parse error write: %v", err)
	}

	// The relay replies with a NOTICE for each. Read them to synchronize.
	for range 3 {
		if _, _, err := conn.Read(ctx); err != nil {
			t.Fatalf("read notice: %v", err)
		}
	}

	got := func(reason string) float64 {
		return testutil.ToFloat64(relay.metrics.WSParseErrors.WithLabelValues(reason))
	}
	assert.Equal(t, float64(1), got("binary_message"))
	assert.Equal(t, float64(1), got("invalid_utf8"))
	assert.Equal(t, float64(1), got("parse_error"))
	assert.Equal(t, float64(0), got("verify_error"))
	assert.Equal(t, float64(0), got("invalid_signature"))
}

// TestMetrics_Router_SubscriptionsCurrent asserts that Subscribe/Unsubscribe/
// Unregister drive the gauge correctly, including the NIP-01 "replace without
// CLOSE" case.
func TestMetrics_Router_SubscriptionsCurrent(t *testing.T) {
	reg := prometheus.NewRegistry()
	router := NewRouter(&RouterOptions{Registerer: reg})

	gauge := func() float64 {
		return testutil.ToFloat64(router.metrics.SubscriptionsCurrent)
	}

	sendA := make(chan *ServerMsg, 1)
	sendB := make(chan *ServerMsg, 1)
	connA := router.Register(sendA)
	connB := router.Register(sendB)

	assert.Equal(t, float64(0), gauge(), "no subscriptions registered yet")

	router.Subscribe(connA, "sub1", []*ReqFilter{{}})
	router.Subscribe(connA, "sub2", []*ReqFilter{{}})
	router.Subscribe(connB, "sub1", []*ReqFilter{{}})
	assert.Equal(t, float64(3), gauge())

	// Replace sub1 on connA — NIP-01 allows re-using sub_id without CLOSE.
	// Must not drift the gauge upward.
	router.Subscribe(connA, "sub1", []*ReqFilter{{}})
	assert.Equal(t, float64(3), gauge(), "replacing an existing sub must not inflate the gauge")

	// Unsubscribe an existing subID: -1
	router.Unsubscribe(connA, "sub1")
	assert.Equal(t, float64(2), gauge())

	// Unsubscribe a non-existent subID: no-op
	router.Unsubscribe(connA, "nonexistent")
	assert.Equal(t, float64(2), gauge())

	// Unregister a connection removes all its remaining subscriptions.
	router.Unregister(connA) // had {sub2}
	assert.Equal(t, float64(1), gauge())
	router.Unregister(connB) // had {sub1}
	assert.Equal(t, float64(0), gauge())
}

// TestMetrics_MetricsStorage_StoreErrors asserts StoreErrors increments on a
// failing Store and stays at zero otherwise.
func TestMetrics_MetricsStorage_StoreErrors(t *testing.T) {
	reg := prometheus.NewRegistry()

	underlying := &errStorage{storeErr: errors.New("disk gone")}
	ms := NewMetricsStorage(underlying, reg)

	_, err := ms.Store(context.Background(), makeEvent("e1", "pubkey01", 1, 100))
	assert.Error(t, err)
	_, err = ms.Store(context.Background(), makeEvent("e2", "pubkey01", 1, 200))
	assert.Error(t, err)

	assert.Equal(t, float64(2), testutil.ToFloat64(ms.metrics.StoreErrors))
}

// TestMetrics_MetricsStorage_QueryErrors asserts QueryErrors increments exactly
// once per failed Query, even if the caller invokes errFn multiple times.
func TestMetrics_MetricsStorage_QueryErrors(t *testing.T) {
	reg := prometheus.NewRegistry()

	underlying := &errStorage{queryErr: errors.New("corrupt index")}
	ms := NewMetricsStorage(underlying, reg)

	_, errFn, closeFn := ms.Query(context.Background(), []*ReqFilter{{}})
	// Iteration yields nothing; errFn reports the error.
	assert.Error(t, errFn())
	// Second call must not double-count.
	assert.Error(t, errFn())
	assert.NoError(t, closeFn())

	// A second query: counter must tick by one more, not two.
	_, errFn2, closeFn2 := ms.Query(context.Background(), []*ReqFilter{{}})
	assert.Error(t, errFn2())
	assert.NoError(t, closeFn2())

	assert.Equal(t, float64(2), testutil.ToFloat64(ms.metrics.QueryErrors))
}

// TestMetrics_MetricsStorage_NoErrorsOnSuccess asserts the counters stay at
// zero on the happy path, guarding against accidental Inc in the no-error
// branch.
func TestMetrics_MetricsStorage_NoErrorsOnSuccess(t *testing.T) {
	reg := prometheus.NewRegistry()

	ms := NewMetricsStorage(NewInMemoryStorage(), reg)

	_, err := ms.Store(context.Background(), makeEvent("e1", "pubkey01", 1, 100))
	assert.NoError(t, err)

	_, errFn, closeFn := ms.Query(context.Background(), []*ReqFilter{{}})
	// Drain the iterator.
	// (no events registered beyond the stored one; we just consume whatever)
	// Then assert errFn is nil.
	events, _, _ := ms.Query(context.Background(), []*ReqFilter{{}})
	for range events {
	}
	assert.NoError(t, errFn())
	assert.NoError(t, closeFn())

	assert.Equal(t, float64(0), testutil.ToFloat64(ms.metrics.StoreErrors))
	assert.Equal(t, float64(0), testutil.ToFloat64(ms.metrics.QueryErrors))
}

// TestMetrics_MergeHandler_LostChildAndBroadcastTimeout asserts that a stuck
// child that trips BroadcastTimeout increments both BroadcastTimeoutsTotal
// and LostChildrenTotal. Mirrors TestMergeHandler_Broadcast_REQTimeoutAdvances
// but observes the metrics side-channel.
func TestMetrics_MergeHandler_LostChildAndBroadcastTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := prometheus.NewRegistry()
		metrics := newMergeHandlerMetrics(reg)

		stuck := &stuckHandler{}
		h := &mergeHandler{
			handlers:         []Handler{stuck},
			broadcastTimeout: 100 * time.Millisecond,
			childRecvBuffer:  1,
			metrics:          metrics,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 32)
		recv := make(chan *ClientMsg, 32)

		errCh := make(chan error, 1)
		go func() { errCh <- h.ServeNostr(ctx, send, recv) }()

		// First REQ fills the stuck child's recv buffer (cap=1) on the
		// fast path. Second REQ cannot fit and trips BroadcastTimeout.
		recv <- &ClientMsg{Type: MsgTypeReq, SubscriptionID: "sub1", Filters: []*ReqFilter{{}}}
		recv <- &ClientMsg{Type: MsgTypeReq, SubscriptionID: "sub2", Filters: []*ReqFilter{{}}}

		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		assert.Equal(t, float64(1), testutil.ToFloat64(metrics.BroadcastTimeoutsTotal),
			"one timeout on the second REQ broadcast")
		assert.Equal(t, float64(1), testutil.ToFloat64(metrics.LostChildrenTotal),
			"the stuck child is retired exactly once")

		cancel()
		synctest.Wait()
		<-errCh
	})
}

// TestMetrics_MergeHandler_EventDrops_Duplicate asserts that EventDrops is
// incremented with reason="duplicate" when both children emit the same event
// id for the same subscription.
func TestMetrics_MergeHandler_EventDrops_Duplicate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := prometheus.NewRegistry()

		storage1 := NewInMemoryStorage()
		storage2 := NewInMemoryStorage()
		ctx := context.Background()
		// Both storages hold an event with the same id — merge must dedup.
		storage1.Store(ctx, makeEvent("dup", "pubkey01", 1, 100))
		storage2.Store(ctx, makeEvent("dup", "pubkey01", 1, 100))

		mh := NewMergeHandler(
			[]Handler{NewStorageHandler(storage1, nil), NewStorageHandler(storage2, nil)},
			&MergeHandlerOptions{Registerer: reg},
		).(*mergeHandler)
		metrics := mh.metrics

		ctx2, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 32)
		recv := make(chan *ClientMsg, 32)
		go mh.ServeNostr(ctx2, send, recv)

		recv <- &ClientMsg{Type: MsgTypeReq, SubscriptionID: "s1", Filters: []*ReqFilter{{}}}
		synctest.Wait()

		assert.Equal(t, float64(1),
			testutil.ToFloat64(metrics.EventDrops.WithLabelValues("duplicate")),
			"exactly one dup dropped (second storage's copy)")
	})
}

// TestMetrics_MergeHandler_EventDrops_RecvBufFull asserts that EventDrops is
// incremented with reason="recv_buf_full" when a child's recv buffer is
// saturated and an EVENT broadcast is silently dropped.
func TestMetrics_MergeHandler_EventDrops_RecvBufFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := prometheus.NewRegistry()
		metrics := newMergeHandlerMetrics(reg)

		stuck := &stuckHandler{}
		h := &mergeHandler{
			handlers:         []Handler{stuck},
			broadcastTimeout: 100 * time.Millisecond,
			childRecvBuffer:  1,
			metrics:          metrics,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		send := make(chan *ServerMsg, 32)
		recv := make(chan *ClientMsg, 32)

		errCh := make(chan error, 1)
		go func() { errCh <- h.ServeNostr(ctx, send, recv) }()

		// First EVENT fills stuck's recv buffer (0/1 → 1/1) via fast path.
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: makeEvent("e1", "pubkey01", 1, 100)}
		// Second EVENT cannot fit; EVENT broadcast is best-effort and is
		// silently dropped, recording recv_buf_full.
		recv <- &ClientMsg{Type: MsgTypeEvent, Event: makeEvent("e2", "pubkey01", 1, 200)}

		synctest.Wait()

		assert.Equal(t, float64(1),
			testutil.ToFloat64(metrics.EventDrops.WithLabelValues("recv_buf_full")))

		cancel()
		synctest.Wait()
		<-errCh
	})
}
