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
	cfg := config.Load()
	handler := api.NewHandler(version)
	router := api.NewRouter(handler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("batch inference engine listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
