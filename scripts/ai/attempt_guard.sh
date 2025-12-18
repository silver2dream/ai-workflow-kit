#!/usr/bin/env bash
set -euo pipefail

ISSUE_ID="${1:?usage: attempt_guard.sh <issue_id> <label>}"
LABEL="${2:-attempt}"

ROOT="${AI_STATE_ROOT:-$(git rev-parse --show-toplevel)}"
RUN_DIR="$ROOT/.ai/runs/issue-$ISSUE_ID"
mkdir -p "$RUN_DIR"

MAX="${AI_MAX_ATTEMPTS:-3}"
COUNT_FILE="$RUN_DIR/fail_count.txt"

COUNT=0
if [[ -f "$COUNT_FILE" ]]; then
  COUNT="$(cat "$COUNT_FILE" || echo 0)"
fi

COUNT=$((COUNT+1))
echo "$COUNT" > "$COUNT_FILE"

echo "[attempt_guard] issue=$ISSUE_ID label=$LABEL attempt=$COUNT/$MAX"

if [[ "$COUNT" -gt "$MAX" ]]; then
  echo "[attempt_guard] STOP-LOSS: exceeded max attempts ($MAX)" >&2
  exit 3
fi
