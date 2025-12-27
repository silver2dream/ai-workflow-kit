#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# Result Recording with Multi-Repo Support
# Requirements: 11.1-11.6, 24.3-24.5, 2.2, 2.3, 6.2, 6.4, 6.5
# ============================================================

ISSUE_ID="${1:?usage: write_result.sh <issue_id> <status> <pr_url> <summary_file>}"
STATUS="${2:?usage: write_result.sh <issue_id> <status> <pr_url> <summary_file>}"
PR_URL="${3:-}"
SUMMARY_FILE="${4:-}"

ROOT="${AI_STATE_ROOT:-$(git rev-parse --show-toplevel)}"
RESULTS_ROOT="${AI_RESULTS_ROOT:-$ROOT}"

# Metrics from environment
EXEC_DURATION="${AI_EXEC_DURATION:-0}"
RETRY_COUNT="${AI_RETRY_COUNT:-0}"

# Session fields from environment (Req 2.2, 2.3, 6.2)
WORKER_SESSION_ID="${WORKER_SESSION_ID:-}"
ATTEMPT_NUMBER="${AI_ATTEMPT_NUMBER:-1}"
PREV_SESSION_IDS="${AI_PREV_SESSION_IDS:-[]}"
PREV_FAILURE_REASON="${AI_PREV_FAILURE_REASON:-}"
FAILURE_REASON="${AI_FAILURE_REASON:-}"

# Multi-repo fields from environment (Req 11.1-11.6)
REPO_TYPE="${AI_REPO_TYPE:-root}"
REPO_PATH="${AI_REPO_PATH:-./}"
SUBMODULE_SHA="${AI_SUBMODULE_SHA:-}"
CONSISTENCY_STATUS="${AI_CONSISTENCY_STATUS:-consistent}"
FAILURE_STAGE="${AI_FAILURE_STAGE:-}"

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

# Compute WORK_DIR based on repo type (Req 11.2)
WORK_DIR="$ROOT"
if [[ "$REPO_TYPE" != "root" && -n "$REPO_PATH" && "$REPO_PATH" != "./" ]]; then
  WORK_DIR="$ROOT/$REPO_PATH"
fi

# Generate recovery command for inconsistent states (Req 24.4, 24.5)
RECOVERY_COMMAND=""
if [[ "$CONSISTENCY_STATUS" != "consistent" && "$REPO_TYPE" == "submodule" ]]; then
  case "$CONSISTENCY_STATUS" in
    submodule_committed_parent_failed)
      RECOVERY_COMMAND="cd $ROOT/$REPO_PATH && git reset --hard HEAD~1"
      ;;
    submodule_push_failed)
      RECOVERY_COMMAND="cd $ROOT/$REPO_PATH && git push origin HEAD"
      ;;
    parent_push_failed_submodule_pushed)
      RECOVERY_COMMAND="cd $ROOT && git push origin $BRANCH"
      ;;
  esac
fi

sanitize_int() {
  local value="${1:-0}"
  local default="${2:-0}"
  if [[ "$value" =~ ^[0-9]+$ ]]; then
    printf '%s' "$value"
    return 0
  fi
  printf '%s' "$default"
}

sanitize_json_array() {
  local raw="${1:-}"
  if [[ -z "$raw" ]]; then
    printf '[]'
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    PREV_JSON="$raw" python3 - <<'PY'
import json
import os
import sys

raw = os.environ.get("PREV_JSON", "")
try:
    v = json.loads(raw)
    if isinstance(v, list):
        sys.stdout.write(json.dumps(v, separators=(",", ":")))
    else:
        sys.stdout.write("[]")
except Exception:
    sys.stdout.write("[]")
PY
    return 0
  fi
  if command -v python >/dev/null 2>&1; then
    PREV_JSON="$raw" python - <<'PY'
import json
import os
import sys

raw = os.environ.get("PREV_JSON", "")
try:
    v = json.loads(raw)
    if isinstance(v, list):
        sys.stdout.write(json.dumps(v, separators=(",", ":")))
    else:
        sys.stdout.write("[]")
except Exception:
    sys.stdout.write("[]")
PY
    return 0
  fi
  printf '[]'
}

TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OUT="$OUT_DIR/issue-$ISSUE_ID.json"
OUT_TMP="${OUT}.tmp.$$"
trap 'rm -f "$OUT_TMP" 2>/dev/null || true' EXIT

