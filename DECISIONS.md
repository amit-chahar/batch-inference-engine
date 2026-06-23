# Architecture & Design Decisions

Living document for the DigitalOcean batch inference interview.  
Use this when walking the interviewer through the codebase: **what we chose, what we rejected, and why**.

Last updated: after Step 7 (backoff helper). Steps 8–15 are **planned** unless marked **implemented**.

---

## How to use this in the review

1. Start with [Executive summary](#executive-summary) (30 seconds).
2. Walk the [decision table](#decision-log) for anything they ask about.
3. Point to code paths listed under each decision.
4. Be honest about [open questions](#open-questions-for-interviewer) you still want to confirm.

---

## Executive summary

We are building a **custom scatter-gather batch engine in Go** that:

- Accepts a **local JSONL file** (1000 prompt lines),
- Returns a **job ID immediately** and processes in the background,
- Calls **DigitalOcean Serverless Inference** (`inference.do-ai.run`) from a **bounded worker pool**,
- Retries **429 / 5xx** with **custom exponential backoff + jitter**,
- Persists state and results **on disk** (not in memory),
- Exposes **status** and **streaming download** APIs.

We deliberately **do not** wrap DO’s managed Batch Inference API (`/v1/batches/*`) — building the orchestration layer is the exercise.

---

## Decision log

| # | Decision | Choice | Status | Why |
|---|----------|--------|--------|-----|
| D1 | Language | **Go** | Implemented | Strong concurrency (goroutines + channels), stdlib HTTP, fast compile/test loop, good fit for worker pools. Python scaffold was replaced early. |
| D2 | Upstream inference | **DO Serverless Inference** (`POST …/v1/chat/completions`) | Config ready | Interviewer confirmed DO endpoint + Model Access Key. OpenAI-compatible request shape. |
| D3 | Orchestration | **Build our own** worker pool + job lifecycle | Partial | Spec / interviewer: do **not** delegate batch orchestration to DO Batch API. We own submit/status/download. |
| D4 | Input format | **JSONL** (1000 lines, one object per line) | Implemented | Interviewer clarified vs original spec’s JSON array. Enables O(1) memory streaming via `bufio.Scanner`. |
| D5 | Input filename | **`sample_batch.jsonl`** | Implemented | Keeps `.jsonl` extension honest. README notes divergence from spec’s `sample_batch.json` wording. |
| D6 | HTTP router | **chi** (`go-chi/chi/v5`) | Implemented | Lightweight, stdlib-compatible, middleware support. Avoids heavier frameworks under time pressure. |
| D7 | Config loading | **Env vars** via `internal/config` | Implemented | 12-factor style; easy to inject DO key in `.env` without code changes. No flag parsing needed for interview scope. |
| D8 | Secrets | **`DO_MODEL_ACCESS_KEY` in `.env` only** | Implemented | Never committed. Tests/CI use mocks. Live key for manual demo only. |
| D9 | Job persistence | **Disk: `meta.json` + `results.jsonl`** | Implemented | Survives process restarts; avoids holding full result sets in RAM. Matches scaling story for 100K–500K rows. |
| D10 | Job IDs | **`github.com/google/uuid`** | Implemented | Standard, collision-safe IDs without rolling our own. |
| D11 | Ingest parsing | **`bufio.Scanner` + line JSON decode** | Implemented | Simple JSONL reader; constant memory per row. Malformed lines emit errors but don’t abort the file scan. |
| D12 | Backoff | **Custom `internal/worker/backoff`** | Implemented | Interview asks to demonstrate 429 handling. ~100 lines, fully tested, honors `Retry-After`. Libraries (`cenkalti/backoff`, `go-retryablehttp`) exist but custom code shows understanding. **Decision reaffirmed:** keep custom helper. |
| D13 | Retryable HTTP codes | **429, 500, 502, 503, 504** | Implemented | Rate limits + transient upstream failures. **4xx (except 429)** → permanent row failure, no retry. |
| D14 | Backoff formula | `min(max, initial × 2^attempt) + jitter(0–25%)` | Implemented | Spread retries under parallel load; jitter reduces thundering herd across workers. |
| D15 | CI/CD | **GitHub Actions** (`go vet`, `go test -race`, `go build`) | Implemented | Spec requirement. Temporarily removed during billing issue, restored once fixed. |
| D16 | Commit strategy | **Small steps → test → commit → push** | Ongoing | Frequent reviewable diffs; easier to explain timeline to interviewer. |
| D17 | Testing | **`httptest` mock inference in CI** | Implemented (Step 8) | No live API spend in CI; deterministic tests for 429/500/400 paths. |
| D18 | Worker concurrency | **Semaphore / bounded channel (`MAX_WORKERS=10`)** | Planned (Step 9) | Caps parallel DO calls; primary backpressure knob alongside retry backoff. |
| D19 | Chunk size | **`CHUNK_SIZE=50` (config)** | Planned | Logical grouping for future chunk files / Spaces extension; default aligns with TODO plan. |
| D20 | Partial failures | **Job status `partial` + per-row `error` in results** | Planned | Spec requires isolated row failures. Confirm with interviewer (see open questions). |
| D21 | Download | **Stream merge from `results.jsonl`** | Planned | Never `json.Marshal` full result slice — O(1) memory at download time. |
| D22 | DO Spaces extension | **Optional (P2)** | Not started | Spec extension for crash-safe chunk spill; only if core path done early. |
| D23 | Webhook extension | **Optional (P2)** | Not started | Spec extension; defer until P0/P1 complete. |
| D24 | Model name | **`llama3.3-70b-instruct` in `.env.example`** | Config default | Placeholder until key scope confirmed; trivial to change via env. |
| D25 | Code comments | **Package docs + design rationale in code** | Implemented | Helps live code walkthrough with interviewer; see package comments and DECISIONS.md. |

---

## Code commenting approach

Comments focus on **why**, not **what**:

- **Package comments** — purpose of each `internal/*` package
- **Exported symbols** — godoc on public API
- **Non-obvious mechanics** — dual-channel ingest, per-job mutex, atomic meta write, backoff formula
- **Interview hooks** — references to spec requirements (429 retry, JSONL, partial failures)

Avoid restating obvious code (`i++ // increment i`). Update comments when behavior changes.

---

### Platform & interview constraints

| Topic | Decision | Rationale |
|-------|----------|-----------|
| Workspace | All code in `/workspaces/batch-inference-engine` | Mandatory interview environment rule. |
| Submission | Push to personal GitHub repo | Required before timer ends; repo: `github.com/amit-chahar/batch-inference-engine`. |
| AI tooling | Cursor / Copilot allowed; we review all output | Interviewer expects candidate to explain architecture, not just generated code. |

### Why Go over Python?

- Initial scaffold was Python/FastAPI; switched to **Go** before feature work.
- **Why:** native concurrency for scatter-gather, single static binary, race detector in CI, aligns with common DO infra language choices.
- **Tradeoff:** more boilerplate for JSON/API types vs Python; acceptable for performance-critical worker pool.

### Why JSONL over JSON array?

| JSON array | JSONL (chosen) |
|------------|----------------|
| Requires streaming token parser or full-file load | One `Scanner` line at a time |
| Spec text mentioned array | Interviewer clarified JSONL |
| Harder to append partial progress | Natural append-only logs |

**Code:** `internal/ingest/reader.go`, `sample_batch.jsonl`, `scripts/generate_batch.go`

### Why disk-backed job store?

| In-memory job map | Disk store (chosen) |
|-------------------|---------------------|
| Lost on crash | `meta.json` + `results.jsonl` survive |
| OOM at large N | Results streamed/appended |
| Harder to demo scaling story | Matches 500K-row narrative |

**Layout:**
```
data/jobs/{uuid}/
  meta.json       # status, counters, timestamps
  results.jsonl   # one PromptResult per line
```

**Code:** `internal/job/store.go` — per-job mutex for concurrent appends.

### Why custom backoff (not a library)?

Considered: `cenkalti/backoff`, `hashicorp/go-retryablehttp`.

**Kept custom because:**
- Interview explicitly tests rate-limit **backpressure** understanding.
- Easy to unit test attempt 0/1/2, cap, jitter, `Retry-After` in isolation.
- Will be wired in `internal/worker/inference.go` (Step 8) — clear call path for review.

**Code:** `internal/worker/backoff.go` → (planned) `internal/worker/inference.go`

### Why chi router?

- Minimal API surface (`/health` today; `/job/*` coming).
- Standard middleware (request ID, recoverer).
- **Rejected:** gin/echo — heavier; stdlib mux alone — awkward path params for `/job/{id}/status`.

**Code:** `internal/api/router.go`, `internal/api/handlers.go`

### Config defaults (tunable without recompile)

| Variable | Default | Why |
|----------|---------|-----|
| `MAX_WORKERS` | 10 | Balance throughput vs DO rate limits during demo |
| `CHUNK_SIZE` | 50 | Future chunk/spill boundaries |
| `MAX_RETRIES` | 5 | Enough for 429 storms without infinite loops |
| `INITIAL_BACKOFF_SECONDS` | 1 | Fast first retry |
| `MAX_BACKOFF_SECONDS` | 60 | Cap wait per attempt |
| `PORT` | 8080 | Common dev port (health was 8000 in early scaffold — now config-driven) |

**Code:** `internal/config/config.go`, `.env.example`

---

## What we explicitly did NOT do (and why)

| Rejected | Reason |
|----------|--------|
| DO managed Batch API (`/v1/batches/*`) | Interviewer: build orchestration ourselves |
| Load full batch / full results in memory | OOM risk at 500K; spec asks for scaling reasoning |
| Live inference in CI | Cost, flakiness, key exposure — mocks instead |
| Commit `.env` or API keys | Security; `.gitignore` blocks it |
| OpenRouter / third-party inference (as default) | DO interview → DO Serverless Inference endpoint |
| Large framework stack | Time-boxed interview; prefer stdlib + small deps |

---

## Priority tiers (time management)

| Tier | Scope | Rationale |
|------|-------|-----------|
| **P0** | Health, submit, runner, workers, backoff, status, download, README, diagram, CI | Minimum viable submission |
| **P1** | Disk store, streaming ingest/download, E2E httptest | Production credibility |
| **P2** | DO Spaces spill, webhook callback | Spec extensions — only if ahead |

---

## Implementation progress (maps to commits)

| Step | Topic | Commit (on `main`) | Decision refs |
|------|-------|-------------------|---------------|
| 1–2 | Scaffold + config | `1ecba51`, `ce8cb42` | D1, D7, D8 |
| 3 | chi + types | `77a6f11` | D6 |
| 4 | JSONL sample | `25c6fe7` | D4, D5 |
| 5 | Streaming ingest | `0fb4860` | D4, D11 |
| 6 | Disk job store | `175596b` | D9, D10 |
| 7 | Backoff helper | `83d1f05` | D12, D13, D14 |
| 8 | DO inference client | *this commit* | D2, D17 |
| 9–15 | Pool, runner, API, E2E, docs | *planned* | D18–D21 |

---

## Open questions for interviewer

Confirm these if they come up in review — document answers here after the conversation:

| # | Question | Our current assumption |
|---|----------|------------------------|
| 1 | Are DO Spaces + webhook **required** or bonus? | **Bonus (P2)** — ask if time remains |
| 2 | Partial job completion: status `partial` vs `completed` with row errors? | **`partial`** when any row fails |
| 3 | Exact model string for the provided key? | Set `INFERENCE_MODEL` in `.env` to key’s scoped model |
| 4 | Submit body: filename relative to repo root? | **`{"input_file": "sample_batch.jsonl"}`** local path |
| 5 | Download while job still running — allow or 409? | **409 Conflict** (planned) |

---

## Talking points for scaling (500K rows)

Use these if asked about memory — they match our design intent:

1. **Input:** JSONL + line scanner → O(1) memory per row (`internal/ingest`).
2. **Execution:** Bounded worker pool → O(`MAX_WORKERS`) concurrent responses in flight.
3. **Output:** Append-only `results.jsonl` → never materialize full array.
4. **Status:** Counters in `meta.json` only → O(1) metadata.
5. **Download:** Stream lines into JSON array response → merge without loading all results.

---

## References

- [DO Serverless Inference](https://docs.digitalocean.com/products/inference/how-to/use-serverless-inference/)
- [DO Batch Inference API](https://docs.digitalocean.com/reference/api/reference/batch-inference/) (reference only — we don’t wrap it)
- Architecture diagram: `docs/architecture.md`
- Build checklist: `TODO.md`
