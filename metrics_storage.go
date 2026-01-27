//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"iter"
	"strconv"
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

	if err == nil {
		kindStr := strconv.FormatInt(event.Kind, 10)
		storedStr := strconv.FormatBool(stored)
		s.metrics.EventsStored.WithLabelValues(kindStr, storedStr).Inc()
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

	// Wrap closeFn to record duration
	wrappedCloseFn := func() error {
		duration := time.Since(start)
		s.metrics.QueryDuration.Observe(duration.Seconds())
		return closeFn()
	}

	return wrappedEvents, errFn, wrappedCloseFn
}
