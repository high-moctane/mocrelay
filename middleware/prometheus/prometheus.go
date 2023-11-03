package prometheus

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/high-moctane/mocrelay"
	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusMiddleware mocrelay.SimpleMiddleware

func NewPrometheusMiddleware(reg prometheus.Registerer) PrometheusMiddleware {
	m := newSimplePrometheusMiddleware(reg)
	return PrometheusMiddleware(mocrelay.NewSimpleMiddleware(m))
}

type simplePrometheusMiddleware struct {
	connectionCount prometheus.Gauge
	recvMsgTotal    *prometheus.CounterVec
	recvEventTotal  *prometheus.CounterVec
	sendMsgTotal    *prometheus.CounterVec
	reqTotal        prometheus.GaugeFunc
	reqResponseTime prometheus.Summary

	reqCounter *reqCounter

	// map[reqID + subID]startTime
	reqStartTimeMu sync.Mutex
	reqStartTime   map[string]map[string]time.Time
}

func newSimplePrometheusMiddleware(reg prometheus.Registerer) *simplePrometheusMiddleware {
	reqCounter := newReqCounter()

	m := &simplePrometheusMiddleware{
		connectionCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mocrelay_connection_count",
			Help: "Current websocket connection count.",
		}),
		recvMsgTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mocrelay_recv_msg_total",
				Help: "Number of received client messages.",
			},
			[]string{"type"},
		),
		recvEventTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mocrelay_recv_event_total",
				Help: "Number of received client messages.",
			},
			[]string{"kind"},
		),
		sendMsgTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mocrelay_send_msg_total",
				Help: "Number of sent server messages.",
			},
			[]string{"type"},
		),
		reqTotal: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Name: "mocrelay_req_count",
				Help: "Current req count.",
			},
			func() float64 { return float64(reqCounter.Count()) },
		),
		reqResponseTime: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Name: "mocrelay_req_response_seconds",
				Help: "Req to EOSE transaction time",
			},
		),

		reqCounter:   reqCounter,
		reqStartTime: make(map[string]map[string]time.Time),
	}

	reg.MustRegister(m.connectionCount)
	reg.MustRegister(m.recvMsgTotal)
	reg.MustRegister(m.recvEventTotal)
	reg.MustRegister(m.sendMsgTotal)
	reg.MustRegister(m.reqTotal)
	reg.MustRegister(m.reqResponseTime)

	return m
}

func (m *simplePrometheusMiddleware) HandleStart(r *http.Request) (*http.Request, error) {
	m.connectionCount.Inc()

	reqID := mocrelay.GetRequestID(r.Context())
	m.reqCounter.AddReqID(reqID)

	return r, nil
}

func (m *simplePrometheusMiddleware) HandleStop(r *http.Request) error {
	m.connectionCount.Dec()

	reqID := mocrelay.GetRequestID(r.Context())
	m.reqCounter.DeleteReqID(reqID)
	m.deleteReqTimer(reqID)

	return nil
}

