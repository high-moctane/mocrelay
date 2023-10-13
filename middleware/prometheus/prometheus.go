package prometheus

import (
	"net/http"
	"strconv"

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

	reqCounter *reqCounter
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

		reqCounter: reqCounter,
	}

	reg.MustRegister(m.connectionCount)
	reg.MustRegister(m.recvMsgTotal)
	reg.MustRegister(m.recvEventTotal)
	reg.MustRegister(m.sendMsgTotal)
	reg.MustRegister(m.reqTotal)

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
	switch msg.(type) {
	case *mocrelay.ServerEOSEMsg:
		m.sendMsgTotal.WithLabelValues("EOSE").Inc()

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

type reqCounter struct {
	// chan map[reqID]chan map[subID]exist
	c chan map[string]chan map[string]bool
}

func newReqCounter() *reqCounter {
	c := &reqCounter{
		c: make(chan map[string]chan map[string]bool, 1),
	}
	c.c <- make(map[string]chan map[string]bool)
	return c
}

func (c *reqCounter) AddReqID(reqID string) {
	cc := make(chan map[string]bool, 1)
	cc <- make(map[string]bool)
	m := <-c.c
	m[reqID] = cc
	c.c <- m
}

func (c *reqCounter) DeleteReqID(reqID string) {
	m := <-c.c
	delete(m, reqID)
	c.c <- m
}

func (c *reqCounter) AddSubID(reqID, subID string) {
	m := <-c.c
	cc := m[reqID]
	c.c <- m

	mm := <-cc
	mm[subID] = true
	cc <- mm
}

func (c *reqCounter) DeleteSubID(reqID, subID string) {
	m := <-c.c
	cc := m[reqID]
	c.c <- m

	mm := <-cc
	delete(mm, subID)
	cc <- mm
}

func (c *reqCounter) Count() int {
	ret := 0

	m := <-c.c
	var ccs []chan map[string]bool
	for _, cc := range m {
		ccs = append(ccs, cc)
	}
	c.c <- m

	for _, cc := range ccs {
		mm := <-cc
		ret += len(mm)
		cc <- mm
	}

	return ret
}
