// Package ingest streams batch input files without loading the full dataset into memory.
// JSONL (one JSON object per line) keeps memory O(1) relative to file size.
package ingest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/amit-chahar/batch-inference-engine/internal/job"
)

// StreamItems reads a JSONL batch file one line at a time.
// Valid rows are sent on the items channel; malformed rows emit errors on the
// errors channel and scanning continues. Both channels are closed when done.
//
// Callers must read from both channels concurrently to avoid deadlocks when
// a malformed line appears between valid rows (producer may block on errs send).
func StreamItems(path string) (<-chan job.PromptItem, <-chan error) {
	items := make(chan job.PromptItem)
	errs := make(chan error)

	go func() {
		defer close(items)
		defer close(errs)

		file, err := os.Open(path)
		if err != nil {
			errs <- fmt.Errorf("open batch file: %w", err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue // tolerate trailing blank lines in fixtures
			}

			var item job.PromptItem
			if err := json.Unmarshal([]byte(line), &item); err != nil {
				// Row-level failure: report and continue (spec: don't abort whole job).
				errs <- fmt.Errorf("line %d: invalid JSON: %w", lineNumber, err)
				continue
			}
			if item.ID == "" || item.Prompt == "" {
				errs <- fmt.Errorf("line %d: missing required id or prompt", lineNumber)
				continue
			}

			items <- item
		}

		if err := scanner.Err(); err != nil {
			errs <- fmt.Errorf("read batch file: %w", err)
		}
	}()

	return items, errs
}
