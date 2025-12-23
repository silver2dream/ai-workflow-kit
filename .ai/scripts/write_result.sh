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

TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OUT="$OUT_DIR/issue-$ISSUE_ID.json"

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
  echo "  \"issue_id\": \"$ISSUE_ID\","
  echo "  \"status\": \"$STATUS\","
  echo "  \"repo\": \"$REPO_NAME\","
  echo "  \"repo_type\": \"$REPO_TYPE\","
  echo "  \"work_dir\": \"$WORK_DIR\","
  echo "  \"branch\": \"$BRANCH\","
  echo "  \"base_branch\": \"$BASE_BRANCH\","
  echo "  \"head_sha\": \"$HEAD_SHA\","
  echo "  \"submodule_sha\": \"$SUBMODULE_SHA\","
  echo "  \"consistency_status\": \"$CONSISTENCY_STATUS\","
  echo "  \"failure_stage\": \"$FAILURE_STAGE\","
  echo "  \"recovery_command\": $(printf '%s' "$RECOVERY_COMMAND" | json_escape),"
  echo "  \"timestamp_utc\": \"$TS\","
  echo "  \"pr_url\": \"${PR_URL}\","
  echo "  \"summary_file\": \"${SUMMARY_FILE}\","
  echo "  \"submodule_status\": $(printf '%s' "$SUBMODULE_STATUS" | json_escape),"
  echo "  \"session\": {"
  echo "    \"worker_session_id\": \"$WORKER_SESSION_ID\","
  echo "    \"principal_session_id\": \"\","
  echo "    \"attempt_number\": $ATTEMPT_NUMBER,"
  echo "    \"previous_session_ids\": $PREV_SESSION_IDS,"
  echo "    \"previous_failure_reason\": $(printf '%s' "$PREV_FAILURE_REASON" | json_escape)"
  echo "  },"
  echo "  \"review_audit\": {"
  echo "    \"reviewer_session_id\": \"\","
  echo "    \"review_timestamp\": \"\","
  echo "    \"ci_status\": \"\","
  echo "    \"ci_timeout\": false,"
  echo "    \"decision\": \"\","
  echo "    \"merge_timestamp\": \"\""
  echo "  },"
  echo "  \"metrics\": {"
  echo "    \"duration_seconds\": $EXEC_DURATION,"
  echo "    \"retry_count\": $RETRY_COUNT"
  echo "  }"
  echo "}"
} > "$OUT"

echo "[write_result] wrote $OUT"
