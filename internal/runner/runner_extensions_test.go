package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
	"github.com/amit-chahar/batch-inference-engine/internal/storage"
	"github.com/amit-chahar/batch-inference-engine/internal/webhook"
)

func TestRunnerUploadsChunksAndCallsWebhook(t *testing.T) {
	var webhookCalls atomic.Int32
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(callbackServer.Close)

	store := job.NewStore(t.TempDir())
	uploader := &storage.RecordingUploader{}
	run := NewWithOptions(Options{
		Store:       store,
		Completer:   stubCompleter{},
		MaxWorkers:  1,
		ChannelSize: 2,
		ChunkSize:   2,
		Uploader:    uploader,
		Notifier:    webhook.NewNotifier(),
	})

	inputPath := writeFixture(t,
		`{"id":"prompt-0000","prompt":"one"}`,
		`{"id":"prompt-0001","prompt":"two"}`,
		`{"id":"prompt-0002","prompt":"three"}`,
	)

	meta, err := store.CreateJobWithOptions(job.JobCreateOptions{
		TotalItems:  3,
		CallbackURL: callbackServer.URL,
	})
	if err != nil {
		t.Fatalf("CreateJobWithOptions: %v", err)
	}

	if err := run.Process(context.Background(), meta.JobID, inputPath); err != nil {
		t.Fatalf("Process: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for webhookCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if webhookCalls.Load() != 1 {
		t.Fatalf("webhook calls = %d, want 1", webhookCalls.Load())
	}
	if len(uploader.Uploads) != 2 {
		t.Fatalf("uploads = %d, want 2 (one sealed chunk + final chunk)", len(uploader.Uploads))
	}

	got, err := store.GetMeta(meta.JobID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if len(got.ChunkKeys) != 2 {
		t.Fatalf("chunk keys = %v", got.ChunkKeys)
	}
}