ATTEMPT_NUMBER="$(sanitize_int "$ATTEMPT_NUMBER" "1")"
EXEC_DURATION="$(sanitize_int "$EXEC_DURATION" "0")"
RETRY_COUNT="$(sanitize_int "$RETRY_COUNT" "0")"
PREV_SESSION_IDS="$(sanitize_json_array "$PREV_SESSION_IDS")"

json_escape() {
  local input
  input="$(cat)"

  # Handle empty input
  if [[ -z "$input" ]]; then
    printf '""'
    return
  fi

  if command -v python3 >/dev/null 2>&1; then
    printf '%s' "$input" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()), end="")'
    return
  fi
  if command -v python >/dev/null 2>&1; then
    printf '%s' "$input" | python -c 'import json,sys; print(json.dumps(sys.stdin.read()), end="")'
    return
  fi

  # Fallback: manual escaping
  input="${input//\\/\\\\}"
  input="${input//\"/\\\"}"
  input="${input//$'\n'/\\n}"
  printf '"%s"' "$input"
}

{
  echo "{"
  echo "  \"issue_id\": $(printf '%s' "$ISSUE_ID" | json_escape),"
  echo "  \"status\": $(printf '%s' "$STATUS" | json_escape),"
  echo "  \"repo\": $(printf '%s' "$REPO_NAME" | json_escape),"
  echo "  \"repo_type\": $(printf '%s' "$REPO_TYPE" | json_escape),"
  echo "  \"work_dir\": $(printf '%s' "$WORK_DIR" | json_escape),"
  echo "  \"branch\": $(printf '%s' "$BRANCH" | json_escape),"
  echo "  \"base_branch\": $(printf '%s' "$BASE_BRANCH" | json_escape),"
  echo "  \"head_sha\": $(printf '%s' "$HEAD_SHA" | json_escape),"
  echo "  \"submodule_sha\": $(printf '%s' "$SUBMODULE_SHA" | json_escape),"
  echo "  \"consistency_status\": $(printf '%s' "$CONSISTENCY_STATUS" | json_escape),"
  echo "  \"failure_stage\": $(printf '%s' "$FAILURE_STAGE" | json_escape),"
  echo "  \"recovery_command\": $(printf '%s' "$RECOVERY_COMMAND" | json_escape),"
  echo "  \"timestamp_utc\": $(printf '%s' "$TS" | json_escape),"
  echo "  \"pr_url\": $(printf '%s' "$PR_URL" | json_escape),"
  echo "  \"summary_file\": $(printf '%s' "$SUMMARY_FILE" | json_escape),"
  echo "  \"submodule_status\": $(printf '%s' "$SUBMODULE_STATUS" | json_escape),"
  echo "  \"session\": {"
  echo "    \"worker_session_id\": $(printf '%s' "$WORKER_SESSION_ID" | json_escape),"
  echo "    \"principal_session_id\": $(printf '%s' "" | json_escape),"
  echo "    \"attempt_number\": $ATTEMPT_NUMBER,"
  echo "    \"previous_session_ids\": $PREV_SESSION_IDS,"
  echo "    \"previous_failure_reason\": $(printf '%s' "$PREV_FAILURE_REASON" | json_escape)"
  echo "  },"
  echo "  \"review_audit\": {"
  echo "    \"reviewer_session_id\": $(printf '%s' "" | json_escape),"
  echo "    \"review_timestamp\": $(printf '%s' "" | json_escape),"
  echo "    \"ci_status\": $(printf '%s' "" | json_escape),"
  echo "    \"ci_timeout\": false,"
  echo "    \"decision\": $(printf '%s' "" | json_escape),"
  echo "    \"merge_timestamp\": $(printf '%s' "" | json_escape)"
  echo "  },"
  echo "  \"metrics\": {"
  echo "    \"duration_seconds\": $EXEC_DURATION,"
  echo "    \"retry_count\": $RETRY_COUNT"
  echo "  }"
  echo "}"
} > "$OUT_TMP"

if command -v python3 >/dev/null 2>&1; then
  python3 - "$OUT_TMP" <<'PY' >/dev/null
import json
import sys

with open(sys.argv[1], "rb") as handle:
    json.load(handle)
PY
elif command -v python >/dev/null 2>&1; then
  python - "$OUT_TMP" <<'PY' >/dev/null
import json
import sys

with open(sys.argv[1], "rb") as handle:
    json.load(handle)
PY
fi

mv -f "$OUT_TMP" "$OUT"

echo "[write_result] wrote $OUT"
