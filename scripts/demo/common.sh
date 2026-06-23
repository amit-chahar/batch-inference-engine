#!/usr/bin/env bash
# Shared helpers for live demo scripts. Source this file; do not execute directly.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASE_URL="${BASE_URL:-http://localhost:8080}"
LAST_JOB_FILE="${ROOT}/scripts/demo/.last-job-id"

require_server() {
  if ! curl -sf "${BASE_URL}/health" >/dev/null; then
    echo "error: server not reachable at ${BASE_URL}" >&2
    echo "start it with: scripts/start-server.sh" >&2
    exit 1
  fi
}

save_job_id() {
  printf '%s\n' "$1" >"${LAST_JOB_FILE}"
}

load_job_id() {
  if [[ -n "${1:-}" ]]; then
    printf '%s\n' "$1"
    return
  fi
  if [[ -f "${LAST_JOB_FILE}" ]]; then
    cat "${LAST_JOB_FILE}"
    return
  fi
  echo "error: job id required (or submit a job first)" >&2
  exit 1
}

pretty_json() {
  jq .
}
