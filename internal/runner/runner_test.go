package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/worker"
)

type stubCompleter struct {
	response string
}

func (s stubCompleter) Complete(_ context.Context, item job.PromptItem) job.PromptResult {
	response := s.response
	if response == "" {
		response = "ok-" + item.ID
	}
	return job.PromptResult{
		ID:       item.ID,
		Prompt:   item.Prompt,
		Response: &response,
		Metadata: item.Metadata,
	}
}

func writeFixture(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestRunnerProcessesFiveItemsEndToEnd(t *testing.T) {
	store := job.NewStore(t.TempDir())
	r := New(store, stubCompleter{}, 2, 4)

	var lines []string
	for i := range 5 {
		lines = append(lines, fmt.Sprintf(
			`{"id":"prompt-%04d","prompt":"Prompt %d","metadata":{"index":%d}}`,
			i, i, i,
		))
	}
	inputPath := writeFixture(t, lines...)

	meta, err := store.CreateJob(len(lines))
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if err := r.Process(context.Background(), meta.JobID, inputPath); err != nil {
		t.Fatalf("Process: %v", err)
	}

	got, err := store.GetMeta(meta.JobID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got.Status != job.JobStatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if got.CompletedItems != 5 {
		t.Fatalf("completed_items = %d, want 5", got.CompletedItems)
	}
	if got.FailedItems != 0 {
		t.Fatalf("failed_items = %d, want 0", got.FailedItems)
	}

	resultsPath, err := store.ResultsPath(meta.JobID)
	if err != nil {
		t.Fatalf("ResultsPath: %v", err)
	}
	data, err := os.ReadFile(resultsPath)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	resultLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(resultLines) != 5 {
		t.Fatalf("result lines = %d, want 5", len(resultLines))
	}
}

func TestRunnerContinuesAfterInferenceRowFailure(t *testing.T) {
	store := job.NewStore(t.TempDir())
	r := New(store, worker.ItemCompleter(completerFunc(func(_ context.Context, item job.PromptItem) job.PromptResult {
		if item.ID == "prompt-0001" {
			errMsg := "upstream status 400: bad request"
			return job.PromptResult{ID: item.ID, Prompt: item.Prompt, Error: &errMsg}
		}
		response := "ok"
		return job.PromptResult{ID: item.ID, Prompt: item.Prompt, Response: &response}
	})), 2, 4)

	inputPath := writeFixture(t,
		`{"id":"prompt-0000","prompt":"one"}`,
		`{"id":"prompt-0001","prompt":"two"}`,
		`{"id":"prompt-0002","prompt":"three"}`,
	)

	meta, err := store.CreateJob(3)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := r.Process(context.Background(), meta.JobID, inputPath); err != nil {
		t.Fatalf("Process: %v", err)
	}

	got, err := store.GetMeta(meta.JobID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got.Status != job.JobStatusPartial {
		t.Fatalf("status = %q, want partial", got.Status)
	}
	if got.CompletedItems != 2 || got.FailedItems != 1 {
		t.Fatalf("completed=%d failed=%d", got.CompletedItems, got.FailedItems)
	}
}

type completerFunc func(context.Context, job.PromptItem) job.PromptResult

func (f completerFunc) Complete(ctx context.Context, item job.PromptItem) job.PromptResult {
	return f(ctx, item)
}

func TestRunnerRecordsIngestParseFailures(t *testing.T) {
	store := job.NewStore(t.TempDir())
	r := New(store, stubCompleter{}, 1, 2)

	inputPath := writeFixture(t,
		`{"id":"prompt-0000","prompt":"ok"}`,
		`not-json`,
		`{"id":"prompt-0001","prompt":"also ok"}`,
	)

	meta, err := store.CreateJob(3)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := r.Process(context.Background(), meta.JobID, inputPath); err != nil {
		t.Fatalf("Process: %v", err)
	}

	got, err := store.GetMeta(meta.JobID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got.CompletedItems != 2 || got.FailedItems != 1 {
		t.Fatalf("completed=%d failed=%d, want 2/1", got.CompletedItems, got.FailedItems)
	}
	if got.Status != job.JobStatusPartial {
		t.Fatalf("status = %q, want partial", got.Status)
	}
}
