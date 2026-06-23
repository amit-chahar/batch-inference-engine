# Batch Inference Engine — TODO

Go implementation plan for the DigitalOcean interview.  
**Workflow:** complete one small step → `go test ./...` → commit → push → review → next step.

> **Secrets:** Store the DO key in `.env` only (gitignored). Never commit `.env` or paste keys into source code, tests, or README.

---

## Confirmed decisions (from interviewer)

| Decision | Answer |
|----------|--------|
| **Upstream API** | DO Serverless Inference — `POST https://inference.do-ai.run/v1/chat/completions` |
| **Auth** | Interviewer will provide a **Model Access Key** (Bearer token). Use mocks in tests/CI for now; plug in live key for manual demo. |
| **Architecture** | **Build your own** scatter-gather worker pool + backoff. Do **NOT** wrap DO's managed Batch Inference API (`/v1/batches/*`). |
| **Input format** | **JSONL file, exactly 1000 lines** — one JSON object per line. Local workspace file path on submit. |
| **Spec note** | Original prompt mentions `sample_batch.json` as an array; interviewer clarified JSONL. Document this in README so reviewers see the intentional choice. |
| **Testing strategy** | `httptest` mock inference server in CI; live DO key for manual demo only. |

**Example input line:**
```json
{"id":"prompt-0042","prompt":"Explain batch processing to a beginner.","metadata":{"index":42}}
```

---

## Pre-flight (you)

- [x] `gh auth login`
- [x] `go mod init` + GitHub remote pushed
- [x] Receive DO Model Access Key from interviewer (add to `.env` only — **never commit**)
- [x] Clarifying questions answered (see bottom of this file)

---

## Progress summary

| Step | Status | Commit |
|------|--------|--------|
| 1 — Repo bootstrap | ✅ Done | `1ecba51`, CI restored `37d3e3d` |
| 2 — Config package | ✅ Done | `ce8cb42` |
| 3 — HTTP skeleton + health | ✅ Done | `77a6f11` |
| 4 — Sample batch (JSONL) | ✅ Done | `25c6fe7` |
| 5 — Streaming ingest | ✅ Done | `0fb4860` |
| 6 — Disk job store | ✅ Done | `175596b` |
| 7 — Backoff helper | ✅ Done | `83d1f05` |
| 8 — DO inference client | ✅ Done | `d275210` |
| 9 — Bounded worker pool | ✅ Done | `5a361e9` |
| 10 — Background job runner | ✅ Done | `edb4e58` |
| 11–14 — API + E2E | ✅ Done | submit/status/download + httptest E2E |
| 15 — Docs polish | 🟡 Partial | architecture draft; README scaling table TBD |
| 16–17 — Extensions | ⬜ Optional | — |

Also see `DECISIONS.md` for rationale behind each choice.

---

## Core build steps

### 3-hour priority guardrails

- **P0 must ship:** repo/CI, health, submit, background runner, bounded workers, retry/backoff, status, download, README + architecture diagram.
- **P1 should ship:** disk-backed `meta.json`/`results.jsonl`, streaming ingest/download, full E2E test with `httptest`.
- **P2 only if ahead:** DO Spaces chunk upload, webhook callback, extra polish.
- Keep the first reviewable push tiny: scaffolding + CI + a compiling server. Do not start live inference until the HTTP skeleton and job model are committed.
- Treat memory scaling as a first-class requirement: no full result slice in memory; results append to disk and downloads stream from disk.

---

### Step 1 — Repo bootstrap
**Goal:** First reviewable push with minimal scaffolding and CI.

- [x] `.gitignore` (`bin/`, `.env`, `data/jobs/*`)
- [x] `Makefile` (`make test`, `make build`, `make run`)
- [x] `README.md` stub
- [x] Initial commit + push to GitHub
- [x] Re-add `.github/workflows/ci.yml` (`go test ./...`, `go build ./cmd/server`)
- [x] Confirm `go test ./...` and `go build ./cmd/server` pass locally before pushing

**Commit:** `chore: init Go module, CI skeleton, and project scaffolding`  
**Verify:** CI green on GitHub after push.

