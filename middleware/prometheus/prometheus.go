package prometheus

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/high-moctane/mocrelay"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/google/uuid"
)

type PrometheusMiddleware mocrelay.Middleware

func NewPrometheusMiddleware(reg prometheus.Registerer) PrometheusMiddleware {
	m := newSimplePrometheusMiddlewareBase(reg)
	return PrometheusMiddleware(mocrelay.NewSimpleMiddleware(m))
}

type requestIDKey struct{}

var requestIDKeyInstance = requestIDKey{}

func setRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, requestIDKeyInstance, reqID)
}

func getRequestID(ctx context.Context) string {
	reqID, _ := ctx.Value(requestIDKeyInstance).(string)
	return reqID
}

type simplePrometheusMiddlewareBase struct {
	connectionCounter      *connectionCounter
	recvMsgCounter         *recvMsgCounter
	recvEventCounter       *recvEventCounter
	sendMsgCounter         *sendMsgCounter
	reqCounter             *reqCounter
	reqResponseTimeCounter *reqResponseTimeCounter
}

func newSimplePrometheusMiddlewareBase(reg prometheus.Registerer) *simplePrometheusMiddlewareBase {
	connectionCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "mocrelay_connection_count",
		Help: "Current websocket connection count.",
	})
	recvMsgTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mocrelay_recv_msg_total",
			Help: "Number of received client messages.",
		},
		[]string{"type"},
	)
	recvEventTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mocrelay_recv_event_total",
			Help: "Number of received client messages.",
		},
		[]string{"kind"},
	)
	sendMsgTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mocrelay_send_msg_total",
			Help: "Number of sent server messages.",
		},
		[]string{"type"},
	)
	reqTotal := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mocrelay_req_count",
			Help: "Current req count.",
		},
	)
	reqResponseTime := prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "mocrelay_req_response_seconds",
			Help: "Req to EOSE transaction time",
		},
	)

	reg.MustRegister(connectionCount)
	reg.MustRegister(recvMsgTotal)
	reg.MustRegister(recvEventTotal)
	reg.MustRegister(sendMsgTotal)
	reg.MustRegister(reqTotal)
	reg.MustRegister(reqResponseTime)

	return &simplePrometheusMiddlewareBase{
		connectionCounter:      newConnectionCounter(connectionCount),
		recvMsgCounter:         newRecvMsgCounter(recvMsgTotal),
		recvEventCounter:       newRecvEventCounter(recvEventTotal),
		sendMsgCounter:         newSendMsgCounter(sendMsgTotal),
		reqCounter:             newReqCounterCounter(reqTotal),
		reqResponseTimeCounter: newReqResponseTimeCounter(reqResponseTime),
	}
}

func (m *simplePrometheusMiddlewareBase) ServeNostrStart(
	ctx context.Context,
) (context.Context, error) {
	reqID := uuid.NewString()
	ctx = setRequestID(ctx, reqID)

	m.connectionCounter.ServeNostrStart(ctx)
	m.recvMsgCounter.ServeNostrStart(ctx)
	m.recvEventCounter.ServeNostrStart(ctx)
	m.sendMsgCounter.ServeNostrStart(ctx)
	m.reqCounter.ServeNostrStart(ctx)
	m.reqResponseTimeCounter.ServeNostrStart(ctx)

	return ctx, nil
}

func (m *simplePrometheusMiddlewareBase) ServeNostrEnd(ctx context.Context) error {
	m.connectionCounter.ServeNostrEnd(ctx)
	m.recvMsgCounter.ServeNostrEnd(ctx)
	m.recvEventCounter.ServeNostrEnd(ctx)
	m.sendMsgCounter.ServeNostrEnd(ctx)
	m.reqCounter.ServeNostrEnd(ctx)
	m.reqResponseTimeCounter.ServeNostrEnd(ctx)

	return nil
}

func (m *simplePrometheusMiddlewareBase) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {
	m.connectionCounter.ServeNostrClientMsg(ctx, msg)
	m.recvMsgCounter.ServeNostrClientMsg(ctx, msg)
	m.recvEventCounter.ServeNostrClientMsg(ctx, msg)
	m.sendMsgCounter.ServeNostrClientMsg(ctx, msg)
	m.reqCounter.ServeNostrClientMsg(ctx, msg)
	m.reqResponseTimeCounter.ServeNostrClientMsg(ctx, msg)

	ret := make(chan mocrelay.ClientMsg, 1)
	defer close(ret)
	ret <- msg

	return ret, nil, nil
}

