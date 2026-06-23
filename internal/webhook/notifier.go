package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

const defaultWebhookTimeout = 10 * time.Second

// Notifier POSTs job completion payloads to optional callback URLs.
type Notifier struct {
	client *http.Client
}

// NewNotifier constructs a webhook client with a bounded HTTP timeout.
func NewNotifier() *Notifier {
	return &Notifier{client: &http.Client{Timeout: defaultWebhookTimeout}}
}

// CompletionPayload is the JSON body sent to callback_url when a job finishes.
type CompletionPayload struct {
	JobID          string         `json:"job_id"`
	Status         job.JobStatus  `json:"status"`
	TotalItems     int            `json:"total_items"`
	CompletedItems int            `json:"completed_items"`
	FailedItems    int            `json:"failed_items"`
	ChunkKeys      []string       `json:"chunk_keys,omitempty"`
}

// Notify POSTs the payload to callbackURL. Errors are returned to the caller.
func (n *Notifier) Notify(ctx context.Context, callbackURL string, payload CompletionPayload) error {
	if callbackURL == "" {
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "batch-inference-engine/0.1.0")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

// PayloadFromMeta builds a completion payload from persisted job metadata.
func PayloadFromMeta(meta job.JobMeta) CompletionPayload {
	return CompletionPayload{
		JobID:          meta.JobID,
		Status:         meta.Status,
		TotalItems:     meta.TotalItems,
		CompletedItems: meta.CompletedItems,
		FailedItems:    meta.FailedItems,
		ChunkKeys:      meta.ChunkKeys,
	}
}
