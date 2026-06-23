package worker

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

func newTestInferenceClient(t *testing.T, handler http.Handler, maxRetries int) *InferenceClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewInferenceClient(InferenceOptions{
		HTTPClient: server.Client(),
		APIURL:     server.URL,
		APIKey:     "test-key",
		Model:      "test-model",
		MaxRetries: maxRetries,
		Backoff:    NewBackoffWithRand(time.Millisecond, 10*time.Millisecond, rand.New(rand.NewSource(1))),
		Sleep:      func(time.Duration) {}, // no waiting in unit tests
	})
}

func testPromptItem() job.PromptItem {
	return job.PromptItem{
		ID:     "prompt-0001",
		Prompt: "Explain batch processing.",
	}
}

func writeChatOK(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(chatResponse{
		Choices: []struct {
			Message chatMessage `json:"message"`
		}{
			{Message: chatMessage{Role: "assistant", Content: content}},
		},
	})
}

func TestCompleteSuccess(t *testing.T) {
	client := newTestInferenceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Fatalf("authorization = %q", auth)
		}
		writeChatOK(w, "batch processing moves work off the hot path")
	}), 2)

	result := client.Complete(context.Background(), testPromptItem())

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", *result.Error)
	}
	if result.Response == nil || *result.Response == "" {
		t.Fatal("expected response text")
	}
}

func TestCompleteRetries429ThenSucceeds(t *testing.T) {
	var attempts atomic.Int32

	client := newTestInferenceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		writeChatOK(w, "ok after retry")
	}), 3)

	result := client.Complete(context.Background(), testPromptItem())

	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", *result.Error)
	}
	if result.Response == nil || *result.Response != "ok after retry" {
		t.Fatalf("response = %v", result.Response)
	}
}

func TestComplete400FailsImmediately(t *testing.T) {
	var attempts atomic.Int32

	client := newTestInferenceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}), 5)

	result := client.Complete(context.Background(), testPromptItem())

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on 400)", attempts.Load())
	}
	if result.Error == nil {
		t.Fatal("expected row error")
	}
	if result.Response != nil {
		t.Fatal("expected no response on permanent failure")
	}
}

func TestComplete500ExhaustsRetries(t *testing.T) {
	var attempts atomic.Int32
	maxRetries := 2

	client := newTestInferenceClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "server error", http.StatusInternalServerError)
	}), maxRetries)

	result := client.Complete(context.Background(), testPromptItem())

	wantAttempts := int32(maxRetries + 1)
	if attempts.Load() != wantAttempts {
		t.Fatalf("attempts = %d, want %d", attempts.Load(), wantAttempts)
	}
	if result.Error == nil {
		t.Fatal("expected row error after retries exhausted")
	}
}
