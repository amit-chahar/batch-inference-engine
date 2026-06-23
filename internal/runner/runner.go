// Package runner wires ingest → bounded channel → worker pool → disk job store.
package runner

import (
	"context"
	"log"

	"github.com/amit-chahar/batch-inference-engine/internal/ingest"
	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/storage"
	"github.com/amit-chahar/batch-inference-engine/internal/webhook"
	"github.com/amit-chahar/batch-inference-engine/internal/worker"
)

// Options configures the scatter-gather batch pipeline.
type Options struct {
	Store       *job.Store
	Completer   worker.ItemCompleter
	MaxWorkers  int
	ChannelSize int
	ChunkSize   int
	Uploader    storage.ChunkUploader
	Notifier    *webhook.Notifier
}

// Runner orchestrates the scatter-gather batch pipeline.
type Runner struct {
	store       *job.Store
	pool        *worker.Pool
	channelSize int
	chunkSize   int
	uploader    storage.ChunkUploader
	notifier    *webhook.Notifier
}

// New creates a runner with a bounded item channel between ingest and workers.
func New(store *job.Store, completer worker.ItemCompleter, maxWorkers, channelSize int) *Runner {
	return NewWithOptions(Options{
		Store:       store,
		Completer:   completer,
		MaxWorkers:  maxWorkers,
		ChannelSize: channelSize,
	})
}

// NewWithOptions constructs a runner with optional chunk upload and webhook support.
func NewWithOptions(opts Options) *Runner {
	channelSize := opts.ChannelSize
	if channelSize < 1 {
		channelSize = opts.MaxWorkers * 2
	}
	uploader := opts.Uploader
	if uploader == nil {
		uploader = storage.NoopUploader{}
	}
	return &Runner{
		store:       opts.Store,
		pool:        worker.NewPool(opts.MaxWorkers, opts.Completer),
		channelSize: channelSize,
		chunkSize:   opts.ChunkSize,
		uploader:    uploader,
		notifier:    opts.Notifier,
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
		if err := r.persistResult(ctx, jobID, result); err != nil {
			_ = r.store.SetStatus(jobID, job.JobStatusFailed)
			<-bridgeDone
			return err
		}
	}

	<-bridgeDone
	return r.finalize(ctx, jobID)
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
			failed := job.PromptResult{Error: stringPtr(err.Error())}
			if persistErr := r.persistResult(ctx, jobID, failed); persistErr != nil {
				return
			}
		}
	}
}

func (r *Runner) persistResult(ctx context.Context, jobID string, result job.PromptResult) error {
	sealed, err := r.store.AppendResultWithChunking(jobID, result, r.chunkSize)
	if err != nil {
		return err
	}
	if sealed != nil {
		if err := r.uploadChunk(ctx, jobID, sealed); err != nil {
			return err
		}
	}
	if result.Error != nil {
		return r.store.IncrementFailed(jobID)
	}
	return r.store.IncrementCompleted(jobID)
}

func (r *Runner) uploadChunk(ctx context.Context, jobID string, sealed *job.SealedChunk) error {
	if !r.uploader.Enabled() {
		return nil
	}
	url, err := r.uploader.UploadFile(ctx, sealed.ObjectKey, sealed.LocalPath)
	if err != nil {
		return err
	}
	return r.store.AddChunkKey(jobID, url)
}

func (r *Runner) finalize(ctx context.Context, jobID string) error {
	if r.chunkSize > 0 {
		sealed, err := r.store.SealActiveChunkIfNonEmpty(jobID)
		if err != nil {
			return err
		}
		if sealed != nil {
			if err := r.uploadChunk(ctx, jobID, sealed); err != nil {
				return err
			}
		}
	}

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

	if err := r.store.SetStatus(jobID, status); err != nil {
		return err
	}

	meta, err = r.store.GetMeta(jobID)
	if err != nil {
		return err
	}

	if meta.CallbackURL != "" && r.notifier != nil {
		if err := r.notifier.Notify(ctx, meta.CallbackURL, webhook.PayloadFromMeta(meta)); err != nil {
			log.Printf("webhook notify job=%s: %v", jobID, err)
		}
	}
	return nil
}

func stringPtr(value string) *string {
	return &value
}
