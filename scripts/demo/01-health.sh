#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

echo "GET ${BASE_URL}/health"
curl -s "${BASE_URL}/health" | pretty_json
