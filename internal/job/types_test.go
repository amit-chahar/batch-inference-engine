package job

import "testing"

func TestJobMetaProgressPercent(t *testing.T) {
	meta := JobMeta{
		TotalItems:     1000,
		CompletedItems: 250,
		FailedItems:    50,
	}

	got := meta.ProgressPercent()
	want := 30.0
	if got != want {
		t.Fatalf("ProgressPercent() = %v, want %v", got, want)
	}
}

func TestJobMetaProgressPercentZeroTotal(t *testing.T) {
	meta := JobMeta{TotalItems: 0}

	if got := meta.ProgressPercent(); got != 0 {
		t.Fatalf("ProgressPercent() = %v, want 0", got)
	}
}
