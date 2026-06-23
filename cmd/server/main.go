// Command server is the HTTP entry point for the batch inference engine.
// It loads env config, wires the chi router, and serves job + health endpoints.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/amit-chahar/batch-inference-engine/internal/api"
	"github.com/amit-chahar/batch-inference-engine/internal/config"
	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/runner"
	"github.com/amit-chahar/batch-inference-engine/internal/storage"
	"github.com/amit-chahar/batch-inference-engine/internal/webhook"
	"github.com/amit-chahar/batch-inference-engine/internal/worker"
)

const version = "0.1.0"

func main() {
	cfg := config.Load()

	store := job.NewStore(cfg.JobsDir)
	inferenceLimiter := worker.NewConcurrencyLimiter(cfg.MaxWorkers)
	inferenceClient := worker.NewLimitedCompleter(
		worker.NewInferenceClientFromConfig(cfg),
		inferenceLimiter,
	)
	uploader := storage.NewSpacesUploaderFromConfig(cfg)
	batchRunner := runner.NewWithOptions(runner.Options{
		Store:       store,
		Completer:   inferenceClient,
		MaxWorkers:  cfg.MaxWorkers,
		ChannelSize: cfg.MaxWorkers * 2,
		ChunkSize:   cfg.ChunkSize,
		Uploader:    uploader,
		Notifier:    webhook.NewNotifier(),
	})

	if uploader.Enabled() {
		log.Printf("DO Spaces chunk upload enabled (bucket=%s region=%s)", cfg.SpacesBucket, cfg.SpacesRegion)
	}

	handler := api.NewHandlerWithRunner(version, store, batchRunner)
	router := api.NewRouter(handler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("batch inference engine listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
