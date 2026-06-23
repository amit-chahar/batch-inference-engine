// Package runner wires ingest → bounded channel → worker pool → disk job store.
package runner

import (
	"context"

	"github.com/amit-chahar/batch-inference-engine/internal/ingest"
	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/worker"
)

// Runner orchestrates the scatter-gather batch pipeline.
type Runner struct {
	store       *job.Store
	pool        *worker.Pool
	channelSize int
}

// New creates a runner with a bounded item channel between ingest and workers.
func New(store *job.Store, completer worker.ItemCompleter, maxWorkers, channelSize int) *Runner {
	if channelSize < 1 {
		channelSize = maxWorkers * 2
	}
	return &Runner{
		store:       store,
		pool:        worker.NewPool(maxWorkers, completer),
		channelSize: channelSize,
	}
}

// ProcessAsync starts background processing detached from the HTTP request lifecycle.
func (r *Runner) ProcessAsync(jobID, inputFile string) {
	go func() {
		_ = r.Process(context.Background(), jobID, inputFile)
	}()
}

// Process runs the full batch pipeline synchronously (used by tests and async wrapper).
func (r *Runner) Process(ctx context.Context, jobID, inputFile string) error {
	if err := r.store.SetStatus(jobID, job.JobStatusRunning); err != nil {
		return err
	}

	rawItems, ingestErrs := ingest.StreamItems(inputFile)
	bounded := make(chan job.PromptItem, r.channelSize)

	bridgeDone := make(chan struct{})
	go func() {
		defer close(bounded)
		defer close(bridgeDone)
		r.bridgeIngest(ctx, jobID, rawItems, ingestErrs, bounded)
	}()

	results := r.pool.ProcessItems(ctx, bounded)
	for result := range results {
		if err := r.persistResult(jobID, result); err != nil {
			_ = r.store.SetStatus(jobID, job.JobStatusFailed)
			<-bridgeDone
			return err
		}
	}

	<-bridgeDone
	return r.finalizeStatus(jobID)
}

func (r *Runner) bridgeIngest(
	ctx context.Context,
	jobID string,
	items <-chan job.PromptItem,
	errs <-chan error,
	out chan<- job.PromptItem,
) {
	itemsOpen := true
	errsOpen := true

	for itemsOpen || errsOpen {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-items:
			if !ok {
				itemsOpen = false
				continue
			}
			select {
			case out <- item:
			case <-ctx.Done():
				return
			}
		case err, ok := <-errs:
			if !ok {
				errsOpen = false
				continue
			}
			if err == nil {
				continue
			}
			// Malformed JSONL row: record failure and continue scanning.
			failed := job.PromptResult{Error: stringPtr(err.Error())}
			_ = r.store.AppendResult(jobID, failed)
			_ = r.store.IncrementFailed(jobID)
		}
	}
}

func (r *Runner) persistResult(jobID string, result job.PromptResult) error {
	if err := r.store.AppendResult(jobID, result); err != nil {
		return err
	}
	if result.Error != nil {
		return r.store.IncrementFailed(jobID)
	}
	return r.store.IncrementCompleted(jobID)
}

func (r *Runner) finalizeStatus(jobID string) error {
	meta, err := r.store.GetMeta(jobID)
	if err != nil {
		return err
	}

	status := job.JobStatusCompleted
	switch {
	case meta.FailedItems > 0 && meta.CompletedItems > 0:
		status = job.JobStatusPartial
	case meta.FailedItems > 0:
		status = job.JobStatusFailed
	}

	return r.store.SetStatus(jobID, status)
}

func stringPtr(value string) *string {
	return &value
}
