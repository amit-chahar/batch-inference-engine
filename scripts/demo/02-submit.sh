#!/usr/bin/env bash
set -euo pipefail

# Usage: scripts/demo/02-submit.sh [input_file]
# Default input: demo_live.jsonl (3 prompts)

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

INPUT_FILE="${1:-demo_live.jsonl}"
if [[ "${INPUT_FILE}" != /* ]]; then
  INPUT_FILE="${ROOT}/${INPUT_FILE}"
fi

if [[ ! -f "${INPUT_FILE}" ]]; then
  echo "error: input file not found: ${INPUT_FILE}" >&2
  exit 1
fi

require_server

BODY="$(jq -n --arg path "${INPUT_FILE}" '{input_file: $path}')"

echo "POST ${BASE_URL}/job/submit"
echo "input_file=${INPUT_FILE}"
echo

RESPONSE="$(curl -s -X POST "${BASE_URL}/job/submit" \
  -H "Content-Type: application/json" \
  -d "${BODY}")"

echo "${RESPONSE}" | pretty_json

JOB_ID="$(echo "${RESPONSE}" | jq -r .job_id)"
if [[ -z "${JOB_ID}" || "${JOB_ID}" == "null" ]]; then
  echo "error: submit did not return job_id" >&2
  exit 1
fi

save_job_id "${JOB_ID}"
echo
echo "saved job_id to scripts/demo/.last-job-id"
echo "next: scripts/demo/03-status.sh   or   scripts/demo/04-poll.sh"
