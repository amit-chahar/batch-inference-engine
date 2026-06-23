# Batch Inference Engine

Production-ready asynchronous batch evaluation service that ingests prompt arrays, fans out inference across a bounded worker pool with exponential back-off, and aggregates results into downloadable reports.

Built in **Go** for the DigitalOcean batch inference interview.

## Quickstart

### Prerequisites

- Go 1.22+
- An inference API key (OpenRouter, Together, Groq, etc.)

### Setup

```bash
cd batch-inference-engine
cp .env.example .env
# Edit .env and set INFERENCE_API_KEY

make build
make run
```

### Health check

```bash
curl http://localhost:8000/health
```

### Submit a batch job (coming soon)

```bash
curl -X POST http://localhost:8000/job/submit \
  -H "Content-Type: application/json" \
  -d '{"input_file": "sample_batch.json"}'
```

## Sample input

`sample_batch.json` contains **1,000** prompt items. Regenerate with:

```bash
make generate-batch
```

## Architecture

See [docs/architecture.md](docs/architecture.md) for the full flow diagram and scaling discussion.

## Testing

```bash
make test
```

## License

MIT
