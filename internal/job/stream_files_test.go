package job

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteResultsArrayFromFilesMergesInOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.jsonl")
	second := filepath.Join(dir, "b.jsonl")
	if err := os.WriteFile(first, []byte(`{"id":"1"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte(`{"id":"2"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteResultsArrayFromFiles(&buf, first, second); err != nil {
		t.Fatalf("WriteResultsArrayFromFiles: %v", err)
	}
	if buf.String() != `[{"id":"1"},{"id":"2"}]` {
		t.Fatalf("output = %s", buf.String())
	}
}
