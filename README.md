# Batch Inference Engine

Production-ready asynchronous batch evaluation service that ingests prompt arrays, fans out inference across a bounded worker pool with exponential back-off, and aggregates results into downloadable reports.

Built in **Go** for the DigitalOcean batch inference interview.

## Quickstart

### Prerequisites

- Go 1.22+
- DigitalOcean Model Access Key (or any OpenAI-compatible inference key)

### Setup

```bash
cd batch-inference-engine
cp .env.example .env
# Edit .env and set DO_MODEL_ACCESS_KEY

make build
make run
```

### Health check

```bash
curl http://localhost:8080/health
```

### Submit a batch job

```bash
curl -X POST http://localhost:8080/job/submit \
  -H "Content-Type: application/json" \
  -d '{"input_file": "sample_batch.jsonl"}'
```

### Check status

```bash
curl http://localhost:8080/job/{job_id}/status
```

### Download results (JSONL today; JSON array merge in Step 13)

```bash
curl http://localhost:8080/job/{job_id}/download -o results.jsonl
```

## Sample input

`sample_batch.jsonl` contains **1,000** prompt lines (JSONL). Regenerate with:

```bash
make generate-batch
```

**Format note:** The original interview prompt mentions a JSON array (`sample_batch.json`); the interviewer clarified **JSONL** (one JSON object per line) for streaming ingest. Input is parsed with `bufio.Scanner` + line decode, so memory stays **O(1)** relative to file size — important when scaling to 500K rows.

## Architecture

See [docs/architecture.md](docs/architecture.md) for the full flow diagram and scaling discussion.

## Testing

```bash
make test
```

## License

MIT
