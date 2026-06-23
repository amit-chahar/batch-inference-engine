package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

// slowCompleter simulates inference latency and tracks peak concurrent calls.
type slowCompleter struct {
	delay time.Duration

	mu          sync.Mutex
	inFlight    int
	maxObserved int
}

func (c *slowCompleter) Complete(ctx context.Context, item job.PromptItem) job.PromptResult {
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.maxObserved {
		c.maxObserved = c.inFlight
	}
	c.mu.Unlock()

	timer := time.NewTimer(c.delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		c.decrement()
		return job.PromptResult{
			ID:     item.ID,
			Prompt: item.Prompt,
			Error:  stringPtr(ctx.Err().Error()),
		}
	case <-timer.C:
	}

	c.decrement()
	response := "ok"
	return job.PromptResult{
		ID:       item.ID,
		Prompt:   item.Prompt,
		Response: &response,
	}
}

func (c *slowCompleter) decrement() {
	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
}

func (c *slowCompleter) peakConcurrency() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxObserved
}

func TestPoolConcurrencyNeverExceedsMaxWorkers(t *testing.T) {
	const maxWorkers = 3
	const itemCount = 12

	completer := &slowCompleter{delay: 25 * time.Millisecond}
	pool := NewPool(maxWorkers, completer)

	items := make(chan job.PromptItem, itemCount)
	for i := range itemCount {
		items <- job.PromptItem{
			ID:     fmt.Sprintf("prompt-%02d", i),
			Prompt: "test",
		}
	}
	close(items)

	results := pool.ProcessItems(context.Background(), items)

	count := 0
	for range results {
		count++
	}

	if count != itemCount {
		t.Fatalf("result count = %d, want %d", count, itemCount)
	}
	if peak := completer.peakConcurrency(); peak > maxWorkers {
		t.Fatalf("peak concurrency = %d, want <= %d", peak, maxWorkers)
	}
}

func TestPoolProcessesAllItems(t *testing.T) {
	var calls atomic.Int32

	pool := NewPool(2, ItemCompleter(completerFunc(func(ctx context.Context, item job.PromptItem) job.PromptResult {
		calls.Add(1)
		response := item.ID
		return job.PromptResult{ID: item.ID, Prompt: item.Prompt, Response: &response}
	})))

	items := make(chan job.PromptItem, 5)
	for range 5 {
		items <- job.PromptItem{ID: "id", Prompt: "prompt"}
	}
	close(items)

	count := 0
	for range pool.ProcessItems(context.Background(), items) {
		count++
	}

	if calls.Load() != 5 {
		t.Fatalf("calls = %d, want 5", calls.Load())
	}
	if count != 5 {
		t.Fatalf("results = %d, want 5", count)
	}
}

type completerFunc func(ctx context.Context, item job.PromptItem) job.PromptResult

func (f completerFunc) Complete(ctx context.Context, item job.PromptItem) job.PromptResult {
	return f(ctx, item)
}

func TestPoolDefaultsToOneWorker(t *testing.T) {
	pool := NewPool(0, completerFunc(func(ctx context.Context, item job.PromptItem) job.PromptResult {
		response := "ok"
		return job.PromptResult{ID: item.ID, Response: &response}
	}))

	if pool.maxWorkers != 1 {
		t.Fatalf("maxWorkers = %d, want 1", pool.maxWorkers)
	}
}
