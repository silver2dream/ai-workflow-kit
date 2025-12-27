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

log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  echo "$msg" >> .ai/exe-logs/principal.log 2>/dev/null || true
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
# 3. Diff Hash
# ============================================================
DIFF_HASH=$(gh_with_timeout pr diff "$PR_NUMBER" 2>/dev/null | sha256_16 || echo "")

# ============================================================
# 4. Worktree Path
# ============================================================
WT_DIR=".worktrees/issue-$ISSUE_NUMBER"
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
TASK_FILE=".ai/runs/issue-$ISSUE_NUMBER/prompt.txt"
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
gh_with_timeout pr diff "$PR_NUMBER" 2>/dev/null || echo "ERROR: Cannot fetch diff"
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
