package job

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendResultWithChunkingSealsLocalChunk(t *testing.T) {
	store := newTestStore(t)
	meta, err := store.CreateJob(5)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	const chunkSize = 2
	for i := range 4 {
		result := PromptResult{ID: fmt.Sprintf("prompt-%d", i), Prompt: "p"}
		sealed, err := store.AppendResultWithChunking(meta.JobID, result, chunkSize)
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if i == 1 || i == 3 {
			if sealed == nil {
				t.Fatalf("expected sealed chunk at append %d", i)
			}
		} else if sealed != nil {
			t.Fatalf("unexpected sealed chunk at append %d", i)
		}
	}

	chunk0 := filepath.Join(store.rootDir, meta.JobID, "chunks", "chunk_0.jsonl")
	chunk1 := filepath.Join(store.rootDir, meta.JobID, "chunks", "chunk_1.jsonl")
	for _, path := range []string{chunk0, chunk1} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("chunk missing %s: %v", path, err)
		}
	}

	var buf bytes.Buffer
	if err := store.StreamResults(meta.JobID, &buf); err != nil {
		t.Fatalf("StreamResults: %v", err)
	}
	var results []PromptResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("decode merged results: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("merged count = %d, want 4", len(results))
	}
}

func TestCreateJobStoresCallbackURL(t *testing.T) {
	store := newTestStore(t)
	meta, err := store.CreateJobWithOptions(JobCreateOptions{
		TotalItems:  1,
		CallbackURL: "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("CreateJobWithOptions: %v", err)
	}
	if meta.CallbackURL != "https://example.com/hook" {
		t.Fatalf("callback = %q", meta.CallbackURL)
	}
}