func (m *simplePrometheusMiddlewareBase) ServeNostrServerMsg(
	ctx context.Context,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	m.connectionCounter.ServeNostrServerMsg(ctx, msg)
	m.recvMsgCounter.ServeNostrServerMsg(ctx, msg)
	m.recvEventCounter.ServeNostrServerMsg(ctx, msg)
	m.sendMsgCounter.ServeNostrServerMsg(ctx, msg)
	m.reqCounter.ServeNostrServerMsg(ctx, msg)
	m.reqResponseTimeCounter.ServeNostrServerMsg(ctx, msg)

	res := make(chan mocrelay.ServerMsg, 1)
	defer close(res)
	res <- msg

	return res, nil
}

type connectionCounter struct {
	c prometheus.Gauge
}

func newConnectionCounter(c prometheus.Gauge) *connectionCounter {
	return &connectionCounter{c: c}
}

func (c *connectionCounter) ServeNostrStart(ctx context.Context) (context.Context, error) {
	c.c.Inc()
	return ctx, nil
}

func (c *connectionCounter) ServeNostrEnd(ctx context.Context) error {
	c.c.Dec()
	return nil
}

func (c *connectionCounter) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {
	return nil, nil, nil
}

func (c *connectionCounter) ServeNostrServerMsg(
	ctx context.Context,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	return nil, nil
}

type recvMsgCounter struct {
	c *prometheus.CounterVec
}

func newRecvMsgCounter(c *prometheus.CounterVec) *recvMsgCounter {
	return &recvMsgCounter{c: c}
}

func (c *recvMsgCounter) ServeNostrStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (c *recvMsgCounter) ServeNostrEnd(ctx context.Context) error {
	return nil
}

func (c *recvMsgCounter) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {
	switch msg.(type) {
	case *mocrelay.ClientEventMsg:
		c.c.WithLabelValues("EVENT").Inc()

	case *mocrelay.ClientReqMsg:
		c.c.WithLabelValues("REQ").Inc()

	case *mocrelay.ClientCloseMsg:
		c.c.WithLabelValues("CLOSE").Inc()

	case *mocrelay.ClientAuthMsg:
		c.c.WithLabelValues("AUTH").Inc()

	case *mocrelay.ClientCountMsg:
		c.c.WithLabelValues("COUNT").Inc()

	default:
		c.c.WithLabelValues("UNDEFINED").Inc()
	}

	return nil, nil, nil
}

func (c *recvMsgCounter) ServeNostrServerMsg(
	ctx context.Context,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	return nil, nil
}

type recvEventCounter struct {
	c *prometheus.CounterVec
}

func newRecvEventCounter(c *prometheus.CounterVec) *recvEventCounter {
	return &recvEventCounter{c: c}
}

func (c *recvEventCounter) ServeNostrStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (c *recvEventCounter) ServeNostrEnd(ctx context.Context) error {
	return nil
}

func (c *recvEventCounter) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {
	if msg, ok := msg.(*mocrelay.ClientEventMsg); ok {
		k := strconv.FormatInt(msg.Event.Kind, 10)
		c.c.WithLabelValues(k).Inc()
	}

	return nil, nil, nil
}

func (c *recvEventCounter) ServeNostrServerMsg(
	ctx context.Context,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	return nil, nil
}

type sendMsgCounter struct {
	c *prometheus.CounterVec
}

func newSendMsgCounter(c *prometheus.CounterVec) *sendMsgCounter {
	return &sendMsgCounter{c: c}
}

