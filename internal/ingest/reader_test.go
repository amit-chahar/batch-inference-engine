package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

func writeBatchFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// drainStream reads items and errors concurrently — mirrors production consumer pattern.
func drainStream(itemsCh <-chan job.PromptItem, errsCh <-chan error) ([]job.PromptItem, []error) {
	var items []job.PromptItem
	var errs []error

	itemsOpen := true
	errsOpen := true

	for itemsOpen || errsOpen {
		select {
		case item, ok := <-itemsCh:
			if !ok {
				itemsOpen = false
				itemsCh = nil
				continue
			}
			items = append(items, item)
		case err, ok := <-errsCh:
			if !ok {
				errsOpen = false
				errsCh = nil
				continue
			}
			errs = append(errs, err)
		}
	}

	return items, errs
}

func TestStreamItemsReadsTenLineFixture(t *testing.T) {
	var content string
	for i := range 10 {
		content += fmt.Sprintf(
			`{"id":"prompt-%04d","prompt":"Prompt %04d","metadata":{"index":%d}}`+"\n",
			i, i, i,
		)
	}

	path := writeBatchFile(t, "batch_10.jsonl", content)
	items, errs := drainStream(StreamItems(path))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(items) != 10 {
		t.Fatalf("item count = %d, want 10", len(items))
	}
	if items[0].ID != "prompt-0000" {
		t.Fatalf("first item id = %q", items[0].ID)
	}
	if items[9].Prompt != "Prompt 0009" {
		t.Fatalf("last item prompt = %q", items[9].Prompt)
	}
}

func TestStreamItemsMalformedLineReturnsErrorAndContinues(t *testing.T) {
	// Spec: row-level parse failures must not abort the entire batch scan.
	content := `{"id":"prompt-0000","prompt":"ok"}
not-json
{"id":"prompt-0001","prompt":"also ok"}
`

	path := writeBatchFile(t, "batch_bad_line.jsonl", content)
	items, errs := drainStream(StreamItems(path))

	if len(items) != 2 {
		t.Fatalf("item count = %d, want 2", len(items))
	}
	if len(errs) != 1 {
		t.Fatalf("error count = %d, want 1", len(errs))
	}
	if errs[0].Error() == "" {
		t.Fatal("expected non-empty row error")
	}
}

func TestStreamItemsMissingFile(t *testing.T) {
	items, errs := drainStream(StreamItems(filepath.Join(t.TempDir(), "missing.jsonl")))

	if len(items) != 0 {
		t.Fatalf("item count = %d, want 0", len(items))
	}
	if len(errs) != 1 {
		t.Fatalf("error count = %d, want 1", len(errs))
	}
}

func TestStreamItemsSkipsEmptyLines(t *testing.T) {
	content := `{"id":"prompt-0000","prompt":"first"}

{"id":"prompt-0001","prompt":"second"}
`
	path := writeBatchFile(t, "batch_blank_lines.jsonl", content)
	items, errs := drainStream(StreamItems(path))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(items) != 2 {
		t.Fatalf("item count = %d, want 2", len(items))
	}
}
