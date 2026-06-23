# Batch Inference Engine

Asynchronous batch inference service in **Go**: ingest a JSONL prompt file, fan out to DigitalOcean Serverless Inference through a bounded worker pool with exponential backoff, persist results on disk, and expose status + streaming download APIs.

Built for the DigitalOcean batch inference interview. Design rationale lives in [`DECISIONS.md`](DECISIONS.md).

## Quickstart

### Prerequisites

- Go 1.22+
- A [DigitalOcean Model Access Key](https://docs.digitalocean.com/products/inference/how-to/use-serverless-inference/) (`doo_v1_…`)

### 1. Configure inference credentials

```bash
git clone https://github.com/amit-chahar/batch-inference-engine.git
cd batch-inference-engine
cp .env.example .env
```

Edit `.env` and set your key:

```bash
DO_MODEL_ACCESS_KEY=doo_v1_your_key_here
# Optional: match the model your key is scoped to
INFERENCE_MODEL=llama3.3-70b-instruct
```

The server reads **process environment variables** (`internal/config`). Load `.env` before starting:

```bash
set -a && source .env && set +a && make run
```

Or export directly:

```bash
export DO_MODEL_ACCESS_KEY=doo_v1_your_key_here
export INFERENCE_API_URL=https://inference.do-ai.run/v1/chat/completions
```

> **CI / unit tests** use an `httptest` mock inference server — no live key required for `make test`.

### 2. Build and run

```bash
make build
make run
# listens on http://localhost:8080
```

### 3. End-to-end curl workflow

**Health**

```bash
curl -s http://localhost:8080/health | jq
```

**Submit** (returns immediately with `202 Accepted`)

```bash
JOB=$(curl -s -X POST http://localhost:8080/job/submit \
  -H "Content-Type: application/json" \
  -d '{"input_file":"sample_batch.jsonl","callback_url":"https://example.com/hook"}' | jq -r .job_id)
echo "job_id=$JOB"
```

**Poll status** until `completed`, `partial`, or `failed`

```bash
curl -s "http://localhost:8080/job/$JOB/status" | jq
# progress_percent reaches 100 when all rows are processed
```

**Download** (streamed JSON array — not JSONL)

```bash
curl -s "http://localhost:8080/job/$JOB/download" -o results.json
jq length results.json   # should match total_items from status
```

Download returns **409 Conflict** while the job is still `pending` or `running`.

## Live demo scripts

Runnable shell scripts for interview walkthrough (requires server + `.env` key):

**Terminal 1 — start server**

```bash
scripts/start-server.sh
```

**Terminal 2 — step-by-step**

```bash
scripts/demo/01-health.sh
scripts/demo/02-submit.sh              # default: demo_live.jsonl (3 prompts)
scripts/demo/03-status.sh              # uses last job id
scripts/demo/04-poll.sh                  # poll until completed/partial/failed
scripts/demo/05-download.sh              # writes demo_results.json
```

**Or run the small batch end-to-end**

```bash
scripts/demo/run-small.sh
```

**Full 1K batch** (long-running — 30–60+ min):

```bash
scripts/demo/run-full.sh
scripts/demo/04-poll.sh
scripts/demo/05-download.sh "" full_results.json
```

Submit saves the latest `job_id` to `scripts/demo/.last-job-id` so status/poll/download work without passing the id each time.

Override server URL: `BASE_URL=http://localhost:8080 scripts/demo/01-health.sh`

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness + version |
| `POST` | `/job/submit` | Body: `{"input_file":"<local path>","callback_url":"<optional https URL>"}` → `job_id`, `status`, `total_items` |
| `GET` | `/job/{id}/status` | Progress counters + `progress_percent` |
| `GET` | `/job/{id}/download` | Streamed JSON array of per-row results |

Submit expects a **local filesystem path** to a JSONL file (not an upload body).

## Sample input

`sample_batch.jsonl` ships with **1,000** prompt lines. Regenerate:

```bash
make generate-batch
```

Each line is one JSON object:

```json
{"id":"prompt-0000","prompt":"Explain batch processing.","metadata":{"topic":"systems"}}
```

### JSONL vs JSON array (interview clarification)

The original spec text references `sample_batch.json` as a **JSON array** of prompts. The interviewer clarified **JSONL** (one object per line) so ingest can stream with `bufio.Scanner` — **O(1) memory** relative to file size. Output download is a **JSON array** merged lazily from append-only `results.jsonl` on disk.

## Configuration

All tunables are env-driven (`internal/config`). See `.env.example`.

| Variable | Default | Purpose |
|----------|---------|---------|
| `DO_MODEL_ACCESS_KEY` | *(required for live inference)* | Bearer token for DO Serverless Inference |
| `INFERENCE_API_URL` | `https://inference.do-ai.run/v1/chat/completions` | OpenAI-compatible chat endpoint |
| `INFERENCE_MODEL` | `llama3.3-70b-instruct` | Model slug in each request |
| `MAX_WORKERS` | `10` | Max concurrent DO inference calls **process-wide** (global limiter) |
| `MAX_RETRIES` | `5` | Per-row retry budget on 429/500/502/503/504 |
| `INITIAL_BACKOFF_SECONDS` | `1` | Base retry delay |
| `MAX_BACKOFF_SECONDS` | `60` | Cap on exponential backoff + `Retry-After` |
| `CHUNK_SIZE` | `50` | Seal local `chunks/chunk_N.jsonl` every N results; upload when Spaces configured |
| `JOBS_DIR` | `data/jobs` | On-disk job root (`meta.json` + `results.jsonl`) |
| `PORT` | `8080` | HTTP listen port |
| `SPACES_KEY` | *(optional)* | DO Spaces access key — enables chunk upload |
| `SPACES_SECRET` | *(optional)* | DO Spaces secret key |
| `SPACES_BUCKET` | *(optional)* | Target bucket name |
| `SPACES_REGION` | `nyc3` | DO region slug |

## Operational ceilings

| Knob | Value | Where enforced |
|------|-------|----------------|
| Concurrent inference calls | `MAX_WORKERS` (default 10) | `internal/worker/limiter.go` wraps shared inference client |
| Per-job worker goroutines | `MAX_WORKERS` per active job | `internal/worker/pool.go` |
| Ingest → worker queue depth | `MAX_WORKERS × 2` | `internal/runner` bounded channel |
| Retries per prompt row | `MAX_RETRIES + 1` attempts | `internal/worker/inference.go` |
| Retryable HTTP codes | 429, 500, 502, 503, 504 | `internal/worker/backoff.go` |
| HTTP client timeout | 30s per inference call | `internal/worker/inference.go` |
| Result storage | Active `results.jsonl` plus sealed `chunks/chunk_N.jsonl` | `internal/job/store.go` |
| Download merge | Stream line-by-line → JSON array | `internal/job/stream.go` |

Peak RAM stays **O(MAX_WORKERS × avg_response_size)**, not O(dataset size).

## Scaling (1K → 500K rows)

| Scale | Input | Execution | Output |
|-------|-------|-----------|--------|
| **1K** (sample) | Line-by-line JSONL scan | 10 workers, channel buffer 20 | Active `results.jsonl`; download streams JSON array |
| **100K** | Same scanner, constant memory | Same bounded pool | Rotate at `CHUNK_SIZE`; upload sealed chunks to Spaces |
| **500K** | Never load full file | DO calls capped by `MAX_WORKERS`; Phase 2 global queue sketched for many jobs | Stream merge on download — never `json.Marshal` all results |

Tune `MAX_WORKERS` down if upstream returns 429; backoff + jitter handle transient pressure.

## Architecture

See [docs/architecture.md](docs/architecture.md) for the Mermaid flow diagram, component map, and backpressure details.

**What you build vs what DO provides**

| Layer | Owner | Endpoint |
|-------|-------|----------|
| Batch REST API | This repo | `POST /job/submit`, `GET /job/{id}/status`, `GET /job/{id}/download` |
| Live inference | DigitalOcean | `POST https://inference.do-ai.run/v1/chat/completions` |
| Managed batch API | DO product (reference only) | `/v1/batches/*` — **not used**; we own orchestration |

## Testing

```bash
make test
# equivalent: go test ./... -race -cover
```

Includes an end-to-end HTTP test (`internal/api/handlers_test.go`) that runs submit → poll → download against an `httptest` mock inference server.

## License

MIT
