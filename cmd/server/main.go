// Command server is the HTTP entry point for the batch inference engine.
// It loads env config, wires the chi router, and serves job + health endpoints.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/amit-chahar/batch-inference-engine/internal/api"
	"github.com/amit-chahar/batch-inference-engine/internal/config"
)

const version = "0.1.0"

func main() {
	// All tunables (port, worker pool, DO inference URL/key) come from env/.env.
	cfg := config.Load()

	handler := api.NewHandler(version)
	router := api.NewRouter(handler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("batch inference engine listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
