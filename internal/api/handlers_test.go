package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/runner"
	"github.com/amit-chahar/batch-inference-engine/internal/worker"
)

func TestHealth(t *testing.T) {
	handler := NewHandler("0.1.0")
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q, want ok", body["status"])
	}
	if body["version"] != "0.1.0" {
		t.Fatalf("version = %q, want 0.1.0", body["version"])
	}
}

// TestE2EJobLifecycle exercises the full HTTP stack: submit → poll → download.
// Inference is served by an httptest mock in the same shape as DO Serverless Inference.
func TestE2EJobLifecycle(t *testing.T) {
	const itemCount = 5
	var inferenceCalls atomic.Int32

	mockInference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inferenceCalls.Add(1)

		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("authorization = %q, want Bearer test-key", auth)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{
					"role":    "assistant",
					"content": "mock: " + req.Messages[0].Content,
				}},
			},
		})
	}))
	t.Cleanup(mockInference.Close)

	store := job.NewStore(t.TempDir())
	inferenceClient := worker.NewInferenceClient(worker.InferenceOptions{
		HTTPClient: mockInference.Client(),
		APIURL:     mockInference.URL,
		APIKey:     "test-key",
		Model:      "test-model",
		MaxRetries: 2,
		Backoff:    worker.NewBackoffWithRand(time.Millisecond, 10*time.Millisecond, rand.New(rand.NewSource(1))),
		Sleep:      func(time.Duration) {},
	})
	batchRunner := runner.New(store, inferenceClient, 2, 4)
	router := NewRouter(NewHandlerWithRunner("0.1.0", store, batchRunner))

	inputPath := writeBatchJSONL(t, itemCount)
	submitted := submitJob(t, router, inputPath)
	if submitted.TotalItems != itemCount {
		t.Fatalf("total_items = %d, want %d", submitted.TotalItems, itemCount)
	}

	status := pollUntilTerminal(t, router, submitted.JobID)
	if status.Status != job.JobStatusCompleted {
		t.Fatalf("job status = %q, want completed", status.Status)
	}
	if status.TotalItems != itemCount || status.CompletedItems != itemCount || status.FailedItems != 0 {
		t.Fatalf("status counts = completed:%d failed:%d total:%d", status.CompletedItems, status.FailedItems, status.TotalItems)
	}
	if status.ProgressPercent != 100 {
		t.Fatalf("progress_percent = %v, want 100", status.ProgressPercent)
	}

	results := downloadResults(t, router, submitted.JobID)
	if len(results) != itemCount {
		t.Fatalf("download result count = %d, want %d", len(results), itemCount)
	}
	for i, result := range results {
		if result.Error != nil {
			t.Fatalf("result[%d] error = %q", i, *result.Error)
		}
		if result.Response == nil || !strings.HasPrefix(*result.Response, "mock: ") {
			t.Fatalf("result[%d] response = %v, want mock prefix", i, result.Response)
		}
	}

	if got := int(inferenceCalls.Load()); got != itemCount {
		t.Fatalf("inference calls = %d, want %d", got, itemCount)
	}
}

func TestDownloadReturns409WhileJobRunning(t *testing.T) {
	store := job.NewStore(t.TempDir())
	meta, err := store.CreateJob(2)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := store.SetStatus(meta.JobID, job.JobStatusRunning); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	handler := NewHandlerWithRunner("0.1.0", store, runner.New(store, blockedCompleter{}, 1, 2))
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/job/"+meta.JobID+"/download", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

type blockedCompleter struct{}

func (blockedCompleter) Complete(ctx context.Context, item job.PromptItem) job.PromptResult {
	<-ctx.Done()
	return job.PromptResult{ID: item.ID, Prompt: item.Prompt}
}

func writeBatchJSONL(t *testing.T, itemCount int) string {
	t.Helper()

	lines := make([]string, itemCount)
	for i := range itemCount {
		lines[i] = fmt.Sprintf(`{"id":"prompt-%04d","prompt":"Prompt %d","metadata":{"index":%d}}`, i, i, i)
	}

	path := filepath.Join(t.TempDir(), "batch.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	return path
}

func submitJob(t *testing.T, router http.Handler, inputPath string) submitResponse {
	t.Helper()

	body := []byte(`{"input_file":` + strconvQuote(inputPath) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/job/submit", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var submitted submitResponse
	if err := json.NewDecoder(rec.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	if submitted.JobID == "" {
		t.Fatal("expected job id")
	}
	return submitted
}

func pollUntilTerminal(t *testing.T, router http.Handler, jobID string) statusResponse {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/job/"+jobID+"/status", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, body = %s", rec.Code, rec.Body.String())
		}

		var status statusResponse
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if status.Status == job.JobStatusCompleted || status.Status == job.JobStatusFailed || status.Status == job.JobStatusPartial {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish before deadline; last status = %q", status.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func downloadResults(t *testing.T, router http.Handler, jobID string) []job.PromptResult {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/job/"+jobID+"/download", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}

	var results []job.PromptResult
	if err := json.NewDecoder(rec.Body).Decode(&results); err != nil {
		t.Fatalf("decode download JSON array: %v, body = %s", err, rec.Body.String())
	}
	return results
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
