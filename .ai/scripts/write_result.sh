#!/usr/bin/env bash
set -euo pipefail

ISSUE_ID="${1:?usage: write_result.sh <issue_id> <status> <pr_url> <summary_file>}"
STATUS="${2:?usage: write_result.sh <issue_id> <status> <pr_url> <summary_file>}"
PR_URL="${3:-}"
SUMMARY_FILE="${4:-}"

ROOT="${AI_STATE_ROOT:-$(git rev-parse --show-toplevel)}"
RESULTS_ROOT="${AI_RESULTS_ROOT:-$ROOT}"

# Metrics from environment
EXEC_DURATION="${AI_EXEC_DURATION:-0}"

OUT_DIR="$RESULTS_ROOT/.ai/results"
mkdir -p "$OUT_DIR"

REPO_NAME="${AI_REPO_NAME:-root}"
BRANCH="${AI_BRANCH_NAME:-feat/ai-issue-$ISSUE_ID}"
BASE_BRANCH="${AI_PR_BASE:-feat/aether}"

HEAD_SHA=""
if git -C "$ROOT" rev-parse --verify HEAD >/dev/null 2>&1; then
  HEAD_SHA="$(git -C "$ROOT" rev-parse HEAD)"
fi

SUBMODULE_STATUS=""
if [[ -f "$RESULTS_ROOT/.gitmodules" ]]; then
  SUBMODULE_STATUS="$(git -C "$RESULTS_ROOT" submodule status --recursive 2>/dev/null || true)"
fi

TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OUT="$OUT_DIR/issue-$ISSUE_ID.json"

{
  echo "{"
  echo "  \"issue_id\": \"$ISSUE_ID\","
  echo "  \"status\": \"$STATUS\","
  echo "  \"repo\": \"$REPO_NAME\","
  echo "  \"branch\": \"$BRANCH\","
  echo "  \"base_branch\": \"$BASE_BRANCH\","
  echo "  \"head_sha\": \"$HEAD_SHA\","
  echo "  \"timestamp_utc\": \"$TS\","
  echo "  \"pr_url\": \"${PR_URL}\","
  echo "  \"summary_file\": \"${SUMMARY_FILE}\","
  echo "  \"submodule_status\": $(python3 - <<PY
import json
s = \"\"\"$SUBMODULE_STATUS\"\"\"
print(json.dumps(s))
PY
  ),"
  echo "  \"metrics\": {"
  echo "    \"duration_seconds\": $EXEC_DURATION"
  echo "  }"
  echo "}"
} > "$OUT"

echo "[write_result] wrote $OUT"
