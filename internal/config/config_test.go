package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DO_MODEL_ACCESS_KEY", "")
	t.Setenv("INFERENCE_API_URL", "")
	t.Setenv("INFERENCE_MODEL", "")
	t.Setenv("MAX_WORKERS", "")
	t.Setenv("CHUNK_SIZE", "")
	t.Setenv("MAX_RETRIES", "")
	t.Setenv("INITIAL_BACKOFF_SECONDS", "")
	t.Setenv("MAX_BACKOFF_SECONDS", "")
	t.Setenv("JOBS_DIR", "")
	t.Setenv("PORT", "")

	cfg := Load()

	if cfg.DOModelAccessKey != "" {
		t.Fatalf("DOModelAccessKey = %q, want empty default", cfg.DOModelAccessKey)
	}
	if cfg.InferenceAPIURL != defaultInferenceAPIURL {
		t.Fatalf("InferenceAPIURL = %q, want %q", cfg.InferenceAPIURL, defaultInferenceAPIURL)
	}
	if cfg.InferenceModel != defaultInferenceModel {
		t.Fatalf("InferenceModel = %q, want %q", cfg.InferenceModel, defaultInferenceModel)
	}
	if cfg.MaxWorkers != defaultMaxWorkers {
		t.Fatalf("MaxWorkers = %d, want %d", cfg.MaxWorkers, defaultMaxWorkers)
	}
	if cfg.ChunkSize != defaultChunkSize {
		t.Fatalf("ChunkSize = %d, want %d", cfg.ChunkSize, defaultChunkSize)
	}
	if cfg.MaxRetries != defaultMaxRetries {
		t.Fatalf("MaxRetries = %d, want %d", cfg.MaxRetries, defaultMaxRetries)
	}
	if cfg.InitialBackoff != time.Second {
		t.Fatalf("InitialBackoff = %v, want 1s", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 60*time.Second {
		t.Fatalf("MaxBackoff = %v, want 60s", cfg.MaxBackoff)
	}
	if cfg.JobsDir != defaultJobsDir {
		t.Fatalf("JobsDir = %q, want %q", cfg.JobsDir, defaultJobsDir)
	}
	if cfg.Port != defaultPort {
		t.Fatalf("Port = %d, want %d", cfg.Port, defaultPort)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("DO_MODEL_ACCESS_KEY", "test-key")
	t.Setenv("INFERENCE_API_URL", "https://example.com/v1/chat/completions")
	t.Setenv("INFERENCE_MODEL", "custom-model")
	t.Setenv("MAX_WORKERS", "4")
	t.Setenv("CHUNK_SIZE", "25")
	t.Setenv("MAX_RETRIES", "3")
	t.Setenv("INITIAL_BACKOFF_SECONDS", "2.5")
	t.Setenv("MAX_BACKOFF_SECONDS", "30")
	t.Setenv("JOBS_DIR", "/tmp/jobs")
	t.Setenv("PORT", "9000")

	cfg := Load()

	if cfg.DOModelAccessKey != "test-key" {
		t.Fatalf("DOModelAccessKey = %q, want test-key", cfg.DOModelAccessKey)
	}
	if cfg.InferenceAPIURL != "https://example.com/v1/chat/completions" {
		t.Fatalf("InferenceAPIURL = %q", cfg.InferenceAPIURL)
	}
	if cfg.InferenceModel != "custom-model" {
		t.Fatalf("InferenceModel = %q", cfg.InferenceModel)
	}
	if cfg.MaxWorkers != 4 {
		t.Fatalf("MaxWorkers = %d, want 4", cfg.MaxWorkers)
	}
	if cfg.ChunkSize != 25 {
		t.Fatalf("ChunkSize = %d, want 25", cfg.ChunkSize)
	}
	if cfg.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.InitialBackoff != 2500*time.Millisecond {
		t.Fatalf("InitialBackoff = %v, want 2.5s", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 30*time.Second {
		t.Fatalf("MaxBackoff = %v, want 30s", cfg.MaxBackoff)
	}
	if cfg.JobsDir != "/tmp/jobs" {
		t.Fatalf("JobsDir = %q", cfg.JobsDir)
	}
	if cfg.Port != 9000 {
		t.Fatalf("Port = %d, want 9000", cfg.Port)
	}
}

func TestLoadInvalidIntFallsBackToDefault(t *testing.T) {
	t.Setenv("MAX_WORKERS", "not-a-number")

	cfg := Load()

	if cfg.MaxWorkers != defaultMaxWorkers {
		t.Fatalf("MaxWorkers = %d, want default %d", cfg.MaxWorkers, defaultMaxWorkers)
	}
}
