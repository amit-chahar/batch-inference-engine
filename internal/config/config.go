// Package config loads runtime settings from environment variables.
// Defaults target DigitalOcean Serverless Inference; override via .env locally.
package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultInferenceAPIURL = "https://inference.do-ai.run/v1/chat/completions"
	defaultInferenceModel  = "llama3.3-70b-instruct"
	defaultMaxWorkers      = 10
	defaultChunkSize       = 50
	defaultMaxRetries      = 5
	defaultInitialBackoff  = 1.0
	defaultMaxBackoff      = 60.0
	defaultJobsDir         = "data/jobs"
	defaultPort            = 8080
)

// Config holds runtime settings loaded from environment variables.
type Config struct {
	// DOModelAccessKey is the Bearer token for DO Serverless Inference (never commit).
	DOModelAccessKey string
	// InferenceAPIURL is the OpenAI-compatible chat completions endpoint.
	InferenceAPIURL string
	// InferenceModel is the model slug passed in each inference request body.
	InferenceModel string

	// MaxWorkers caps concurrent inference goroutines (primary rate-limit lever).
	MaxWorkers int
	// ChunkSize groups items for future chunk/spill boundaries (Spaces extension).
	ChunkSize int
	// MaxRetries is the per-request retry budget for 429/5xx responses.
	MaxRetries int
	// InitialBackoff is the base delay before the first retry attempt.
	InitialBackoff time.Duration
	// MaxBackoff caps exponential growth and Retry-After delays.
	MaxBackoff time.Duration

	// JobsDir is the on-disk root for meta.json + results.jsonl per job.
	JobsDir string
	// Port is the HTTP listen port for this API server.
	Port int

	// SpacesKey is the DO Spaces access key (optional — enables chunk upload).
	SpacesKey string
	// SpacesSecret is the DO Spaces secret key.
	SpacesSecret string
	// SpacesBucket is the target bucket name.
	SpacesBucket string
	// SpacesRegion is the DO region slug (e.g. nyc3).
	SpacesRegion string
}

// Load reads configuration from the process environment.
// Missing or invalid numeric env vars fall back to safe defaults.
func Load() Config {
	return Config{
		DOModelAccessKey: envString("DO_MODEL_ACCESS_KEY", ""),
		InferenceAPIURL:  envString("INFERENCE_API_URL", defaultInferenceAPIURL),
		InferenceModel:   envString("INFERENCE_MODEL", defaultInferenceModel),
		MaxWorkers:       envInt("MAX_WORKERS", defaultMaxWorkers),
		ChunkSize:        envInt("CHUNK_SIZE", defaultChunkSize),
		MaxRetries:       envInt("MAX_RETRIES", defaultMaxRetries),
		InitialBackoff:   secondsToDuration(envFloat("INITIAL_BACKOFF_SECONDS", defaultInitialBackoff)),
		MaxBackoff:       secondsToDuration(envFloat("MAX_BACKOFF_SECONDS", defaultMaxBackoff)),
		JobsDir:          envString("JOBS_DIR", defaultJobsDir),
		Port:             envInt("PORT", defaultPort),
		SpacesKey:        envString("SPACES_KEY", ""),
		SpacesSecret:     envString("SPACES_SECRET", ""),
		SpacesBucket:     envString("SPACES_BUCKET", ""),
		SpacesRegion:     envString("SPACES_REGION", ""),
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		// Ignore malformed env values so a typo does not crash the server.
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
