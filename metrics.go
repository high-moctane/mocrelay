package main

import (
	"context"
	"fmt"
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
		Name: "mocrelay_active_websocket_count",
		Help: "active websocket",
	},
	[]string{"host", "conn_id"},
))

type PromActiveWebsocket prometheus.GaugeVec

func (pc *PromActiveWebsocket) WithLabelValues(ctx context.Context) prometheus.Gauge {
	return (*prometheus.GaugeVec)(pc).WithLabelValues(GetCtxRealIP(ctx), GetCtxConnID(ctx))
}

var promWSSendCounter *PromWSSendCounter = (*PromWSSendCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_msg_send_count",
		Help: "message send counter",
	},
	[]string{"host", "conn_id", "msg_type"},
))

type PromWSSendCounter prometheus.CounterVec

func (pc *PromWSSendCounter) WithLabelValues(ctx context.Context, msg ServerMsg) prometheus.Counter {
	var msgType string

	switch msg.(type) {
	case *ServerEOSEMsg:
		msgType = "EOSE"
	case *ServerEventMsg:
		msgType = "EVENT"
	case *ServerNoticeMsg:
		msgType = "NOTICE"
	}

	return (*prometheus.CounterVec)(pc).WithLabelValues(GetCtxRealIP(ctx), GetCtxConnID(ctx), msgType)
}

var promWSRecvCounter *PromWSRecvCounter = (*PromWSRecvCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_msg_recv_count",
		Help: "message recv counter",
	},
	[]string{"host", "conn_id", "msg_type"},
))

type PromWSRecvCounter prometheus.CounterVec

func (pc *PromWSRecvCounter) WithLabelValues(ctx context.Context, msg ClientMsgJSON) prometheus.Counter {
	var msgType string

	switch msg.(type) {
	case *ClientReqMsgJSON:
		msgType = "REQ"
	case *ClientCloseMsgJSON:
		msgType = "CLOSE"
	case *ClientEventMsgJSON:
		msgType = "EVENT"
	}

	return (*prometheus.CounterVec)(pc).WithLabelValues(GetCtxRealIP(ctx), GetCtxConnID(ctx), msgType)
}

var promEventCounter *PromEventCounter = (*PromEventCounter)(promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mocrelay_event_recv_count",
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
		Name: "mocrelay_router_publish_second",
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
		Name: "mocrelay_receive_failed_count",
		Help: "receive failed events",
	},
	[]string{"conn_id", "sub_id"},
))

type PromReceiveFail prometheus.CounterVec

func (pc *PromReceiveFail) WithLabelValues(connID, subID string) prometheus.Counter {
	return (*prometheus.CounterVec)(pc).WithLabelValues(connID, subID)
}

var promCacheQueryTime prometheus.Histogram = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name: "mocrelay_cache_query_second",
		Help: "consumption of query req",
	},
)
