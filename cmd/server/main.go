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
	"github.com/amit-chahar/batch-inference-engine/internal/worker"
)

const version = "0.1.0"

func main() {
	// All tunables (port, worker pool, DO inference URL/key) come from env/.env.
	cfg := config.Load()

	store := job.NewStore(cfg.JobsDir)
	inferenceClient := worker.NewInferenceClientFromConfig(cfg)
	runner := runner.New(store, inferenceClient, cfg.MaxWorkers, cfg.MaxWorkers*2)

	handler := api.NewHandlerWithRunner(version, store, runner)
	router := api.NewRouter(handler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("batch inference engine listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
