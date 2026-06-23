// Command generate_batch writes a JSONL sample input file for local testing and demos.
// Each line is one PromptItem JSON object — matches the interviewer-clarified input format.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

var promptTemplates = []string{
	"Summarize the key benefits of %s in two sentences.",
	"Explain %s to a beginner in plain language.",
	"List three pros and three cons of %s.",
	"Write a one-paragraph overview of %s.",
	"What are common misconceptions about %s?",
}

var topics = []string{
	"renewable energy",
	"machine learning",
	"microservices",
	"distributed systems",
	"cloud computing",
	"REST APIs",
	"batch processing",
	"rate limiting",
	"worker pools",
	"async I/O",
}

type promptItem struct {
	ID       string         `json:"id"`
	Prompt   string         `json:"prompt"`
	Metadata map[string]any `json:"metadata"`
}

func main() {
	count := 1000
	outputPath := "sample_batch.jsonl"

	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid count: %v\n", err)
			os.Exit(1)
		}
		count = n
	}
	if len(os.Args) > 2 {
		outputPath = os.Args[2]
	}

	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create %s: %v\n", outputPath, err)
		os.Exit(1)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for i := range count {
		topic := topics[i%len(topics)]
		template := promptTemplates[i%len(promptTemplates)]
		item := promptItem{
			ID:     fmt.Sprintf("prompt-%04d", i),
			Prompt: fmt.Sprintf(template, topic),
			Metadata: map[string]any{
				"index": i,
				"topic": topic,
			},
		}

		// Compact single-line JSON — required for JSONL ingest via bufio.Scanner.
		line, err := json.Marshal(item)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal item %d: %v\n", i, err)
			os.Exit(1)
		}
		if _, err := writer.Write(line); err != nil {
			fmt.Fprintf(os.Stderr, "write item %d: %v\n", i, err)
			os.Exit(1)
		}
		if err := writer.WriteByte('\n'); err != nil {
			fmt.Fprintf(os.Stderr, "write newline for item %d: %v\n", i, err)
			os.Exit(1)
		}
	}

	if err := writer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "flush %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %d lines to %s\n", count, outputPath)
}
