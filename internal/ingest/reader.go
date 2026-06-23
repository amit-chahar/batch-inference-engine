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
// Callers must read from both channels concurrently to avoid deadlocks.
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
				continue
			}

			var item job.PromptItem
			if err := json.Unmarshal([]byte(line), &item); err != nil {
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
