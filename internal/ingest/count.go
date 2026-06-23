package ingest

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// CountNonEmptyLines counts non-blank lines in a JSONL input file for job totals.
func CountNonEmptyLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open input file: %w", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read input file: %w", err)
	}
	return count, nil
}
