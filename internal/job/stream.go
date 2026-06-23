package job

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// WriteResultsArray streams a JSON array to w by merging JSONL lines from r.
func WriteResultsArray(w io.Writer, r io.Reader) error {
	return writeMergedReaders(w, r)
}

// WriteResultsArrayFromFiles merges multiple JSONL files into one streamed JSON array.
func WriteResultsArrayFromFiles(w io.Writer, paths ...string) error {
	if len(paths) == 0 {
		_, err := io.WriteString(w, "[]")
		return err
	}

	if _, err := io.WriteString(w, "["); err != nil {
		return err
	}

	first := true
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open results file: %w", err)
		}
		first, err = writeJSONLLines(w, file, first)
		file.Close()
		if err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "]"); err != nil {
		return err
	}
	return nil
}

func writeMergedReaders(w io.Writer, readers ...io.Reader) error {
	if len(readers) == 0 {
		_, err := io.WriteString(w, "[]")
		return err
	}
	if _, err := io.WriteString(w, "["); err != nil {
		return err
	}
	first := true
	for _, reader := range readers {
		var err error
		first, err = writeJSONLLines(w, reader, first)
		if err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "]"); err != nil {
		return err
	}
	return nil
}

func writeJSONLLines(w io.Writer, r io.Reader, first bool) (bool, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return first, err
			}
		}
		if _, err := io.WriteString(w, line); err != nil {
			return first, err
		}
		first = false
	}
	if err := scanner.Err(); err != nil {
		return first, fmt.Errorf("read results: %w", err)
	}
	return first, nil
}
