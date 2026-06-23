#!/usr/bin/env bash
set -euo pipefail

# Usage: scripts/demo/04-poll.sh [job_id]
# Polls every 2s until status is completed, partial, or failed.

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

JOB_ID="$(load_job_id "${1:-}")"
require_server

echo "polling ${BASE_URL}/job/${JOB_ID}/status (every 2s)"
echo

for i in $(seq 1 120); do
  STATUS_JSON="$(curl -s "${BASE_URL}/job/${JOB_ID}/status")"
  LINE="$(echo "${STATUS_JSON}" | jq -r '[.status, .completed_items, .failed_items, .progress_percent] | @tsv')"
  echo "[${i}] status completed failed progress% — ${LINE//	/ }"

  STATE="$(echo "${STATUS_JSON}" | jq -r .status)"
  case "${STATE}" in
    completed|partial|failed)
      echo
      echo "${STATUS_JSON}" | pretty_json
      echo
      echo "next: scripts/demo/05-download.sh"
      exit 0
      ;;
  esac
  sleep 2
done

echo "error: job did not finish within timeout" >&2
exit 1
