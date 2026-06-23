package main

import (
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
	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid count: %v\n", err)
			os.Exit(1)
		}
		count = n
	}

	items := make([]promptItem, 0, count)
	for i := range count {
		topic := topics[i%len(topics)]
		template := promptTemplates[i%len(promptTemplates)]
		items = append(items, promptItem{
			ID:     fmt.Sprintf("prompt-%04d", i),
			Prompt: fmt.Sprintf(template, topic),
			Metadata: map[string]any{
				"index": i,
				"topic": topic,
			},
		})
	}

	out, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal batch: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile("sample_batch.json", append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write sample_batch.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %d items to sample_batch.json\n", len(items))
}
