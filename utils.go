package mocrelay

import (
	"context"
	"time"
)

func toPtr[T any](v T) *T { return &v }

type rateLimiter struct {
	C      chan struct{}
	cancel context.CancelFunc
}

func newRateLimiter(rate time.Duration, burst int) *rateLimiter {
	c := make(chan struct{}, burst)
	if rate == 0 {
		close(c)
	} else {
		for i := 0; i < burst; i++ {
			c <- struct{}{}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if rate == 0 {
			return
		}

		t := time.NewTicker(rate)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c <- struct{}{}
			}
		}
	}()

	return &rateLimiter{
		C:      c,
		cancel: cancel,
	}
}

func (l *rateLimiter) Stop() { l.cancel() }
