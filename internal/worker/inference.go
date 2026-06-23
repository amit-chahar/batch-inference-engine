package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/config"
	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

const defaultInferenceTimeout = 30 * time.Second

// InferenceClient calls DigitalOcean Serverless Inference (OpenAI-compatible chat API).
// Retries transient 429/5xx responses using internal/worker/backoff.
type InferenceClient struct {
	httpClient *http.Client
	apiURL     string
	apiKey     string
	model      string
	maxRetries int
	backoff    *Backoff
	sleep      func(time.Duration)
}

// InferenceOptions configures a custom inference client (used in tests and production).
type InferenceOptions struct {
	HTTPClient *http.Client
	APIURL     string
	APIKey     string
	Model      string
	MaxRetries int
	Backoff    *Backoff
	Sleep      func(time.Duration)
}

// NewInferenceClientFromConfig builds a client from loaded env config.
func NewInferenceClientFromConfig(cfg config.Config) *InferenceClient {
	httpClient := &http.Client{Timeout: defaultInferenceTimeout}
	return NewInferenceClient(InferenceOptions{
		HTTPClient: httpClient,
		APIURL:     cfg.InferenceAPIURL,
		APIKey:     cfg.DOModelAccessKey,
		Model:      cfg.InferenceModel,
		MaxRetries: cfg.MaxRetries,
		Backoff:    NewBackoff(cfg.InitialBackoff, cfg.MaxBackoff),
		Sleep:      time.Sleep,
	})
}

// NewInferenceClient constructs a client with explicit options.
func NewInferenceClient(opts InferenceOptions) *InferenceClient {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultInferenceTimeout}
	}

	backoff := opts.Backoff
	if backoff == nil {
		backoff = NewBackoff(time.Second, 60*time.Second)
	}

	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	return &InferenceClient{
		httpClient: httpClient,
		apiURL:     opts.APIURL,
		apiKey:     opts.APIKey,
		model:      opts.Model,
		maxRetries: opts.MaxRetries,
		backoff:    backoff,
		sleep:      sleep,
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete runs one chat completion for a prompt row.
// Row-level upstream failures are returned in PromptResult.Error (job continues).
func (c *InferenceClient) Complete(ctx context.Context, item job.PromptItem) job.PromptResult {
	result := job.PromptResult{
		ID:       item.ID,
		Prompt:   item.Prompt,
		Metadata: item.Metadata,
	}

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		statusCode, responseText, retryAfter, err := c.doCompletion(ctx, item.Prompt)
		if err != nil {
			if attempt == c.maxRetries {
				result.Error = stringPtr(err.Error())
				return result
			}
			c.sleep(c.backoff.Delay(attempt, retryAfter, time.Now()))
			continue
		}

		if statusCode == http.StatusOK {
			result.Response = stringPtr(responseText)
			return result
		}

		if ShouldRetry(statusCode) {
			if attempt == c.maxRetries {
				result.Error = stringPtr(fmt.Sprintf("upstream status %d after %d retries", statusCode, c.maxRetries))
				return result
			}
			c.sleep(c.backoff.Delay(attempt, retryAfter, time.Now()))
			continue
		}

		// Permanent client/upstream error (400, 401, etc.) — no retry.
		result.Error = stringPtr(fmt.Sprintf("upstream status %d: %s", statusCode, responseText))
		return result
	}

	result.Error = stringPtr("inference failed after retries")
	return result
}

func (c *InferenceClient) doCompletion(ctx context.Context, prompt string) (int, string, string, error) {
	payload, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return 0, "", "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
	if err != nil {
		return 0, "", "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", "", fmt.Errorf("call inference API: %w", err)
	}
	defer resp.Body.Close()

	retryAfter := resp.Header.Get("Retry-After")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", retryAfter, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, strings.TrimSpace(string(body)), retryAfter, nil
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return resp.StatusCode, "", retryAfter, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return resp.StatusCode, "", retryAfter, fmt.Errorf("empty choices in response")
	}

	return resp.StatusCode, parsed.Choices[0].Message.Content, retryAfter, nil
}

func stringPtr(value string) *string {
	return &value
}
