package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func StartMetricsServer() error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(*PprofAddr, nil)
}

var promActiveWebsocket *PromActiveWebsocket = (*PromActiveWebsocket)(promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "mocrelay_active_websocket",
		Help: "active websocket",
	},
	[]string{"host", "conn_id"},
))

type PromActiveWebsocket prometheus.GaugeVec

func (pc *PromActiveWebsocket) WithLabelValues(addr, connID string) prometheus.Gauge {
	host, _, _ := net.SplitHostPort(addr)
	return (*prometheus.GaugeVec)(pc).WithLabelValues(host, connID)
}

var promWSSendCounter *PromWSSendCounter = (*PromWSSendCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_send",
		Help: "message send counter",
	},
	[]string{"host", "conn_id", "msg_type"},
))

type PromWSSendCounter prometheus.CounterVec

func (pc *PromWSSendCounter) WithLabelValues(addr, connID string, msg ServerMsg) prometheus.Counter {
	host, _, _ := net.SplitHostPort(addr)

	var msgType string

	switch msg.(type) {
	case *ServerEOSEMsg:
		msgType = "EOSE"
	case *ServerEventMsg:
		msgType = "EVENT"
	case *ServerNoticeMsg:
		msgType = "NOTICE"
	}

	return (*prometheus.CounterVec)(pc).WithLabelValues(host, connID, msgType)
}

var promWSRecvCounter *PromWSRecvCounter = (*PromWSRecvCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_recv",
		Help: "message recv counter",
	},
	[]string{"host", "conn_id", "msg_type"},
))

type PromWSRecvCounter prometheus.CounterVec

func (pc *PromWSRecvCounter) WithLabelValues(addr, connID string, msg ClientMsgJSON) prometheus.Counter {
	host, _, _ := net.SplitHostPort(addr)

	var msgType string

	switch msg.(type) {
	case *ClientReqMsgJSON:
		msgType = "REQ"
	case *ClientCloseMsgJSON:
		msgType = "CLOSE"
	case *ClientEventMsgJSON:
		msgType = "EVENT"
	}

	return (*prometheus.CounterVec)(pc).WithLabelValues(host, connID, msgType)
}

var promEventCounter *PromEventCounter = (*PromEventCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_receive_event",
		Help: "received events",
	},
	[]string{"pubkey", "kind"},
))

type PromEventCounter prometheus.CounterVec

func (pc *PromEventCounter) WithLabelValues(event *EventJSON) prometheus.Counter {
	return (*prometheus.CounterVec)(pc).WithLabelValues(event.Pubkey, fmt.Sprintf("%d", event.Kind))
}

var promRouterPublishTime *PromRouterPublishTime = (*PromRouterPublishTime)(promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name: "mocrelay_router_publish_time",
		Help: "consumption of publishing events",
	},
	[]string{"kind"},
))

type PromRouterPublishTime prometheus.HistogramVec

func (pc *PromRouterPublishTime) WithLabelValues(event *Event) prometheus.Observer {
	return (*prometheus.HistogramVec)(pc).WithLabelValues(fmt.Sprintf("%d", event.Kind))
}

var promReceiveFail *PromReceiveFail = (*PromReceiveFail)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_receive_failed",
		Help: "receive failed events",
	},
	[]string{"conn_id", "sub_id"},
))

type PromReceiveFail prometheus.CounterVec

func (pc *PromReceiveFail) WithLabelValues(connID, subID string) prometheus.Counter {
	return (*prometheus.CounterVec)(pc).WithLabelValues(connID, subID)
}

var promDBQueryTime prometheus.Histogram = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name: "mocrelay_db_query_time",
		Help: "consumption of query req",
	},
)
