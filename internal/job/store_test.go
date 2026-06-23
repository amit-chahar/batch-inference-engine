package job

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir())
}

func strPtr(value string) *string {
	return &value
}

func TestCreateJobWritesMetaAndResultsFile(t *testing.T) {
	store := newTestStore(t)

	meta, err := store.CreateJob(1000)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if meta.JobID == "" {
		t.Fatal("expected job ID")
	}
	if meta.Status != JobStatusPending {
		t.Fatalf("status = %q, want pending", meta.Status)
	}
	if meta.TotalItems != 1000 {
		t.Fatalf("total_items = %d, want 1000", meta.TotalItems)
	}
	if meta.CreatedAt.IsZero() {
		t.Fatal("expected created_at")
	}

	metaPath := filepath.Join(store.rootDir, meta.JobID, "meta.json")
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("meta.json missing: %v", err)
	}

	resultsPath := filepath.Join(store.rootDir, meta.JobID, "results.jsonl")
	if _, err := os.Stat(resultsPath); err != nil {
		t.Fatalf("results.jsonl missing: %v", err)
	}
}

func TestGetMetaReturnsCreatedJob(t *testing.T) {
	store := newTestStore(t)

	created, err := store.CreateJob(10)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := store.GetMeta(created.JobID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got.JobID != created.JobID {
		t.Fatalf("job_id = %q, want %q", got.JobID, created.JobID)
	}
}

func TestGetMetaMissingJob(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetMeta("missing-job")
	if err != ErrJobNotFound {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestIncrementCountersAndSetStatus(t *testing.T) {
	store := newTestStore(t)

	meta, err := store.CreateJob(5)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if err := store.IncrementCompleted(meta.JobID); err != nil {
		t.Fatalf("IncrementCompleted: %v", err)
	}
	if err := store.IncrementCompleted(meta.JobID); err != nil {
		t.Fatalf("IncrementCompleted: %v", err)
	}
	if err := store.IncrementFailed(meta.JobID); err != nil {
		t.Fatalf("IncrementFailed: %v", err)
	}
	if err := store.SetStatus(meta.JobID, JobStatusRunning); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	got, err := store.GetMeta(meta.JobID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got.CompletedItems != 2 {
		t.Fatalf("completed_items = %d, want 2", got.CompletedItems)
	}
	if got.FailedItems != 1 {
		t.Fatalf("failed_items = %d, want 1", got.FailedItems)
	}
	if got.Status != JobStatusRunning {
		t.Fatalf("status = %q, want running", got.Status)
	}
}

func TestAppendResultWritesJSONL(t *testing.T) {
	store := newTestStore(t)

	meta, err := store.CreateJob(1)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	result := PromptResult{
		ID:       "prompt-0000",
		Prompt:   "hello",
		Response: strPtr("world"),
	}
	if err := store.AppendResult(meta.JobID, result); err != nil {
		t.Fatalf("AppendResult: %v", err)
	}

	resultsPath := filepath.Join(store.rootDir, meta.JobID, "results.jsonl")
	file, err := os.Open(resultsPath)
	if err != nil {
		t.Fatalf("open results: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("expected one result line")
	}

	var got PromptResult
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.ID != result.ID {
		t.Fatalf("id = %q, want %q", got.ID, result.ID)
	}
	if got.Response == nil || *got.Response != "world" {
		t.Fatalf("response = %v, want world", got.Response)
	}
}

func TestConcurrentAppends(t *testing.T) {
	// Simulates multiple workers appending results + updating counters at once.
	store := newTestStore(t)

	meta, err := store.CreateJob(20)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			result := PromptResult{
				ID:       fmt.Sprintf("prompt-%04d", index),
				Prompt:   "prompt",
				Response: strPtr("ok"),
			}
			if err := store.AppendResult(meta.JobID, result); err != nil {
				t.Errorf("AppendResult: %v", err)
				return
			}
			if err := store.IncrementCompleted(meta.JobID); err != nil {
				t.Errorf("IncrementCompleted: %v", err)
			}
		}(i)
	}
	wg.Wait()

	got, err := store.GetMeta(meta.JobID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got.CompletedItems != 20 {
		t.Fatalf("completed_items = %d, want 20", got.CompletedItems)
	}

	resultsPath := filepath.Join(store.rootDir, meta.JobID, "results.jsonl")
	lines, err := countLines(resultsPath)
	if err != nil {
		t.Fatalf("count lines: %v", err)
	}
	if lines != 20 {
		t.Fatalf("result lines = %d, want 20", lines)
	}
}

func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
