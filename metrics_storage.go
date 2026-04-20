package mocrelay

import (
	"context"
	"iter"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsStorage wraps a Storage and collects Prometheus metrics for each
// Store / Query call (mocrelay_events_stored_total, mocrelay_store_duration_seconds,
// mocrelay_query_duration_seconds, mocrelay_store_errors_total,
// mocrelay_query_errors_total).
//
// Because mocrelay's Storage interface has multiple implementations
// (InMemory / Pebble / Composite), MetricsStorage is a decorator rather than
// an Options-style constructor: wrap whatever Storage you want to measure.
type MetricsStorage struct {
	storage Storage
	metrics *storageMetrics
}

// NewMetricsStorage creates a MetricsStorage wrapping storage. Metrics are
// registered against reg; if reg is nil, all metric calls no-op and the
// decorator is effectively a pass-through (no registration happens and no
// Prometheus collectors are created).
func NewMetricsStorage(storage Storage, reg prometheus.Registerer) *MetricsStorage {
	return &MetricsStorage{
		storage: storage,
		metrics: newStorageMetrics(reg),
	}
}

// Store implements Storage.Store with metrics collection.
func (s *MetricsStorage) Store(ctx context.Context, event *Event) (bool, error) {
	if s.metrics == nil {
		return s.storage.Store(ctx, event)
	}

	start := time.Now()
	stored, err := s.storage.Store(ctx, event)
	duration := time.Since(start)

	s.metrics.StoreDuration.Observe(duration.Seconds())

	if err != nil {
		s.metrics.StoreErrors.Inc()
	} else {
		kind, typ := kindLabels(event.Kind)
		s.metrics.EventsStored.WithLabelValues(kind, typ, strconv.FormatBool(stored)).Inc()
	}

	return stored, err
}

// Query implements Storage.Query with metrics collection.
func (s *MetricsStorage) Query(ctx context.Context, filters []*ReqFilter) (iter.Seq[*Event], func() error, func() error) {
	if s.metrics == nil {
		return s.storage.Query(ctx, filters)
	}

	start := time.Now()
	events, errFn, closeFn := s.storage.Query(ctx, filters)

	// Wrap the events iterator to observe duration when iteration completes
	wrappedEvents := func(yield func(*Event) bool) {
		for event := range events {
			if !yield(event) {
				return
			}
		}
	}

	// Wrap errFn to count Query errors. Guarded by sync.Once so a caller
	// invoking errFn multiple times (allowed by the Storage contract)
	// doesn't inflate the counter.
	var errOnce sync.Once
	wrappedErrFn := func() error {
		err := errFn()
		if err != nil {
			errOnce.Do(func() {
				s.metrics.QueryErrors.Inc()
			})
		}
		return err
	}

	// Wrap closeFn to record duration
	wrappedCloseFn := func() error {
		duration := time.Since(start)
		s.metrics.QueryDuration.Observe(duration.Seconds())
		return closeFn()
	}

	return wrappedEvents, wrappedErrFn, wrappedCloseFn
}