---

### Step 2 — Config package
**Goal:** All tunables in one place.

- [x] `internal/config/config.go` — load from env
- [x] Update `.env.example` for DO Serverless Inference:
  ```
  DO_MODEL_ACCESS_KEY=
  INFERENCE_API_URL=https://inference.do-ai.run/v1/chat/completions
  INFERENCE_MODEL=llama3.3-70b-instruct
  MAX_WORKERS=10
  CHUNK_SIZE=50
  MAX_RETRIES=5
  INITIAL_BACKOFF_SECONDS=1
  MAX_BACKOFF_SECONDS=60
  JOBS_DIR=data/jobs
  PORT=8080
  ```
- [x] `internal/config/config_test.go`

**Commit:** `feat: add env-based configuration for DO inference and worker pool`  
**Verify:** `go test ./internal/config/...`

---

### Step 3 — HTTP skeleton + health
**Goal:** Server starts; JSON health check.

- [x] `cmd/server/main.go` — basic health (stdlib mux)
- [x] Refactor to `internal/api/router.go` (chi router)
- [x] `internal/api/handlers.go` — `Health` handler
- [x] `internal/job/types.go` — `JobStatus`, `PromptItem`, `PromptResult`, `JobMeta`
- [x] Keep this step compile-only; no background processing yet

**Commit:** `feat: add chi router, health endpoint, and job domain types`  
**Verify:** `curl localhost:8080/health` → `{"status":"ok"}`

---

### Step 4 — Sample batch file
**Goal:** 1,000-line JSONL input in repo (one prompt per line).

- [x] Regenerate as **JSONL** (not JSON array) — exactly **1000 lines**
- [x] `scripts/generate_batch.go` — write one JSON object per line
- [x] `sample_batch.jsonl` (or `sample_batch.json` with JSONL content — confirm filename with interviewer)
- [x] ~~`sample_batch.json` (JSON array)~~ — **replace** with 1000-line JSONL format

**Per-line schema:**
```json
{"id":"prompt-0000","prompt":"Summarize...","metadata":{"index":0}}
```

**Commit:** `feat: add sample_batch.jsonl with 1000 prompt lines and generator script`  
**Verify:** `wc -l sample_batch.jsonl` → `1000`

---

### Step 5 — Streaming ingest reader
**Goal:** Parse JSONL line-by-line without loading full file into memory.

