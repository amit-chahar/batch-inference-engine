package job

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteResultsArrayEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResultsArray(&buf, strings.NewReader("")); err != nil {
		t.Fatalf("WriteResultsArray: %v", err)
	}
	if buf.String() != "[]" {
		t.Fatalf("output = %q, want []", buf.String())
	}

	var results []PromptResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len = %d, want 0", len(results))
	}
}

func TestWriteResultsArrayMultipleLines(t *testing.T) {
	input := strings.Join([]string{
		`{"id":"a","prompt":"one"}`,
		"",
		`{"id":"b","prompt":"two","response":"ok"}`,
	}, "\n")

	var buf bytes.Buffer
	if err := WriteResultsArray(&buf, strings.NewReader(input)); err != nil {
		t.Fatalf("WriteResultsArray: %v", err)
	}

	var results []PromptResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, buf.String())
	}
	if len(results) != 2 {
		t.Fatalf("len = %d, want 2", len(results))
	}
	if results[0].ID != "a" || results[1].ID != "b" {
		t.Fatalf("unexpected ids: %q, %q", results[0].ID, results[1].ID)
	}
}

func TestWriteResultsArraySingleLine(t *testing.T) {
	input := `{"id":"only","prompt":"solo"}`

	var buf bytes.Buffer
	if err := WriteResultsArray(&buf, strings.NewReader(input)); err != nil {
		t.Fatalf("WriteResultsArray: %v", err)
	}

	var results []PromptResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 1 || results[0].ID != "only" {
		t.Fatalf("unexpected result: %+v", results)
	}
}
