package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/runner"
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

func TestSubmitStatusDownloadFlow(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "batch.jsonl")
	content := strings.Join([]string{
		`{"id":"prompt-0000","prompt":"First prompt","metadata":{"topic":"test"}}`,
		`{"id":"prompt-0001","prompt":"Second prompt"}`,
		"",
	}, "\n")
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := job.NewStore(t.TempDir())
	run := runner.New(store, apiStubCompleter{}, 2, 4)
	handler := NewHandlerWithRunner("0.1.0", store, run)
	router := NewRouter(handler)

	body := []byte(`{"input_file":` + strconvQuote(inputPath) + `}`)
	submitReq := httptest.NewRequest(http.MethodPost, "/job/submit", bytes.NewReader(body))
	submitRec := httptest.NewRecorder()
	router.ServeHTTP(submitRec, submitReq)
	if submitRec.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d, body = %s", submitRec.Code, submitRec.Body.String())
	}

	var submitted submitResponse
	if err := json.NewDecoder(submitRec.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	if submitted.JobID == "" {
		t.Fatal("expected job id")
	}
	if submitted.TotalItems != 2 {
		t.Fatalf("total_items = %d, want 2", submitted.TotalItems)
	}

	status := waitForTerminalStatus(t, router, submitted.JobID)
	if status.Status != job.JobStatusCompleted {
		t.Fatalf("job status = %q, want completed", status.Status)
	}
	if status.TotalItems != 2 || status.CompletedItems != 2 || status.FailedItems != 0 {
		t.Fatalf("status counts = completed:%d failed:%d total:%d", status.CompletedItems, status.FailedItems, status.TotalItems)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/job/"+submitted.JobID+"/download", nil)
	downloadRec := httptest.NewRecorder()
	router.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", downloadRec.Code, downloadRec.Body.String())
	}
	lines := strings.Split(strings.TrimSpace(downloadRec.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("download result lines = %d, want 2", len(lines))
	}
	if !strings.Contains(downloadRec.Body.String(), "prompt-0000") {
		t.Fatalf("download body missing prompt id: %s", downloadRec.Body.String())
	}
}

type apiStubCompleter struct{}

func (apiStubCompleter) Complete(_ context.Context, item job.PromptItem) job.PromptResult {
	response := "Processed prompt " + item.ID
	return job.PromptResult{
		ID:       item.ID,
		Prompt:   item.Prompt,
		Response: &response,
		Metadata: item.Metadata,
	}
}

func waitForTerminalStatus(t *testing.T, router http.Handler, jobID string) statusResponse {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
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

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
