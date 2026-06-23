package job

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// WriteResultsArray streams a JSON array to w by merging JSONL lines from r.
// Each non-empty line is copied verbatim; nothing is loaded into a slice.
func WriteResultsArray(w io.Writer, r io.Reader) error {
	if _, err := io.WriteString(w, "["); err != nil {
		return err
	}

	scanner := bufio.NewScanner(r)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
		first = false
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read results: %w", err)
	}

	if _, err := io.WriteString(w, "]"); err != nil {
		return err
	}
	return nil
}
