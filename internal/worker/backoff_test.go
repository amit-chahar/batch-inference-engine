package worker

import (
	"math/rand"
	"net/http"
	"testing"
	"time"
)

func testBackoff(t *testing.T) *Backoff {
	t.Helper()
	return NewBackoffWithRand(time.Second, 60*time.Second, rand.New(rand.NewSource(42)))
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusTooManyRequests, want: true},
		{status: http.StatusInternalServerError, want: true},
		{status: http.StatusBadGateway, want: true},
		{status: http.StatusServiceUnavailable, want: true},
		{status: http.StatusGatewayTimeout, want: true},
		{status: http.StatusBadRequest, want: false},
		{status: http.StatusOK, want: false},
	}

	for _, tc := range tests {
		if got := ShouldRetry(tc.status); got != tc.want {
			t.Fatalf("ShouldRetry(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestDelayAttempts(t *testing.T) {
	b := testBackoff(t)

	tests := []struct {
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{attempt: 0, minDelay: time.Second, maxDelay: 1250 * time.Millisecond},
		{attempt: 1, minDelay: 2 * time.Second, maxDelay: 2500 * time.Millisecond},
		{attempt: 2, minDelay: 4 * time.Second, maxDelay: 5 * time.Second},
	}

	for _, tc := range tests {
		delay := b.Delay(tc.attempt, "", time.Now())
		if delay < tc.minDelay || delay > tc.maxDelay {
			t.Fatalf("attempt %d delay = %v, want between %v and %v", tc.attempt, delay, tc.minDelay, tc.maxDelay)
		}
	}
}

func TestDelayCapsAtMaxBackoff(t *testing.T) {
	b := testBackoff(t)

	delay := b.exponentialDelay(10)
	if delay < 60*time.Second || delay > 75*time.Second {
		t.Fatalf("delay = %v, want between 60s and 75s", delay)
	}
}

func TestDelayHonorsRetryAfterSeconds(t *testing.T) {
	b := testBackoff(t)

	delay := b.Delay(0, "15", time.Now())
	if delay != 15*time.Second {
		t.Fatalf("delay = %v, want 15s", delay)
	}
}

func TestDelayHonorsRetryAfterHTTPDate(t *testing.T) {
	b := testBackoff(t)
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)

	delay := b.Delay(0, now.Add(20*time.Second).Format(http.TimeFormat), now)
	if delay != 20*time.Second {
		t.Fatalf("delay = %v, want 20s", delay)
	}
}

func TestDelayRetryAfterCappedByMax(t *testing.T) {
	b := testBackoff(t)

	delay := b.Delay(0, "120", time.Now())
	if delay != 60*time.Second {
		t.Fatalf("delay = %v, want 60s cap", delay)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	if _, ok := ParseRetryAfter("not-a-date", time.Now()); ok {
		t.Fatal("expected invalid Retry-After to fail parsing")
	}
}
