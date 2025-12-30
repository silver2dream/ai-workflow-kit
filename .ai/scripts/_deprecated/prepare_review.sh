#!/usr/bin/env bash
# Prepare Review - 準備 PR 審查所需的所有資訊
# stdout: 輸出所有審查資訊
# stderr: log
#
# 用法: bash prepare_review.sh <PR_NUMBER> <ISSUE_NUMBER>

set -euo pipefail

# Timeout helpers
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/timeout.sh"
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/hash.sh"

resolve_main_root() {
  if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    local common_dir=""
    common_dir="$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"
    if [[ -z "$common_dir" ]]; then
      common_dir="$(git rev-parse --git-common-dir 2>/dev/null || true)"
      if [[ -n "$common_dir" ]]; then
        common_dir="$(cd "$common_dir" 2>/dev/null && pwd -P || true)"
      fi
    fi
    if [[ -n "$common_dir" ]]; then
      echo "$(dirname "$common_dir")"
      return 0
    fi
    git rev-parse --show-toplevel 2>/dev/null || pwd -P 2>/dev/null || pwd
    return 0
  fi
  pwd -P 2>/dev/null || pwd
}

file_size_bytes() {
  local path="${1:?}"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$path" <<'PY'
import os
import sys

print(os.path.getsize(sys.argv[1]))
PY
    return 0
  fi
  if command -v python >/dev/null 2>&1; then
    python - "$path" <<'PY'
import os
import sys

print(os.path.getsize(sys.argv[1]))
PY
    return 0
  fi
  wc -c <"$path" 2>/dev/null || echo "0"
}

MAIN_ROOT="$(resolve_main_root)"
PRINCIPAL_LOG="$MAIN_ROOT/.ai/exe-logs/principal.log"

log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  echo "$msg" >> "$PRINCIPAL_LOG" 2>/dev/null || true
}

PR_NUMBER="${1:?Usage: prepare_review.sh <PR_NUMBER> <ISSUE_NUMBER>}"
ISSUE_NUMBER="${2:?Usage: prepare_review.sh <PR_NUMBER> <ISSUE_NUMBER>}"

log "準備審查 PR #$PR_NUMBER (Issue #$ISSUE_NUMBER)"

# ============================================================
# 1. Session ID
# ============================================================
PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "unknown")

# ============================================================
# 2. CI Status
# ============================================================
# Try --json first (gh >= 2.12), fall back to parsing text output for older versions
CI_STATUS="passed"
CI_OUTPUT=$(gh_with_timeout pr checks "$PR_NUMBER" --json state --jq '.[].state' 2>/dev/null || true)
if [[ -n "$CI_OUTPUT" ]]; then
  # New gh version with --json support
  if echo "$CI_OUTPUT" | grep -q "FAILURE"; then
    CI_STATUS="failed"
  fi
else
  # Fallback for older gh versions: parse text output (e.g., "backend  pass  21s  ...")
  CI_OUTPUT=$(gh_with_timeout pr checks "$PR_NUMBER" 2>/dev/null || true)
  if echo "$CI_OUTPUT" | grep -qE '\bfail\b'; then
    CI_STATUS="failed"
  fi
fi

# ============================================================
# 3. Diff (saved for audit)
# ============================================================
REVIEW_DIR="$MAIN_ROOT/.ai/state/reviews/pr-$PR_NUMBER"
mkdir -p "$REVIEW_DIR" 2>/dev/null || true
DIFF_PATH="$REVIEW_DIR/diff.patch"

DIFF_HASH="unavailable"
DIFF_BYTES="unavailable"

DIFF_TMP="$DIFF_PATH.tmp.$$"
if gh_with_timeout pr diff "$PR_NUMBER" >"$DIFF_TMP" 2>/dev/null; then
  mv -f "$DIFF_TMP" "$DIFF_PATH" 2>/dev/null || true
  rm -f "$DIFF_TMP" 2>/dev/null || true
else
  rm -f "$DIFF_TMP" "$DIFF_PATH" 2>/dev/null || true
fi

if [[ -f "$DIFF_PATH" ]]; then
  if [[ -s "$DIFF_PATH" ]]; then
    DIFF_HASH="$(sha256_16 <"$DIFF_PATH" 2>/dev/null || echo "unavailable")"
    DIFF_BYTES="$(file_size_bytes "$DIFF_PATH" 2>/dev/null || echo "unavailable")"
  else
    DIFF_HASH="empty"
    DIFF_BYTES="0"
  fi
fi

# ============================================================
# 4. Worktree Path
# ============================================================
WT_DIR="$MAIN_ROOT/.worktrees/issue-$ISSUE_NUMBER"
if [[ ! -d "$WT_DIR" ]]; then
  WT_DIR="NOT_FOUND"
  log "⚠ Worktree 不存在: $WT_DIR"
fi

# ============================================================
# 輸出 Header
# ============================================================
cat <<EOF
============================================================
AWK PR REVIEW CONTEXT
============================================================
PR_NUMBER: $PR_NUMBER
ISSUE_NUMBER: $ISSUE_NUMBER
PRINCIPAL_SESSION_ID: $PRINCIPAL_SESSION_ID
CI_STATUS: $CI_STATUS
DIFF_HASH: $DIFF_HASH
DIFF_BYTES: $DIFF_BYTES
REVIEW_DIR: $REVIEW_DIR
WORKTREE_PATH: $WT_DIR
============================================================

EOF

# ============================================================
# 5. Issue 內容（Ticket 需求）
# ============================================================
echo "## TICKET REQUIREMENTS (Issue #$ISSUE_NUMBER)"
echo ""
gh_with_timeout issue view "$ISSUE_NUMBER" --json title,body,labels 2>/dev/null || echo "ERROR: Cannot fetch issue"
echo ""

# ============================================================
# 6. Task File（如果有）
# ============================================================
TASK_FILE="$MAIN_ROOT/.ai/runs/issue-$ISSUE_NUMBER/prompt.txt"
if [[ -f "$TASK_FILE" ]]; then
  echo "## TASK FILE"
  echo ""
  cat "$TASK_FILE"
  echo ""
fi

# ============================================================
# 7. PR Diff
# ============================================================
echo "============================================================"
echo "## PR DIFF"
echo "============================================================"
echo ""
if [[ -f "$DIFF_PATH" ]]; then
  cat "$DIFF_PATH"
else
  gh_with_timeout pr diff "$PR_NUMBER" 2>/dev/null || echo "ERROR: Cannot fetch diff"
fi
echo ""

# ============================================================
# 8. PR Commits
# ============================================================
echo "============================================================"
echo "## PR COMMITS"
echo "============================================================"
echo ""
gh_with_timeout pr view "$PR_NUMBER" --json commits --jq '.commits[] | "- \(.oid[0:7]) \(.messageHeadline)"' 2>/dev/null || echo "ERROR: Cannot fetch commits"
echo ""

echo "============================================================"
echo "END OF REVIEW CONTEXT"
echo "============================================================"

log "✓ 審查資訊準備完成"