- [x] `internal/ingest/reader.go` — `StreamItems(path) (<-chan PromptItem, <-chan error)`
- [x] `internal/ingest/reader_test.go`
- [x] Technique: `bufio.Scanner` → `json.Unmarshal` each non-empty line
- [x] Skip/malformed lines → record as row error, continue (don't abort job)
- [x] Add a README note: scanner/channel path keeps input memory O(1) for 500K rows

**Commit:** `feat: stream-parse JSONL batch file one line at a time`  
**Verify:** Tests pass with 10-line fixture; bad line returns error but doesn't panic.

---

### Step 6 — Disk job store
**Goal:** Persist job metadata and append-only results.

- [x] `internal/job/store.go`
- [x] `internal/job/store_test.go`
- [x] `data/jobs/.gitkeep`
- [x] Layout per job:
  ```
  data/jobs/{uuid}/
    meta.json       # status, total, completed, failed, created_at
    results.jsonl   # one PromptResult JSON per line
  ```
- [x] API: `CreateJob`, `GetMeta`, `IncrementCompleted`, `IncrementFailed`, `AppendResult`, `SetStatus`

**Commit:** `feat: disk-backed job store with meta.json and results.jsonl`  
**Verify:** Unit tests for create, counters, concurrent appends.

---

### Step 7 — Backoff helper
**Goal:** Pure, testable retry delay logic.

- [x] `internal/worker/backoff.go`
- [x] `internal/worker/backoff_test.go`
- [x] Retry on: 429, 500, 502, 503, 504
- [x] Delay: `min(maxBackoff, initial * 2^attempt) + jitter(0..25%)`
- [x] Honor `Retry-After` header when present

**Commit:** `feat: exponential backoff with jitter for rate-limit retries`  
**Verify:** Table-driven tests for attempt 0/1/2, cap, jitter bounds.

---

### Step 8 — DO inference client
**Goal:** HTTP client calling DO chat completions with retry.

- [x] `internal/worker/inference.go`
- [x] `internal/worker/inference_test.go` (httptest mock — no live key in CI)
- [x] Target: `POST https://inference.do-ai.run/v1/chat/completions`

**Commit:** `feat: DO serverless inference client with retry on 429/5xx`  
**Verify:** Mock — 429 then 200 succeeds; 400 fails immediately; 500 exhausts retries.

---

### Step 9 — Bounded worker pool
**Goal:** Semaphore-limited concurrent inference.

- [x] `internal/worker/pool.go`
- [x] `internal/worker/pool_test.go`
- [x] Pattern: fixed worker goroutines on shared channel (caps concurrency at `MAX_WORKERS`)

**Commit:** `feat: bounded goroutine pool for concurrent inference`  
**Verify:** Test proves concurrency never exceeds `MAX_WORKERS`.

---

### Step 10 — Background job runner
**Goal:** Wire ingest → pool → store; non-blocking from HTTP.

- [x] `internal/runner/runner.go` (avoids job↔ingest import cycle)
- [x] `internal/runner/runner_test.go`
- [x] Flow: `Submit` → return UUID → `go Process(jobID)` → stream → pool → store
- [x] Final status: `completed` | `partial` | `failed`
- [x] Use a bounded item channel between ingest and workers so slow inference cannot grow memory unbounded
- [x] Runner must continue after row-level failures and write failed rows to `results.jsonl`

**Commit:** `feat: background job runner with scatter-gather pipeline`  
**Verify:** Integration test with mock inference processes 5 items end-to-end.

---

### Step 11 — POST /job/submit
**Goal:** Public API to start a batch job.

- [ ] Request: `{"input_file": "sample_batch.jsonl"}`
- [ ] Response: `{"job_id": "...", "status": "pending", "total_items": 1000}`
- [ ] Returns in <100ms; processing runs in background

**Commit:** `feat: POST /job/submit returns job ID and starts background processing`  
**Verify:** curl returns immediately; job dir appears on disk.

---

### Step 12 — GET /job/{id}/status
**Goal:** Poll progress without loading results.

- [ ] Response: `job_id`, `status`, `total_items`, `completed_items`, `failed_items`, `progress_percent`
- [ ] 404 for unknown job ID

**Commit:** `feat: GET /job/{id}/status with progress counters`  
**Verify:** Counters update as job runs.

---

### Step 13 — GET /job/{id}/download
**Goal:** Stream merged JSON array without OOM.

- [x] Write `[`, stream each `results.jsonl` line with commas, write `]`
- [x] Never `json.Marshal` the full result slice
- [x] Optional: 409 if job still running

**Commit:** `feat: GET /job/{id}/download streams merged results`  
**Verify:** Download is valid JSON array.

---

### Step 14 — Full E2E integration test
**Goal:** CI-provable full lifecycle.

- [x] `internal/api/handlers_test.go`
- [x] Flow: submit → poll until done → download → assert result count

**Commit:** `test: end-to-end job lifecycle integration test`  
**Verify:** `go test ./... -race -cover` all green.

---

### Step 15 — Documentation + architecture diagram
**Goal:** Submission-ready docs.

- [x] `docs/architecture.md` (draft exists — update for final design)
- [ ] `README.md` — DO key setup, curl examples, scaling table (1K → 500K)
- [ ] Mermaid diagram: ingestion → scatter → backpressure → gather
- [ ] Explicitly explain JSONL clarification vs original `sample_batch.json` array wording *(partial — see README Sample input section)*
- [ ] Include ceilings: worker count, channel buffer, retry cap, result file size/rotation

**Commit:** `docs: README quickstart and architecture diagram`  
**Verify:** Follow README from clean checkout.

---

## Optional extensions (if ahead of schedule)

Do not start these until Steps 1–15 are pushed and CI is green.

### Step 16 — DO Spaces chunk upload
- [ ] `internal/storage/spaces.go` — S3-compatible upload
- [ ] Env: `SPACES_KEY`, `SPACES_SECRET`, `SPACES_BUCKET`, `SPACES_REGION`

**Commit:** `feat: upload completed chunk results to DO Spaces`

### Step 17 — Webhook callback
- [ ] Accept optional `callback_url` on submit
- [ ] POST JSON payload when job finishes

**Commit:** `feat: optional webhook callback on job completion`

---

## Memory scaling (document in Step 15)

| Scale | Input | Execution | Output |
|-------|-------|-----------|--------|
| 1K | Line-by-line JSONL scan | 10 workers | Single `results.jsonl` |
| 100K | Same line scanner | Same pool | Rotate jsonl every `CHUNK_SIZE` lines |
| 500K | Never load all lines into memory | Bounded goroutines + channel | Stream merge on download |

Peak RAM: **O(MAX_WORKERS × avg_response_size)**, not O(dataset).

---

## Interviewer clarifying questions

| # | Question | Answer |
|---|----------|--------|
| 1 | Should workers call **DO Serverless Inference** (`inference.do-ai.run`)? Will a Model Access Key be provided? | **Yes — DO inference. Key will be provided.** Use mock for dev/CI. |
| 2 | Build our own scatter-gather engine, or wrap DO's **Batch Inference API** (`/v1/batches/*`)? | **Build our own worker pool.** This is the interview exercise. |
| 3 | Are **DO Spaces** + **webhook** extensions required or bonus? | *(still ask if time)* |
| 4 | Which **model** should we use from the DO Model Catalog? | Key received (`doo_v1_*`). Set `INFERENCE_MODEL` to whatever model the key is scoped to — ask interviewer if unsure. |
| 5 | Live inference required for demo, or **mocked tests** sufficient? | **Mocks for tests/CI; live key for manual demo at end.** |
| 6 | Input format fixed as `[{id, prompt, metadata?}]`? Submit takes **local file path**? | **JSONL file — exactly 1000 lines, one prompt record per line.** Local workspace file path on submit. |
| 7 | Final status for partial failures: `partial` or `completed` with per-row errors? | *(still ask — plan uses `partial`)* |

---

## Quick reference — what you build vs what DO provides

| Layer | Owner | Endpoint |
|-------|-------|----------|
| Your REST API | **You build** | `POST /job/submit`, `GET /job/{id}/status`, `GET /job/{id}/download` |
| Live inference | **DO provides** | `POST https://inference.do-ai.run/v1/chat/completions` |
| Managed batch (reference only) | DO product | `POST /v1/batches/files`, `POST /v1/batches`, etc. |

Docs:
- [Serverless Inference](https://docs.digitalocean.com/products/inference/how-to/use-serverless-inference/)
- [Batch Inference API](https://docs.digitalocean.com/reference/api/reference/batch-inference/)

---

## Commit cadence (~time estimates)

| Step | Commit message | ~Time |
|------|----------------|-------|
| 1 | `chore: init Go module, CI skeleton` | 10 min |
| 2 | `feat: env-based configuration` | 10 min |
| 3 | `feat: chi router + health + types` | 15 min |
| 4 | `feat: sample_batch.json + generator` | 10 min |
| 5 | `feat: streaming JSON ingest` | 20 min |
| 6 | `feat: disk job store` | 25 min |
| 7 | `feat: backoff helper` | 15 min |
| 8 | `feat: DO inference client` | 25 min |
| 9 | `feat: bounded worker pool` | 20 min |
| 10 | `feat: background job runner` | 30 min |
| 11 | `feat: POST /job/submit` | 15 min |
| 12 | `feat: GET /job/{id}/status` | 10 min |
| 13 | `feat: GET /job/{id}/download` | 15 min |
| 14 | `test: E2E integration test` | 20 min |
| 15 | `docs: README + architecture` | 20 min |

**Core path (Steps 1–15):** ~4 hours. Optional 16–17 if time permits.
