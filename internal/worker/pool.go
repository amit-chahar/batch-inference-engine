package worker

import (
	"context"
	"sync"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

// ItemCompleter runs inference for a single prompt row.
// InferenceClient satisfies this interface; tests can inject slow mocks.
type ItemCompleter interface {
	Complete(ctx context.Context, item job.PromptItem) job.PromptResult
}

// Pool limits concurrent inference calls using a fixed worker count.
// Each worker reads from a shared input channel — natural scatter pattern for Step 10 runner.
type Pool struct {
	maxWorkers int
	client     ItemCompleter
}

// NewPool creates a worker pool capped at maxWorkers concurrent Complete calls.
func NewPool(maxWorkers int, client ItemCompleter) *Pool {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	return &Pool{
		maxWorkers: maxWorkers,
		client:     client,
	}
}

// ProcessItems fans out items to at most maxWorkers concurrent inference calls.
// Results are emitted on the returned channel (order not guaranteed).
// The caller must close items; this method closes results when all workers finish.
func (p *Pool) ProcessItems(ctx context.Context, items <-chan job.PromptItem) <-chan job.PromptResult {
	results := make(chan job.PromptResult)

	go func() {
		defer close(results)

		var wg sync.WaitGroup
		for range p.maxWorkers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range items {
					select {
					case <-ctx.Done():
						results <- job.PromptResult{
							ID:     item.ID,
							Prompt: item.Prompt,
							Error:  stringPtr(ctx.Err().Error()),
						}
						continue
					default:
					}

					results <- p.client.Complete(ctx, item)
				}
			}()
		}

		wg.Wait()
	}()

	return results
}
