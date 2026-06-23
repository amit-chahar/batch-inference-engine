package worker

import (
	"context"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

// ConcurrencyLimiter caps concurrent inference calls process-wide.
// Use one instance per server and wrap the shared inference client.
type ConcurrencyLimiter struct {
	sem chan struct{}
}

// NewConcurrencyLimiter creates a semaphore allowing at most max concurrent holders.
func NewConcurrencyLimiter(max int) *ConcurrencyLimiter {
	if max < 1 {
		max = 1
	}
	return &ConcurrencyLimiter{sem: make(chan struct{}, max)}
}

// Acquire blocks until a slot is available or ctx is cancelled.
func (l *ConcurrencyLimiter) Acquire(ctx context.Context) error {
	select {
	case l.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns a slot acquired with Acquire.
func (l *ConcurrencyLimiter) Release() {
	<-l.sem
}

// LimitedCompleter wraps an ItemCompleter with a process-wide concurrency cap.
type LimitedCompleter struct {
	inner   ItemCompleter
	limiter *ConcurrencyLimiter
}

// NewLimitedCompleter returns a completer that shares limiter across all jobs.
func NewLimitedCompleter(inner ItemCompleter, limiter *ConcurrencyLimiter) ItemCompleter {
	if limiter == nil {
		return inner
	}
	return LimitedCompleter{inner: inner, limiter: limiter}
}

// Complete runs inference under the global concurrency limit.
func (c LimitedCompleter) Complete(ctx context.Context, item job.PromptItem) job.PromptResult {
	if err := c.limiter.Acquire(ctx); err != nil {
		errMsg := err.Error()
		return job.PromptResult{ID: item.ID, Prompt: item.Prompt, Error: &errMsg}
	}
	defer c.limiter.Release()
	return c.inner.Complete(ctx, item)
}
