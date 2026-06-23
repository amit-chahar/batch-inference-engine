// Package worker runs concurrent inference calls with rate-limit aware retries.
// Backoff logic lives here; the inference HTTP client (Step 8) will call into it.
package worker

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// retryableStatusCodes are retried with exponential backoff (transient upstream pressure).
var retryableStatusCodes = map[int]struct{}{
	http.StatusTooManyRequests:    {}, // 429 — primary interview scenario
	http.StatusBadGateway:         {}, // 502
	http.StatusServiceUnavailable: {}, // 503
	http.StatusGatewayTimeout:     {}, // 504
}

// Backoff computes retry delays with exponential growth and jitter.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
	rng     *rand.Rand
}

// NewBackoff creates a backoff calculator with a random jitter source.
func NewBackoff(initial, max time.Duration) *Backoff {
	return NewBackoffWithRand(initial, max, rand.New(rand.NewSource(time.Now().UnixNano())))
}

// NewBackoffWithRand creates a backoff calculator using the provided random source.
// Inject a seeded RNG in tests for deterministic jitter assertions.
func NewBackoffWithRand(initial, max time.Duration, rng *rand.Rand) *Backoff {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Backoff{
		Initial: initial,
		Max:     max,
		rng:     rng,
	}
}

// ShouldRetry reports whether an HTTP status code should be retried.
// Non-retryable 4xx (400, 401, etc.) become permanent row failures in the inference client.
func ShouldRetry(statusCode int) bool {
	_, ok := retryableStatusCodes[statusCode]
	return ok
}

// Delay returns the wait duration before the next retry attempt.
// When retryAfter is present and valid, it takes precedence (capped at Max).
func (b *Backoff) Delay(attempt int, retryAfter string, now time.Time) time.Duration {
	if delay, ok := ParseRetryAfter(retryAfter, now); ok {
		return capDuration(delay, b.Max)
	}
	return b.exponentialDelay(attempt)
}

// exponentialDelay implements: min(max, initial * 2^attempt) + jitter(0..25%).
func (b *Backoff) exponentialDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	base := b.Initial
	if attempt > 0 {
		multiplier := math.Pow(2, float64(attempt))
		base = time.Duration(float64(b.Initial) * multiplier)
	}
	base = capDuration(base, b.Max)

	// Jitter spreads retries when many workers hit 429 simultaneously.
	jitter := time.Duration(b.rng.Float64() * float64(base) * 0.25)
	return base + jitter
}

// ParseRetryAfter parses the Retry-After header as seconds or an HTTP date.
func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}

	delay := retryAt.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

func capDuration(value, max time.Duration) time.Duration {
	if value > max {
		return max
	}
	return value
}
