#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# rollback.sh - 回滾已合併的 PR
# ============================================================================
# 用法:
#   bash .ai/scripts/rollback.sh <PR_NUMBER> [--dry-run]
#
# 功能:
#   1. 獲取 PR 資訊
#   2. 創建 revert commit
#   3. 創建 revert PR
#   4. 重新開啟原 issue
#   5. 發送通知
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"

PR_NUMBER="${1:?usage: rollback.sh <PR_NUMBER> [--dry-run]}"
DRY_RUN="${2:-}"

echo "[rollback] Starting rollback for PR #$PR_NUMBER"

# 檢查 gh CLI
if ! command -v gh &> /dev/null; then
  echo "[rollback] ERROR: gh CLI not found"
  exit 1
fi

# 檢查認證
if ! gh auth status &> /dev/null; then
  echo "[rollback] ERROR: gh not authenticated. Run 'gh auth login'"
  exit 1
fi

# 獲取 PR 資訊
echo "[rollback] Fetching PR info..."
PR_INFO=$(gh pr view "$PR_NUMBER" --json title,body,mergeCommit,headRefName,state,mergedAt 2>/dev/null)

if [[ -z "$PR_INFO" ]]; then
  echo "[rollback] ERROR: PR #$PR_NUMBER not found"
  exit 1
fi

# 解析 PR 資訊
PR_TITLE=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('title',''))")
PR_BODY=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('body',''))")
MERGE_COMMIT=$(echo "$PR_INFO" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('mergeCommit',{}).get('oid','') if d.get('mergeCommit') else '')")
PR_STATE=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('state',''))")
MERGED_AT=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('mergedAt',''))")

echo "[rollback] PR Title: $PR_TITLE"
echo "[rollback] PR State: $PR_STATE"
echo "[rollback] Merge Commit: $MERGE_COMMIT"

# 檢查 PR 是否已合併
if [[ "$PR_STATE" != "MERGED" ]]; then
  echo "[rollback] ERROR: PR #$PR_NUMBER is not merged (state: $PR_STATE)"
  exit 1
fi

if [[ -z "$MERGE_COMMIT" ]]; then
  echo "[rollback] ERROR: Cannot find merge commit for PR #$PR_NUMBER"
  exit 1
fi

# 從 PR body 提取原 issue 編號
ISSUE_NUMBER=$(echo "$PR_BODY" | grep -oP '(?i)(?:closes|fixes|resolves)\s*#\K\d+' | head -1 || echo "")
echo "[rollback] Original Issue: ${ISSUE_NUMBER:-none}"

# Dry run 模式
if [[ "$DRY_RUN" == "--dry-run" ]]; then
  echo ""
  echo "[rollback] DRY RUN - Would execute:"
  echo "  1. git revert $MERGE_COMMIT --no-edit"
  echo "  2. git push origin HEAD"
  echo "  3. gh pr create --title 'Revert: $PR_TITLE'"
  if [[ -n "$ISSUE_NUMBER" ]]; then
    echo "  4. gh issue reopen $ISSUE_NUMBER"
  fi
  echo "  5. Send notification"
  exit 0
fi

# 確保工作目錄乾淨
if [[ -n "$(git status --porcelain)" ]]; then
  echo "[rollback] ERROR: Working directory not clean"
  exit 1
fi

# 獲取當前分支
CURRENT_BRANCH=$(git branch --show-current)
echo "[rollback] Current branch: $CURRENT_BRANCH"

# 創建 revert 分支
REVERT_BRANCH="revert-pr-$PR_NUMBER"
echo "[rollback] Creating revert branch: $REVERT_BRANCH"

git fetch origin "$CURRENT_BRANCH"
git checkout -b "$REVERT_BRANCH" "origin/$CURRENT_BRANCH"

# 執行 revert
echo "[rollback] Reverting commit $MERGE_COMMIT..."
if ! git revert "$MERGE_COMMIT" --no-edit; then
  echo "[rollback] ERROR: Revert failed. Manual intervention required."
  git revert --abort 2>/dev/null || true
  git checkout "$CURRENT_BRANCH"
  git branch -D "$REVERT_BRANCH" 2>/dev/null || true
  exit 1
fi

# 推送分支
echo "[rollback] Pushing revert branch..."
git push origin "$REVERT_BRANCH"

# 創建 revert PR
echo "[rollback] Creating revert PR..."
REVERT_PR_URL=$(gh pr create \
  --title "Revert: $PR_TITLE" \
  --body "This reverts PR #$PR_NUMBER (commit $MERGE_COMMIT).

**Reason:** [Please add reason for rollback]

**Original PR:** #$PR_NUMBER
**Original Issue:** ${ISSUE_NUMBER:-N/A}
**Reverted at:** $(date -u +"%Y-%m-%dT%H:%M:%SZ")

---
_This revert was created automatically by rollback.sh_" \
  --head "$REVERT_BRANCH" \
  2>&1)

echo "[rollback] Revert PR created: $REVERT_PR_URL"

# 重新開啟原 issue
if [[ -n "$ISSUE_NUMBER" ]]; then
  echo "[rollback] Reopening issue #$ISSUE_NUMBER..."
  gh issue reopen "$ISSUE_NUMBER" --comment "🔄 Reopened due to rollback of PR #$PR_NUMBER.

Revert PR: $REVERT_PR_URL" 2>/dev/null || echo "[rollback] WARN: Could not reopen issue #$ISSUE_NUMBER"
fi

# 發送通知
if [[ -f "$SCRIPT_DIR/notify.sh" ]]; then
  echo "[rollback] Sending notification..."
  bash "$SCRIPT_DIR/notify.sh" "🔄 Rollback: PR #$PR_NUMBER has been reverted. Revert PR: $REVERT_PR_URL" 2>/dev/null || true
fi

# 切回原分支
git checkout "$CURRENT_BRANCH"

echo ""
echo "[rollback] ✅ Rollback complete!"
echo "  Revert PR: $REVERT_PR_URL"
echo "  Original Issue: ${ISSUE_NUMBER:-N/A}"
echo ""
echo "Next steps:"
echo "  1. Review and merge the revert PR"
echo "  2. Investigate the issue"
echo "  3. Create a fix PR"