func (c *sendMsgCounter) ServeNostrStart(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (c *sendMsgCounter) ServeNostrEnd(ctx context.Context) error {
	return nil
}

func (c *sendMsgCounter) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {
	return nil, nil, nil
}

func (c *sendMsgCounter) ServeNostrServerMsg(
	ctx context.Context,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	switch msg.(type) {
	case *mocrelay.ServerEOSEMsg:
		c.c.WithLabelValues("EOSE").Inc()

	case *mocrelay.ServerEventMsg:
		c.c.WithLabelValues("EVENT").Inc()

	case *mocrelay.ServerNoticeMsg:
		c.c.WithLabelValues("NOTICE").Inc()

	case *mocrelay.ServerOKMsg:
		c.c.WithLabelValues("OK").Inc()

	case *mocrelay.ServerAuthMsg:
		c.c.WithLabelValues("AUTH").Inc()

	case *mocrelay.ServerCountMsg:
		c.c.WithLabelValues("COUNT").Inc()

	case *mocrelay.ServerClosedMsg:
		c.c.WithLabelValues("CLOSED").Inc()

	default:
		c.c.WithLabelValues("UNDEFINED").Inc()
	}

	return nil, nil
}

type reqCounter struct {
	c prometheus.Gauge

	mu sync.Mutex
	// map[reqID]map[subID]bool
	m map[string]map[string]bool
}

func newReqCounterCounter(c prometheus.Gauge) *reqCounter {
	return &reqCounter{
		c: c,
		m: make(map[string]map[string]bool),
	}
}

func (c *reqCounter) ServeNostrStart(ctx context.Context) (context.Context, error) {
	reqID := getRequestID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.m[reqID] = make(map[string]bool)

	return ctx, nil
}

func (c *reqCounter) ServeNostrEnd(ctx context.Context) error {
	reqID := getRequestID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	cnt := len(c.m[reqID])
	c.c.Sub(float64(cnt))

	delete(c.m, reqID)

	return nil
}

func (c *reqCounter) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {

	switch msg := msg.(type) {
	case *mocrelay.ClientReqMsg:
		reqID := getRequestID(ctx)

		c.mu.Lock()
		defer c.mu.Unlock()

		if _, ok := c.m[reqID][msg.SubscriptionID]; !ok {
			c.m[reqID][msg.SubscriptionID] = true
			c.c.Inc()
		}

	case *mocrelay.ClientCloseMsg:
		reqID := getRequestID(ctx)

		c.mu.Lock()
		defer c.mu.Unlock()

		if _, ok := c.m[reqID][msg.SubscriptionID]; ok {
			delete(c.m[reqID], msg.SubscriptionID)
			c.c.Dec()
		}
	}

	return nil, nil, nil
}

func (c *reqCounter) ServeNostrServerMsg(
	ctx context.Context,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	switch msg := msg.(type) {
	case *mocrelay.ServerClosedMsg:
		reqID := getRequestID(ctx)

		c.mu.Lock()
		defer c.mu.Unlock()

		if _, ok := c.m[reqID][msg.SubscriptionID]; ok {
			delete(c.m[reqID], msg.SubscriptionID)
			c.c.Dec()
		}
	}

	return nil, nil
}

type reqResponseTimeCounter struct {
	c prometheus.Summary

	mu sync.Mutex
	// map[reqID]map[subID]startTime
	m map[string]map[string]time.Time
}

func newReqResponseTimeCounter(c prometheus.Summary) *reqResponseTimeCounter {
	return &reqResponseTimeCounter{
		c: c,
		m: make(map[string]map[string]time.Time),
	}
}

func (c *reqResponseTimeCounter) ServeNostrStart(ctx context.Context) (context.Context, error) {
	reqID := getRequestID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.m[reqID] = make(map[string]time.Time)

	return ctx, nil
}

func (c *reqResponseTimeCounter) ServeNostrEnd(ctx context.Context) error {
	reqID := getRequestID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.m, reqID)

	return nil
}

func (c *reqResponseTimeCounter) ServeNostrClientMsg(
	ctx context.Context,
	msg mocrelay.ClientMsg,
) (<-chan mocrelay.ClientMsg, <-chan mocrelay.ServerMsg, error) {
	if msg, ok := msg.(*mocrelay.ClientReqMsg); ok {
		reqID := getRequestID(ctx)

		c.mu.Lock()
		defer c.mu.Unlock()

		c.m[reqID][msg.SubscriptionID] = time.Now()
	}

	return nil, nil, nil
}

func (c *reqResponseTimeCounter) ServeNostrServerMsg(
	ctx context.Context,
	msg mocrelay.ServerMsg,
) (<-chan mocrelay.ServerMsg, error) {
	var subID string

	switch msg := msg.(type) {
	case *mocrelay.ServerEOSEMsg:
		subID = msg.SubscriptionID

	case *mocrelay.ServerClosedMsg:
		subID = msg.SubscriptionID

	default:
		return nil, nil
	}

	reqID := getRequestID(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	start, ok := c.m[reqID][subID]
	if !ok {
		return nil, nil
	}

	delete(c.m[reqID], subID)

	c.c.Observe(float64(time.Since(start)) / float64(time.Second))

	return nil, nil
}
