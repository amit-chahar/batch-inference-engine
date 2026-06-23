package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

func TestNotifyPostsCompletionPayload(t *testing.T) {
	var received CompletionPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	notifier := NewNotifier()
	meta := job.JobMeta{
		JobID:          "job-1",
		Status:         job.JobStatusCompleted,
		TotalItems:     3,
		CompletedItems: 3,
		ChunkKeys:      []string{"https://example.com/chunk_0.jsonl"},
	}

	if err := notifier.Notify(context.Background(), server.URL, PayloadFromMeta(meta)); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if received.JobID != "job-1" || received.Status != job.JobStatusCompleted {
		t.Fatalf("unexpected payload: %+v", received)
	}
	if len(received.ChunkKeys) != 1 {
		t.Fatalf("chunk keys = %v", received.ChunkKeys)
	}
}

func TestNotifySkipsEmptyURL(t *testing.T) {
	notifier := NewNotifier()
	if err := notifier.Notify(context.Background(), "", CompletionPayload{}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
}
