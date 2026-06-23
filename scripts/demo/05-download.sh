#!/usr/bin/env bash
set -euo pipefail

# Usage: scripts/demo/05-download.sh [job_id] [output_file]
# Default output: demo_results.json

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

JOB_ID="$(load_job_id "${1:-}")"
OUTPUT="${2:-demo_results.json}"
if [[ "${OUTPUT}" != /* ]]; then
  OUTPUT="${ROOT}/${OUTPUT}"
fi

require_server

echo "GET ${BASE_URL}/job/${JOB_ID}/download"
echo "writing ${OUTPUT}"
echo

HTTP_CODE="$(curl -s -w '%{http_code}' -o "${OUTPUT}" "${BASE_URL}/job/${JOB_ID}/download")"
if [[ "${HTTP_CODE}" != "200" ]]; then
  echo "error: download returned HTTP ${HTTP_CODE}" >&2
  cat "${OUTPUT}" >&2
  exit 1
fi

COUNT="$(jq length "${OUTPUT}")"
echo "downloaded ${COUNT} results"
echo
echo "preview:"
jq -r '.[] | "- \(.id): \((.response // .error // "no response") | .[0:120])"' "${OUTPUT}"
