#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# attempt_guard.sh - Track attempts and stop on non-retryable failures.
# ============================================================================
# Usage:
#   bash .ai/scripts/attempt_guard.sh <issue_id> [label] [log_file]
#
# Exit codes:
#   0 - OK to proceed
#   1 - Non-retryable failure
#   3 - Stop-loss (max attempts exceeded)
# ============================================================================

ISSUE_ID="${1:?usage: attempt_guard.sh <issue_id> [label] [log_file]}"
LABEL="${2:-attempt}"
LOG_FILE="${3:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="${AI_STATE_ROOT:-$(git rev-parse --show-toplevel)}"
RUN_DIR="$ROOT/.ai/runs/issue-$ISSUE_ID"
STATE_DIR="$ROOT/.ai/state"
mkdir -p "$RUN_DIR" "$STATE_DIR"

MAX="${AI_MAX_ATTEMPTS:-3}"
COUNT_FILE="$RUN_DIR/fail_count.txt"
HISTORY_FILE="$STATE_DIR/failure_history.jsonl"

# Read previous attempt count.
COUNT=0
if [[ -f "$COUNT_FILE" ]]; then
  COUNT="$(cat "$COUNT_FILE" || echo 0)"
fi

# Analyze the failure log (if provided).
ANALYSIS='{"matched":false,"type":"unknown","retryable":false}'
if [[ -n "$LOG_FILE" ]] && [[ -f "$LOG_FILE" ]]; then
  ANALYSIS=$(bash "$SCRIPT_DIR/analyze_failure.sh" "$LOG_FILE" 2>/dev/null || echo "$ANALYSIS")
elif [[ -n "$LOG_FILE" ]] && [[ "$LOG_FILE" == "-" ]]; then
  ANALYSIS=$(bash "$SCRIPT_DIR/analyze_failure.sh" - 2>/dev/null || echo "$ANALYSIS")
fi

# Extract analysis fields.
MATCHED=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(str(d.get('matched',False)).lower())" 2>/dev/null || echo "false")
ERROR_TYPE=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('type','unknown'))" 2>/dev/null || echo "unknown")
RETRYABLE=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(str(d.get('retryable',False)).lower())" 2>/dev/null || echo "false")
SUGGESTION=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('suggestion',''))" 2>/dev/null || echo "")
PATTERN_ID=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pattern_id','unknown'))" 2>/dev/null || echo "unknown")

# Append to failure history.
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "{\"timestamp\":\"$TIMESTAMP\",\"issue_id\":$ISSUE_ID,\"attempt\":$COUNT,\"pattern_id\":\"$PATTERN_ID\",\"type\":\"$ERROR_TYPE\",\"retryable\":$RETRYABLE}" >> "$HISTORY_FILE"

echo "[attempt_guard] issue=$ISSUE_ID label=$LABEL attempt=$COUNT/$MAX"
echo "[attempt_guard] error_type=$ERROR_TYPE retryable=$RETRYABLE"

# Stop immediately for non-retryable failures.
if [[ "$MATCHED" == "true" ]] && [[ "$RETRYABLE" == "false" ]]; then
  echo "[attempt_guard] NON-RETRYABLE ERROR: $ERROR_TYPE" >&2
  if [[ -n "$SUGGESTION" ]]; then
    echo "[attempt_guard] Suggestion: $SUGGESTION" >&2
  fi
  exit 1
fi

# Increment attempt count.
COUNT=$((COUNT+1))
echo "$COUNT" > "$COUNT_FILE"

# Stop-loss when exceeding max attempts.
if [[ "$COUNT" -gt "$MAX" ]]; then
  echo "[attempt_guard] STOP-LOSS: exceeded max attempts ($MAX)" >&2
  exit 3
fi

# Backoff before retry if a retry delay is specified.
if [[ "$MATCHED" == "true" ]] && [[ "$RETRYABLE" == "true" ]]; then
  RETRY_DELAY=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('retry_delay_seconds',0))" 2>/dev/null || echo "0")
  if [[ "$RETRY_DELAY" -gt 0 ]]; then
    echo "[attempt_guard] Waiting ${RETRY_DELAY}s before retry..."
    sleep "$RETRY_DELAY"
  fi
fi

echo "[attempt_guard] OK to proceed with attempt $COUNT"
exit 0
