package mocrelay

import (
	"context"
	"iter"
	"strconv"
	"sync"
	"time"
)

// MetricsStorage wraps a Storage and collects Prometheus metrics.
type MetricsStorage struct {
	storage Storage
	metrics *StorageMetrics
}

// NewMetricsStorage creates a new MetricsStorage that wraps the given storage.
func NewMetricsStorage(storage Storage, metrics *StorageMetrics) *MetricsStorage {
	return &MetricsStorage{
		storage: storage,
		metrics: metrics,
	}
}

// Store implements Storage.Store with metrics collection.
func (s *MetricsStorage) Store(ctx context.Context, event *Event) (bool, error) {
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
