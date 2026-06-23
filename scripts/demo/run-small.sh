#!/usr/bin/env bash
set -euo pipefail

# Full small-batch demo: health → submit (3 prompts) → poll → download.
# Run the server first in another terminal: scripts/start-server.sh

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

DEMO_DIR="$(dirname "${BASH_SOURCE[0]}")"

echo "=== 1. Health ==="
"${DEMO_DIR}/01-health.sh"
echo

echo "=== 2. Submit (demo_live.jsonl — 3 prompts) ==="
"${DEMO_DIR}/02-submit.sh" demo_live.jsonl
echo

echo "=== 3. Poll until done ==="
"${DEMO_DIR}/04-poll.sh"
echo

echo "=== 4. Download ==="
"${DEMO_DIR}/05-download.sh" "" demo_results.json