func (m *simplePrometheusMiddleware) HandleClientMsg(
	r *http.Request,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {
	switch msg := msg.(type) {
	case *mocrelay.ClientUnknownMsg:
		m.recvMsgTotal.WithLabelValues("UNKNOWN").Inc()

	case *mocrelay.ClientEventMsg:
		m.recvMsgTotal.WithLabelValues("EVENT").Inc()
		k := strconv.FormatInt(msg.Event.Kind, 10)
		m.recvEventTotal.WithLabelValues(k).Inc()

	case *mocrelay.ClientReqMsg:
		m.recvMsgTotal.WithLabelValues("REQ").Inc()
		reqID := mocrelay.GetRequestID(r.Context())
		m.reqCounter.AddSubID(reqID, msg.SubscriptionID)
		m.startReqTimer(reqID, msg.SubscriptionID)

	case *mocrelay.ClientCloseMsg:
		m.recvMsgTotal.WithLabelValues("CLOSE").Inc()
		reqID := mocrelay.GetRequestID(r.Context())
		m.reqCounter.DeleteSubID(reqID, msg.SubscriptionID)

	case *mocrelay.ClientAuthMsg:
		m.recvMsgTotal.WithLabelValues("AUTH").Inc()

	case *mocrelay.ClientCountMsg:
		m.recvMsgTotal.WithLabelValues("COUNT").Inc()

	default:
		m.recvMsgTotal.WithLabelValues("UNDEFINED").Inc()
	}

	res := make(chan mocrelay.ClientMsg, 1)
	defer close(res)
	res <- msg

	return res, nil, nil
}

func (m *simplePrometheusMiddleware) HandleServerMsg(
	r *http.Request,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	switch msg := msg.(type) {
	case *mocrelay.ServerEOSEMsg:
		m.sendMsgTotal.WithLabelValues("EOSE").Inc()
		reqID := mocrelay.GetRequestID(r.Context())
		if dur := m.stopReqTimer(reqID, msg.SubscriptionID); dur != 0 {
			m.reqResponseTime.Observe(float64(dur) / float64(time.Second))
		}

	case *mocrelay.ServerEventMsg:
		m.sendMsgTotal.WithLabelValues("EVENT").Inc()

	case *mocrelay.ServerNoticeMsg:
		m.sendMsgTotal.WithLabelValues("NOTICE").Inc()

	case *mocrelay.ServerOKMsg:
		m.sendMsgTotal.WithLabelValues("OK").Inc()

	case *mocrelay.ServerAuthMsg:
		m.sendMsgTotal.WithLabelValues("AUTH").Inc()

	case *mocrelay.ServerCountMsg:
		m.sendMsgTotal.WithLabelValues("COUNT").Inc()

	default:
		m.sendMsgTotal.WithLabelValues("UNDEFINED").Inc()
	}

	res := make(chan mocrelay.ServerMsg, 1)
	defer close(res)
	res <- msg

	return res, nil
}

func (m *simplePrometheusMiddleware) startReqTimer(reqID, subID string) {
	m.reqStartTimeMu.Lock()
	defer m.reqStartTimeMu.Unlock()

	mm, ok := m.reqStartTime[reqID]
	if !ok {
		mm = make(map[string]time.Time)
		m.reqStartTime[reqID] = mm
	}
	mm[subID] = time.Now()
}

func (m *simplePrometheusMiddleware) stopReqTimer(reqID, subID string) time.Duration {
	m.reqStartTimeMu.Lock()
	defer m.reqStartTimeMu.Unlock()

	start, ok := m.reqStartTime[reqID][subID]
	if !ok {
		return 0
	}
	defer delete(m.reqStartTime[reqID], subID)

	return time.Since(start)
}

func (m *simplePrometheusMiddleware) deleteReqTimer(reqID string) {
	m.reqStartTimeMu.Lock()
	defer m.reqStartTimeMu.Unlock()

	delete(m.reqStartTime, reqID)
}

type reqCounter struct {
	// map[reqID]map[subID]exist
	c *safeMap[string, *safeMap[string, bool]]
}

func newReqCounter() *reqCounter {
	return &reqCounter{
		c: newSafeMap[string, *safeMap[string, bool]](),
	}
}

func (c *reqCounter) AddReqID(reqID string) {
	c.c.Add(reqID, newSafeMap[string, bool]())
}

func (c *reqCounter) DeleteReqID(reqID string) {
	c.c.Delete(reqID)
}

func (c *reqCounter) AddSubID(reqID, subID string) {
	m, ok := c.c.TryGet(reqID)
	if !ok {
		return
	}
	m.Add(subID, true)
}

func (c *reqCounter) DeleteSubID(reqID, subID string) {
	m, ok := c.c.TryGet(reqID)
	if !ok {
		return
	}

	m.Delete(subID)
}

func (c *reqCounter) Count() int {
	ret := 0

	c.c.Loop(func(_ string, m *safeMap[string, bool]) {
		ret += m.Len()
	})

	return ret
}

type safeMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func newSafeMap[K comparable, V any]() *safeMap[K, V] {
	return &safeMap[K, V]{m: make(map[K]V)}
}

func (m *safeMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.m)
}

func (m *safeMap[K, V]) Get(k K) V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.m[k]
}

func (m *safeMap[K, V]) TryGet(k K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.m[k]
	return v, ok
}

func (m *safeMap[K, V]) Add(k K, v V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[k] = v
}

func (m *safeMap[K, V]) Delete(k K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, k)
}

func (m *safeMap[K, V]) Loop(f func(k K, v V)) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for key, val := range m.m {
		f(key, val)
	}
}
