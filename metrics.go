//go:build goexperiment.jsonv2

package mocrelay

import (
	"github.com/prometheus/client_golang/prometheus"
)

// RelayMetrics holds Prometheus metrics for Relay.
type RelayMetrics struct {
	ConnectionsCurrent prometheus.Gauge
	ConnectionsTotal   prometheus.Counter
	MessagesReceived   *prometheus.CounterVec
	MessagesSent       *prometheus.CounterVec
	EventsReceived     *prometheus.CounterVec
}

// NewRelayMetrics creates and registers Relay metrics with the given registry.
func NewRelayMetrics(reg prometheus.Registerer) *RelayMetrics {
	m := &RelayMetrics{
		ConnectionsCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mocrelay_connections_current",
			Help: "Current number of WebSocket connections",
		}),
		ConnectionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_connections_total",
			Help: "Total number of WebSocket connections",
		}),
		MessagesReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_messages_received_total",
			Help: "Total number of messages received from clients",
		}, []string{"type"}),
		MessagesSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_messages_sent_total",
			Help: "Total number of messages sent to clients",
		}, []string{"type"}),
		EventsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_events_received_total",
			Help: "Total number of events received from clients",
		}, []string{"kind"}),
	}

	reg.MustRegister(
		m.ConnectionsCurrent,
		m.ConnectionsTotal,
		m.MessagesReceived,
		m.MessagesSent,
		m.EventsReceived,
	)

	return m
}

// StorageMetrics holds Prometheus metrics for Storage.
type StorageMetrics struct {
	EventsStored  *prometheus.CounterVec
	StoreDuration prometheus.Histogram
	QueryDuration prometheus.Histogram
}

// NewStorageMetrics creates and registers Storage metrics with the given registry.
func NewStorageMetrics(reg prometheus.Registerer) *StorageMetrics {
	m := &StorageMetrics{
		EventsStored: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_events_stored_total",
			Help: "Total number of event store attempts",
		}, []string{"kind", "stored"}),
		StoreDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mocrelay_store_duration_seconds",
			Help:    "Time spent storing events",
			Buckets: prometheus.DefBuckets,
		}),
		QueryDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mocrelay_query_duration_seconds",
			Help:    "Time spent querying events",
			Buckets: prometheus.DefBuckets,
		}),
	}

	reg.MustRegister(
		m.EventsStored,
		m.StoreDuration,
		m.QueryDuration,
	)

	return m
}
