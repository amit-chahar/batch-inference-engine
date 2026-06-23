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
	DOModelAccessKey string
	InferenceAPIURL  string
	InferenceModel   string
	MaxWorkers       int
	ChunkSize        int
	MaxRetries       int
	InitialBackoff   time.Duration
	MaxBackoff       time.Duration
	JobsDir          string
	Port             int
}

// Load reads configuration from the process environment.
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
