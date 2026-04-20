package mocrelay

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
)

// relayMetrics holds Prometheus metrics for Relay.
type relayMetrics struct {
	ConnectionsCurrent prometheus.Gauge
	ConnectionsTotal   prometheus.Counter
	MessagesReceived   *prometheus.CounterVec
	MessagesSent       *prometheus.CounterVec
	EventsReceived     *prometheus.CounterVec

	// WSParseErrors counts client messages rejected before they reach the
	// handler pipeline (binary frames, invalid UTF-8, parse failures,
	// signature verification failures). Labeled by reason.
	WSParseErrors *prometheus.CounterVec

	// WSWriteErrors counts WebSocket write-path failures (marshal errors,
	// write timeouts, ping timeouts, other write errors). Labeled by reason.
	// A write-path failure typically terminates the connection; expect low
	// rates with sharp spikes during outages.
	WSWriteErrors *prometheus.CounterVec

	// WSWriteDuration observes the latency of each conn.Write call in the
	// writeLoop. Diagnoses slow clients / saturated send paths that can
	// cause write timeouts and ping-starvation.
	WSWriteDuration prometheus.Histogram
}

// newRelayMetrics creates and registers Relay metrics with reg.
// Returns nil if reg is nil so callers can treat a disabled registry as
// "no metrics collected" via a simple nil check at each instrument site.
func newRelayMetrics(reg prometheus.Registerer) *relayMetrics {
	if reg == nil {
		return nil
	}
	m := &relayMetrics{
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
		WSParseErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_ws_parse_errors_total",
			Help: "Total number of client messages rejected by the WebSocket " +
				"read path before reaching the handler pipeline, labeled by " +
				"reason (binary_message, invalid_utf8, parse_error, " +
				"verify_error, invalid_signature).",
		}, []string{"reason"}),
		WSWriteErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_ws_write_errors_total",
			Help: "Total number of WebSocket write-path failures, labeled by " +
				"reason (marshal, timeout, ping_timeout, other). A failure " +
				"typically terminates the connection.",
		}, []string{"reason"}),
		WSWriteDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mocrelay_ws_write_duration_seconds",
			Help:    "Time spent in each WebSocket conn.Write call in the writeLoop",
			Buckets: prometheus.DefBuckets,
		}),
	}

	reg.MustRegister(
		m.ConnectionsCurrent,
		m.ConnectionsTotal,
		m.MessagesReceived,
		m.MessagesSent,
		m.EventsReceived,
		m.WSParseErrors,
		m.WSWriteErrors,
		m.WSWriteDuration,
	)

	return m
}

// routerMetrics holds Prometheus metrics for Router.
type routerMetrics struct {
	MessagesDropped prometheus.Counter

	// SubscriptionsCurrent is the total number of active subscriptions
	// across every connection registered with the Router. Sums over
	// (per-connection subID) entries; a client that replaces an existing
	// subID does not cause drift.
	SubscriptionsCurrent prometheus.Gauge
}

// newRouterMetrics creates and registers Router metrics with reg.
// Returns nil if reg is nil.
func newRouterMetrics(reg prometheus.Registerer) *routerMetrics {
	if reg == nil {
		return nil
	}
	m := &routerMetrics{
		MessagesDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_router_messages_dropped_total",
			Help: "Total number of messages dropped due to full send channel",
		}),
		SubscriptionsCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mocrelay_router_subscriptions_current",
			Help: "Current number of active subscriptions across all connections",
		}),
	}

	reg.MustRegister(
		m.MessagesDropped,
		m.SubscriptionsCurrent,
	)

	return m
}

// authMetrics holds Prometheus metrics for AuthMiddleware.
//
// authMetrics is injected into the request context by [Relay] (constructed
// from [RelayOptions.Registerer]); the auth middleware reads it via
// [authMetricsFromContext] and no-ops on nil. Middleware code never holds a
// reference directly, mirroring the [rejectionMetrics] / Logger pattern.
//
// Rejection metrics are not part of this struct. They are unified across all
// middleware under [rejectionMetrics] and are wired automatically via
// [logRejection] — Auth contributes to the shared counter with label
// middleware="auth".
type authMetrics struct {
	AuthTotal                       *prometheus.CounterVec
	AuthenticatedConnectionsCurrent prometheus.Gauge
}

