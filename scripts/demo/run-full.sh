#!/usr/bin/env bash
set -euo pipefail

# Submit the full 1,000-line sample batch. Expect 30–60+ minutes.
# Run the server first: scripts/start-server.sh

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

DEMO_DIR="$(dirname "${BASH_SOURCE[0]}")"

echo "warning: this submits sample_batch.jsonl (1000 prompts) to live inference" >&2
echo "press Ctrl+C within 5s to cancel..." >&2
sleep 5

"${DEMO_DIR}/02-submit.sh" sample_batch.jsonl
echo
echo "poll with: scripts/demo/04-poll.sh"
echo "download with: scripts/demo/05-download.sh \"\" full_results.json"
