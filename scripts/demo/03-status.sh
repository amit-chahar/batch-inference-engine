#!/usr/bin/env bash
set -euo pipefail

# Usage: scripts/demo/03-status.sh [job_id]

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

JOB_ID="$(load_job_id "${1:-}")"
require_server

echo "GET ${BASE_URL}/job/${JOB_ID}/status"
curl -s "${BASE_URL}/job/${JOB_ID}/status" | pretty_json