// newAuthMetrics creates and registers AuthMiddleware metrics with reg.
// Returns nil if reg is nil.
func newAuthMetrics(reg prometheus.Registerer) *authMetrics {
	if reg == nil {
		return nil
	}
	m := &authMetrics{
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

type authMetricsKey struct{}

// contextWithAuthMetrics returns a new context carrying the given auth
// metrics. [Relay] installs this at connection start so the auth middleware
// downstream can record attempts and authenticated connections via
// [authMetricsFromContext] without holding a direct reference.
func contextWithAuthMetrics(ctx context.Context, m *authMetrics) context.Context {
	return context.WithValue(ctx, authMetricsKey{}, m)
}

// authMetricsFromContext returns the auth metrics from the context, or nil
// if none was set. Callers must be nil-safe.
func authMetricsFromContext(ctx context.Context) *authMetrics {
	if m, ok := ctx.Value(authMetricsKey{}).(*authMetrics); ok {
		return m
	}
	return nil
}

// rejectionMetrics holds the unified rejection counter shared across every
// middleware in mocrelay. Each middleware reports its rejections through
// [logRejection], which increments this counter with labels
// {middleware, reason} — the same pair that appears in the structured log
// entry, so logs and metrics are cross-indexable.
//
// The (middleware, reason) pair is a functional relationship (each middleware
// emits a fixed, small enum of reasons), so the label set is bounded and
// does not form a cardinality cross-product — analogous to the (kind, type)
// two-axis label on [relayMetrics.EventsReceived].
//
// rejectionMetrics is injected into the request context by [Relay]
// (constructed from [RelayOptions.Registerer]); middleware code never holds
// a reference and never needs a nil check — [logRejection] handles both.
type rejectionMetrics struct {
	Total *prometheus.CounterVec
}

// newRejectionMetrics creates and registers the unified rejection counter
// with reg. Returns nil if reg is nil.
func newRejectionMetrics(reg prometheus.Registerer) *rejectionMetrics {
	if reg == nil {
		return nil
	}
	m := &rejectionMetrics{
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

// contextWithRejectionMetrics returns a new context carrying the given
// rejection metrics. [Relay] installs this at connection start so middleware
// downstream can report rejections via [logRejection] without holding a
// direct reference.
func contextWithRejectionMetrics(ctx context.Context, m *rejectionMetrics) context.Context {
	return context.WithValue(ctx, rejectionMetricsKey{}, m)
}

// rejectionMetricsFromContext returns the rejection metrics from the
// context, or nil if none was set. Callers (in practice only
// [logRejection]) must be nil-safe.
func rejectionMetricsFromContext(ctx context.Context) *rejectionMetrics {
	if m, ok := ctx.Value(rejectionMetricsKey{}).(*rejectionMetrics); ok {
		return m
	}
	return nil
}

// storageMetrics holds Prometheus metrics for Storage.
type storageMetrics struct {
	EventsStored  *prometheus.CounterVec
	StoreDuration prometheus.Histogram
	QueryDuration prometheus.Histogram

	// StoreErrors counts failed Storage.Store calls. One increment per
	// failed Store; paired with EventsStored to compute a success ratio.
	StoreErrors prometheus.Counter

	// QueryErrors counts failed Storage.Query calls. A Query reports its
	// error via errFn after iteration completes, so this counter is
	// incremented at most once per Query.
	QueryErrors prometheus.Counter
}

// newStorageMetrics creates and registers Storage metrics with reg.
// Returns nil if reg is nil.
func newStorageMetrics(reg prometheus.Registerer) *storageMetrics {
	if reg == nil {
		return nil
	}
	m := &storageMetrics{
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
		StoreErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_store_errors_total",
			Help: "Total number of failed Storage.Store calls",
		}),
		QueryErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_query_errors_total",
			Help: "Total number of failed Storage.Query calls (reported via errFn)",
		}),
	}

	reg.MustRegister(
		m.EventsStored,
		m.StoreDuration,
		m.QueryDuration,
		m.StoreErrors,
		m.QueryErrors,
	)

	return m
}

// compositeStorageMetrics holds Prometheus metrics for [CompositeStorage].
//
// These instrument the two places where [CompositeStorage] would otherwise
// silently swallow backend failures:
//
//   - Search (Query path): on [SearchIndex].Search error the Query falls
//     back to the primary storage, so the client sees a result — just not
//     a search-scored one. Without [SearchErrors] the backend could be
//     fully broken and nobody would notice.
//   - Index (Store path): [SearchIndex].Index is best-effort (the event is
//     already in primary), so its return value is discarded. Without
//     [IndexErrors] the search index can silently drift out of sync with
//     the primary.
//
// Pairing *Total with *Errors yields an error rate on each side. The
// underlying Bleve / SearchIndex resource state itself (doc count, index
// size, batch statistics) lives on the caller's [bleve.Index] and should
// be collected directly from idx.StatsMap() by the caller — see the
// "Caller-owned *pebble.DB and bleve.Index" section in CLAUDE.md.
type compositeStorageMetrics struct {
	// SearchTotal counts Query calls that hit the search backend (Search
	// filter non-empty and a search index is configured). Paired with
	// SearchErrors to derive a search error rate.
	SearchTotal prometheus.Counter

	// SearchErrors counts [SearchIndex].Search failures. On failure the
	// Query falls back to the primary storage; without this counter the
	// failure is silent.
	SearchErrors prometheus.Counter

	// IndexTotal counts [SearchIndex].Index attempts on successful Store
	// (non-ephemeral events with non-empty content). Paired with
	// IndexErrors to derive an indexing error rate.
	IndexTotal prometheus.Counter

	// IndexErrors counts [SearchIndex].Index failures. Indexing is
	// best-effort (the event is already in primary storage), so failures
	// do not propagate to the client — but a sustained non-zero rate
	// means the search index is drifting out of sync with the primary.
	IndexErrors prometheus.Counter
}

