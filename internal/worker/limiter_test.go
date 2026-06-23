package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

func TestConcurrencyLimiterCapsParallelAcquires(t *testing.T) {
	limiter := NewConcurrencyLimiter(2)
	var inFlight, peak int
	var mu sync.Mutex

	release := make(chan struct{})
	for range 4 {
		go func() {
			if err := limiter.Acquire(context.Background()); err != nil {
				return
			}
			mu.Lock()
			inFlight++
			if inFlight > peak {
				peak = inFlight
			}
			mu.Unlock()

			<-release
			mu.Lock()
			inFlight--
			mu.Unlock()
			limiter.Release()
		}()
	}

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		currentPeak := peak
		mu.Unlock()
		if currentPeak >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("peak concurrency = %d, want >= 2", currentPeak)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if peak > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", peak)
	}
	close(release)
}

func TestLimitedCompleterCapsCallsAcrossPools(t *testing.T) {
	const globalMax = 3
	const poolWorkers = 5
	const itemsPerPool = 8

	inner := &slowCompleter{delay: 20 * time.Millisecond}
	limiter := NewConcurrencyLimiter(globalMax)
	completer := NewLimitedCompleter(inner, limiter)

	runPool := func(prefix string) {
		pool := NewPool(poolWorkers, completer)
		items := make(chan job.PromptItem, itemsPerPool)
		for i := range itemsPerPool {
			items <- job.PromptItem{
				ID:     fmt.Sprintf("%s-%02d", prefix, i),
				Prompt: "test",
			}
		}
		close(items)
		for range pool.ProcessItems(context.Background(), items) {
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		runPool("job-a")
	}()
	go func() {
		defer wg.Done()
		runPool("job-b")
	}()
	wg.Wait()

	if peak := inner.peakConcurrency(); peak > globalMax {
		t.Fatalf("peak concurrency = %d, want <= %d across parallel jobs", peak, globalMax)
	}
}

func TestNewLimitedCompleterNilLimiterPassthrough(t *testing.T) {
	var calls int
	inner := completerFunc(func(ctx context.Context, item job.PromptItem) job.PromptResult {
		calls++
		response := "ok"
		return job.PromptResult{ID: item.ID, Response: &response}
	})

	completer := NewLimitedCompleter(inner, nil)
	_ = completer.Complete(context.Background(), job.PromptItem{ID: "x"})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
