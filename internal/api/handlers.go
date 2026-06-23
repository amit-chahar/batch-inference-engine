// Package api exposes the REST surface for batch job submission, status, and download.
// Handlers stay thin; background processing lives in internal/runner.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/amit-chahar/batch-inference-engine/internal/ingest"
	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/runner"
)

// Handler serves HTTP endpoints for the batch inference engine.
type Handler struct {
	version string
	store   *job.Store
	runner  *runner.Runner
}

// NewHandler constructs an API handler with default store and a noop runner (health-only tests).
func NewHandler(version string) *Handler {
	store := job.NewStore("data/jobs")
	return NewHandlerWithRunner(version, store, runner.New(store, stubNoopCompleter{}, 1, 2))
}

// NewHandlerWithStore constructs handlers with an explicit store (health tests).
func NewHandlerWithStore(version string, store *job.Store) *Handler {
	return NewHandlerWithRunner(version, store, runner.New(store, stubNoopCompleter{}, 1, 2))
}

// NewHandlerWithRunner constructs handlers with explicit store and runner dependencies.
func NewHandlerWithRunner(version string, store *job.Store, batchRunner *runner.Runner) *Handler {
	return &Handler{version: version, store: store, runner: batchRunner}
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health reports service liveness. Used by load balancers and CI smoke checks.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "ok",
		Version: h.version,
	})
}

type submitRequest struct {
	InputFile string `json:"input_file"`
}

type submitResponse struct {
	JobID      string        `json:"job_id"`
	Status     job.JobStatus `json:"status"`
	TotalItems int           `json:"total_items"`
}

type statusResponse struct {
	JobID           string        `json:"job_id"`
	Status          job.JobStatus `json:"status"`
	TotalItems      int           `json:"total_items"`
	CompletedItems  int           `json:"completed_items"`
	FailedItems     int           `json:"failed_items"`
	ProgressPercent float64       `json:"progress_percent"`
}

// Submit creates a job and starts background processing for the JSONL input file.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	req.InputFile = strings.TrimSpace(req.InputFile)
	if req.InputFile == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input_file is required"})
		return
	}

	totalItems, err := ingest.CountNonEmptyLines(req.InputFile)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	meta, err := h.store.CreateJob(totalItems)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create job"})
		return
	}

	h.runner.ProcessAsync(meta.JobID, req.InputFile)

	writeJSON(w, http.StatusAccepted, submitResponse{
		JobID:      meta.JobID,
		Status:     meta.Status,
		TotalItems: meta.TotalItems,
	})
}

// Status returns current persisted job progress.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	meta, err := h.store.GetMeta(chi.URLParam(r, "id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err == job.ErrJobNotFound {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, statusResponse{
		JobID:           meta.JobID,
		Status:          meta.Status,
		TotalItems:      meta.TotalItems,
		CompletedItems:  meta.CompletedItems,
		FailedItems:     meta.FailedItems,
		ProgressPercent: meta.ProgressPercent(),
	})
}

// Download streams merged job results as a JSON array without loading all rows into memory.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	meta, err := h.store.GetMeta(jobID)
	if err != nil {
		status := http.StatusInternalServerError
		if err == job.ErrJobNotFound {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if meta.Status == job.JobStatusPending || meta.Status == job.JobStatusRunning {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job still running"})
		return
	}

	path, err := h.store.ResultsPath(jobID)
	if err != nil {
		status := http.StatusInternalServerError
		if err == job.ErrJobNotFound {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	file, err := os.Open(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "open results"})
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := job.WriteResultsArray(w, file); err != nil {
		// Headers may already be sent; client may see truncated JSON.
		return
	}
}

// stubNoopCompleter satisfies handler construction in tests that only hit /health.
type stubNoopCompleter struct{}

func (stubNoopCompleter) Complete(_ context.Context, item job.PromptItem) job.PromptResult {
	response := "noop"
	return job.PromptResult{ID: item.ID, Prompt: item.Prompt, Response: &response}
}

// writeJSON is a small helper shared by all handlers for consistent JSON responses.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