// newCompositeStorageMetrics creates and registers CompositeStorage
// metrics with reg. Returns nil if reg is nil.
func newCompositeStorageMetrics(reg prometheus.Registerer) *compositeStorageMetrics {
	if reg == nil {
		return nil
	}
	m := &compositeStorageMetrics{
		SearchTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_search_total",
			Help: "Total number of Query calls that hit the search backend " +
				"(Search filter non-empty and a search index configured).",
		}),
		SearchErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_search_errors_total",
			Help: "Total number of SearchIndex.Search failures. Query falls " +
				"back to the primary storage on failure, so clients see results " +
				"but not search-scored ones.",
		}),
		IndexTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_index_total",
			Help: "Total number of SearchIndex.Index attempts on successful Store " +
				"(non-ephemeral events with non-empty content).",
		}),
		IndexErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_index_errors_total",
			Help: "Total number of SearchIndex.Index failures. Indexing is " +
				"best-effort and failures do not propagate to the client, but a " +
				"sustained non-zero rate means the search index is drifting out " +
				"of sync with the primary.",
		}),
	}

	reg.MustRegister(
		m.SearchTotal,
		m.SearchErrors,
		m.IndexTotal,
		m.IndexErrors,
	)

	return m
}

// mergeHandlerMetrics holds Prometheus metrics for [NewMergeHandler].
//
// These surface the USE-Saturation and USE-Errors axes of the merge
// handler: how often downstream children are retired or stall on control
// broadcasts, and how many EVENT messages are dropped (on the way in,
// or during dedup/sort/limit enforcement on the way out).
type mergeHandlerMetrics struct {
	// LostChildrenTotal counts child handlers retired mid-flight. A lost
	// child contributes a synthesized accepted=false OK to its pending
	// responses (see MergeHandlerOKLostHandlerMessage) and prompts the
	// client to retry. Paired with BroadcastTimeoutsTotal to split the
	// causes: timeout on a control broadcast vs. child closed its send
	// channel (early exit / error).
	LostChildrenTotal prometheus.Counter

	// BroadcastTimeoutsTotal counts control-message (REQ / COUNT / CLOSE)
	// broadcast timeouts. Every timeout retires exactly one child and is
	// therefore also counted in LostChildrenTotal; subtract to isolate the
	// early-exit path. EVENT broadcasts are never counted here — they are
	// best-effort and land on EventDrops{reason="recv_buf_full"}.
	BroadcastTimeoutsTotal prometheus.Counter

	// EventDrops counts EVENT messages dropped at the merge boundary,
	// labeled by reason:
	//
	//   - recv_buf_full  — per-child recv buffer full during broadcast
	//                      (client is notified via OK accepted=false).
	//   - duplicate      — event id already seen for this subscription.
	//   - out_of_order   — breaks (created_at DESC, id ASC) sort order.
	//   - after_limit    — subscription already hit its limit / EOSE.
	EventDrops *prometheus.CounterVec
}

// newMergeHandlerMetrics creates and registers merge handler metrics with
// reg. Returns nil if reg is nil.
func newMergeHandlerMetrics(reg prometheus.Registerer) *mergeHandlerMetrics {
	if reg == nil {
		return nil
	}
	m := &mergeHandlerMetrics{
		LostChildrenTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_merge_lost_children_total",
			Help: "Total number of merge-handler child handlers retired " +
				"mid-flight (broadcast timeout or early exit).",
		}),
		BroadcastTimeoutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mocrelay_merge_broadcast_timeouts_total",
			Help: "Total number of merge-handler control-message broadcasts " +
				"that timed out waiting for a single child to drain.",
		}),
		EventDrops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mocrelay_merge_event_drops_total",
			Help: "Total number of EVENT messages dropped by the merge handler, " +
				"labeled by reason (recv_buf_full, duplicate, out_of_order, after_limit).",
		}, []string{"reason"}),
	}

	reg.MustRegister(
		m.LostChildrenTotal,
		m.BroadcastTimeoutsTotal,
		m.EventDrops,
	)

	return m
}
