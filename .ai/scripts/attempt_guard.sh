#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# attempt_guard.sh - ?箄?岫摰?嚗?航炊??嚗?
# ============================================================================
# ?冽?:
#   bash .ai/scripts/attempt_guard.sh <issue_id> [label] [log_file]
#
# ??箇Ⅳ:
#   0 - ?臭誑蝜潛??岫
#   1 - 銝?岫?隤?
#   3 - 頞??憭批?閰行活??
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

# 霈?????
COUNT=0
if [[ -f "$COUNT_FILE" ]]; then
  COUNT="$(cat "$COUNT_FILE" || echo 0)"
fi

# ??憭望???嚗???靘??亥?嚗?
ANALYSIS='{"matched":false,"type":"unknown","retryable":false}'
if [[ -n "$LOG_FILE" ]] && [[ -f "$LOG_FILE" ]]; then
  ANALYSIS=$(bash "$SCRIPT_DIR/analyze_failure.sh" "$LOG_FILE" 2>/dev/null || echo "$ANALYSIS")
elif [[ -n "$LOG_FILE" ]] && [[ "$LOG_FILE" == "-" ]]; then
  ANALYSIS=$(bash "$SCRIPT_DIR/analyze_failure.sh" - 2>/dev/null || echo "$ANALYSIS")
fi

# 閫????蝯?
MATCHED=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(str(d.get('matched',False)).lower())" 2>/dev/null || echo "false")
ERROR_TYPE=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('type','unknown'))" 2>/dev/null || echo "unknown")
RETRYABLE=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(str(d.get('retryable',False)).lower())" 2>/dev/null || echo "false")
SUGGESTION=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('suggestion',''))" 2>/dev/null || echo "")
PATTERN_ID=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pattern_id','unknown'))" 2>/dev/null || echo "unknown")

# 閮??唳風??
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "{\"timestamp\":\"$TIMESTAMP\",\"issue_id\":$ISSUE_ID,\"attempt\":$COUNT,\"pattern_id\":\"$PATTERN_ID\",\"type\":\"$ERROR_TYPE\",\"retryable\":$RETRYABLE}" >> "$HISTORY_FILE"

echo "[attempt_guard] issue=$ISSUE_ID label=$LABEL attempt=$COUNT/$MAX"
echo "[attempt_guard] error_type=$ERROR_TYPE retryable=$RETRYABLE"

# 憒??臭??舫?閰衣??航炊嚗??喳?甇?
if [[ "$MATCHED" == "true" ]] && [[ "$RETRYABLE" == "false" ]]; then
  echo "[attempt_guard] NON-RETRYABLE ERROR: $ERROR_TYPE" >&2
  if [[ -n "$SUGGESTION" ]]; then
    echo "[attempt_guard] Suggestion: $SUGGESTION" >&2
  fi
  exit 1
fi

# 憓?閮
COUNT=$((COUNT+1))
echo "$COUNT" > "$COUNT_FILE"

# 瑼Ｘ?臬頞??憭批?閰行活??
if [[ "$COUNT" -gt "$MAX" ]]; then
  echo "[attempt_guard] STOP-LOSS: exceeded max attempts ($MAX)" >&2
  exit 3
fi

# 憒??臬?岫?隤歹?憿舐內撱箄降
if [[ "$MATCHED" == "true" ]] && [[ "$RETRYABLE" == "true" ]]; then
  RETRY_DELAY=$(echo "$ANALYSIS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('retry_delay_seconds',0))" 2>/dev/null || echo "0")
  if [[ "$RETRY_DELAY" -gt 0 ]]; then
    echo "[attempt_guard] Waiting ${RETRY_DELAY}s before retry..."
    sleep "$RETRY_DELAY"
  fi
fi

echo "[attempt_guard] OK to proceed with attempt $COUNT"
exit 0
