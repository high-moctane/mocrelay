package mocrelay

import (
	"context"

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
		}, []string{"kind", "type"}),
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

// RouterMetrics holds Prometheus metrics for Router.
type RouterMetrics struct {
	MessagesDropped prometheus.Counter
}

// NewRouterMetrics creates and registers Router metrics with the given registry.
func NewRouterMetrics(reg prometheus.Registerer) *RouterMetrics {
	m := &RouterMetrics{
		MessagesDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_router_messages_dropped_total",
			Help: "Total number of messages dropped due to full send channel",
		}),
	}

	reg.MustRegister(
		m.MessagesDropped,
	)

	return m
}

// AuthMetrics holds Prometheus metrics for AuthMiddleware.
//
// Rejection metrics are not part of this struct. They are unified across all
// middleware under [RejectionMetrics] and are wired automatically via
// [logRejection] — Auth contributes to the shared counter with label
// middleware="auth".
type AuthMetrics struct {
	AuthTotal                       *prometheus.CounterVec
	AuthenticatedConnectionsCurrent prometheus.Gauge
}

// NewAuthMetrics creates and registers AuthMiddleware metrics with the given registry.
func NewAuthMetrics(reg prometheus.Registerer) *AuthMetrics {
	m := &AuthMetrics{
		AuthTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_auth_total",
			Help: "Total number of authentication attempts",
		}, []string{"result"}),
		AuthenticatedConnectionsCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mocrelay_auth_authenticated_connections_current",
			Help: "Current number of authenticated WebSocket connections",
		}),
	}

	reg.MustRegister(
		m.AuthTotal,
		m.AuthenticatedConnectionsCurrent,
	)

	return m
}

// RejectionMetrics holds the unified rejection counter shared across every
// middleware in mocrelay. Each middleware reports its rejections through
// [logRejection], which increments this counter with labels
// {middleware, reason} — the same pair that appears in the structured log
// entry, so logs and metrics are cross-indexable.
//
// The (middleware, reason) pair is a functional relationship (each middleware
// emits a fixed, small enum of reasons), so the label set is bounded and
// does not form a cardinality cross-product — analogous to the (kind, type)
// two-axis label on [RelayMetrics.EventsReceived].
//
// RejectionMetrics is injected into the request context by [Relay] (via
// [RelayOptions.RejectionMetrics]); middleware code never holds a reference
// and never needs a nil check — [logRejection] handles both.
type RejectionMetrics struct {
	Total *prometheus.CounterVec
}

// NewRejectionMetrics creates and registers the unified rejection counter
// with the given registry.
func NewRejectionMetrics(reg prometheus.Registerer) *RejectionMetrics {
	m := &RejectionMetrics{
		Total: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_rejections_total",
			Help: "Total number of client messages rejected by middleware, " +
				"labeled by the rejecting middleware and the reason. " +
				"The reason label aligns with the structured log field " +
				"emitted by logRejection, so logs and metrics share a key.",
		}, []string{"middleware", "reason"}),
	}

	reg.MustRegister(m.Total)

	return m
}

type rejectionMetricsKey struct{}

// ContextWithRejectionMetrics returns a new context carrying the given
// rejection metrics. [Relay] installs this at connection start so middleware
// downstream can report rejections via [logRejection] without holding a
// direct reference.
func ContextWithRejectionMetrics(ctx context.Context, m *RejectionMetrics) context.Context {
	return context.WithValue(ctx, rejectionMetricsKey{}, m)
}

// RejectionMetricsFromContext returns the rejection metrics from the context,
// or nil if none was set. Callers (in practice only [logRejection]) must be
// nil-safe.
func RejectionMetricsFromContext(ctx context.Context) *RejectionMetrics {
	if m, ok := ctx.Value(rejectionMetricsKey{}).(*RejectionMetrics); ok {
		return m
	}
	return nil
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
		}, []string{"kind", "type", "stored"}),
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
