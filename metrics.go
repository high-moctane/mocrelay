package main

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func StartMetricsServer() error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(Cfg.MonitorAddr, nil)
}

var promActiveWebsocket = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "mocrelay_active_websocket_count",
	Help: "active websocket",
})

var promWSSendCounter *PromWSSendCounter = (*PromWSSendCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_msg_send_count",
		Help: "message send counter",
	},
	[]string{"msg_type"},
))

type PromWSSendCounter prometheus.CounterVec

func (pc *PromWSSendCounter) WithLabelValues(msg ServerMsg) prometheus.Counter {
	var msgType string

	switch msg.(type) {
	case *ServerEOSEMsg:
		msgType = "EOSE"
	case *ServerEventMsg:
		msgType = "EVENT"
	case *ServerNoticeMsg:
		msgType = "NOTICE"
	case *ServerOKMsg:
		msgType = "OK"
	}

	return (*prometheus.CounterVec)(pc).WithLabelValues(msgType)
}

var promWSRecvCounter *PromWSRecvCounter = (*PromWSRecvCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_msg_recv_count",
		Help: "message recv counter",
	},
	[]string{"msg_type"},
))

type PromWSRecvCounter prometheus.CounterVec

func (pc *PromWSRecvCounter) WithLabelValues(msg ClientMsgJSON) prometheus.Counter {
	var msgType string

	switch msg.(type) {
	case *ClientReqMsgJSON:
		msgType = "REQ"
	case *ClientCloseMsgJSON:
		msgType = "CLOSE"
	case *ClientEventMsgJSON:
		msgType = "EVENT"
	}

	return (*prometheus.CounterVec)(pc).WithLabelValues(msgType)
}

var promEventCounter *PromEventCounter = (*PromEventCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_event_recv_count",
		Help: "received events",
	},
	[]string{"kind"},
))

type PromEventCounter prometheus.CounterVec

func (pc *PromEventCounter) WithLabelValues(event *EventJSON) prometheus.Counter {
	return (*prometheus.CounterVec)(pc).WithLabelValues(fmt.Sprintf("%05d", event.Kind))
}

var promTryEnqueueServerMsgFail = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "mocrelay_try_enqueue_server_msg_failed_count",
		Help: "try_enqueue_server_msg failed events",
	},
)

var promCacheQueryTime prometheus.Histogram = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name: "mocrelay_cache_query_second",
		Help: "consumption of query req",
	},
)
