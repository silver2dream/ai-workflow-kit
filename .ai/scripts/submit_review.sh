#!/usr/bin/env bash
# Submit Review - 提交 PR 審查結果
# stdout: 執行結果
# stderr: log
#
# 用法: bash submit_review.sh <PR_NUMBER> <ISSUE_NUMBER> <SCORE> <CI_STATUS> "<REVIEW_BODY>"

set -euo pipefail

# Timeout helpers
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/timeout.sh"
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/hash.sh"

log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  echo "$msg" >> .ai/exe-logs/principal.log 2>/dev/null || true
}

PR_NUMBER="${1:?Usage: submit_review.sh <PR_NUMBER> <ISSUE_NUMBER> <SCORE> <CI_STATUS> <REVIEW_BODY>}"
ISSUE_NUMBER="${2:?}"
SCORE="${3:?}"
CI_STATUS="${4:?}"
REVIEW_BODY="${5:-}"

log "提交審查 PR #$PR_NUMBER (Score: $SCORE/10)"

# ============================================================
# 獲取基本資訊
# ============================================================
PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "unknown")
DIFF_HASH=$(gh_with_timeout pr diff "$PR_NUMBER" 2>/dev/null | sha256_16 || echo "")
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# ============================================================
# 發布 AWK Review Comment
# ============================================================
log "發布 AWK Review Comment..."

COMMENT_BODY="<!-- AWK:session:$PRINCIPAL_SESSION_ID -->
🤖 **AWK Review**

| Field | Value |
|-------|-------|
| Reviewer Session | \`$PRINCIPAL_SESSION_ID\` |
| Review Timestamp | $TIMESTAMP |
| CI Status | $CI_STATUS |
| Diff Hash | \`$DIFF_HASH\` |
| Score | $SCORE/10 |

$REVIEW_BODY"

gh_with_timeout pr comment "$PR_NUMBER" --body "$COMMENT_BODY"
log "✓ AWK Review Comment 已發布"

# ============================================================
# 發布 GitHub Review
# ============================================================
if [[ "$SCORE" -ge 7 ]]; then
  log "發布 GitHub Review: APPROVE"
  gh_with_timeout pr review "$PR_NUMBER" --approve --body "AWK Review: APPROVED (score: $SCORE/10)"
  
  # ============================================================
  # 審查通過：合併 PR（如果 CI 通過）
  # ============================================================
  if [[ "$CI_STATUS" == "passed" ]]; then
    log "CI 通過，合併 PR..."
    
    if gh_with_timeout pr merge "$PR_NUMBER" --squash --delete-branch; then
      log "✓ PR 已合併"
      
      # 關閉 Issue
      gh_with_timeout issue close "$ISSUE_NUMBER" 2>/dev/null || true
      log "✓ Issue #$ISSUE_NUMBER 已關閉"
      
      # 移除 pr-ready 標籤
      gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" 2>/dev/null || true
      
      # 更新 tasks.md
      RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"
      if [[ -f "$RESULT_FILE" ]]; then
        SPEC_NAME=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('spec_name',''))" 2>/dev/null || echo "")
        TASK_LINE=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('task_line',''))" 2>/dev/null || echo "")
        
        if [[ -n "$SPEC_NAME" && -n "$TASK_LINE" ]]; then
          TASKS_FILE=".ai/specs/$SPEC_NAME/tasks.md"
          if [[ -f "$TASKS_FILE" ]]; then
            if python3 - "$TASKS_FILE" "$TASK_LINE" <<'PY' 2>/dev/null; then
import sys

path = sys.argv[1]
line_number = int(sys.argv[2])

with open(path, "r", encoding="utf-8") as handle:
    lines = handle.readlines()

if 1 <= line_number <= len(lines):
    lines[line_number - 1] = lines[line_number - 1].replace("[ ]", "[x]", 1)

with open(path, "w", encoding="utf-8") as handle:
    handle.writelines(lines)
PY
              log "✓ 已更新 $TASKS_FILE 第 $TASK_LINE 行為完成"
            else
              log "⚠ 更新 tasks.md 失敗: $TASKS_FILE (line $TASK_LINE)"
            fi
          fi
        fi
      fi
      
      # 清理 worktree
      WT_DIR=".worktrees/issue-$ISSUE_NUMBER"
      if [[ -d "$WT_DIR" ]]; then
        git worktree remove "$WT_DIR" --force 2>/dev/null || true
        log "✓ 已清理 worktree: $WT_DIR"
      fi
      
      echo "RESULT=merged"
    else
      MERGE_STATE_STATUS="$(gh_with_timeout pr view "$PR_NUMBER" --json mergeStateStatus --jq '.mergeStateStatus' 2>/dev/null || echo "unknown")"
      log "✗ PR 合併失敗 (mergeStateStatus: $MERGE_STATE_STATUS)"

      NEXT_STEP="請到 PR 頁面查看 merge 錯誤原因。"
      case "$MERGE_STATE_STATUS" in
        DIRTY) NEXT_STEP="PR 有 merge conflict，請解決衝突後 push 重新嘗試合併。" ;;
        BEHIND) NEXT_STEP="PR 分支落後 base branch，請 rebase/merge base branch 後 push 重新嘗試合併。" ;;
        BLOCKED) NEXT_STEP="PR 被保護規則擋住（checks/reviews），請確認 required checks/reviews 後再合併。" ;;
      esac

      gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" 2>/dev/null || true
      gh_with_timeout issue edit "$ISSUE_NUMBER" --add-label "needs-human-review" 2>/dev/null || true
      gh_with_timeout issue comment "$ISSUE_NUMBER" --body "## AWK Review: 合併失敗（需要人工介入）

PR: #$PR_NUMBER
mergeStateStatus: \`$MERGE_STATE_STATUS\`

下一步建議：$NEXT_STEP" 2>/dev/null || true

      echo "RESULT=merge_failed"
    fi
  else
    log "⚠ CI 未通過，審查通過但不合併"
    
    # 在 Issue 上留言說明 CI 失敗
    gh_with_timeout issue comment "$ISSUE_NUMBER" --body "## AWK Review 通過，但 CI 失敗

審查評分: $SCORE/10 ✅

$REVIEW_BODY

---
**CI 狀態**: ❌ 失敗

請檢查 CI 日誌並修復問題後重新提交。
PR: #$PR_NUMBER" 2>/dev/null || true
    
    # 移除 pr-ready，加回 ai-task 讓 Worker 重做
    gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "ai-task" 2>/dev/null || true
    
    log "✓ Issue 標籤已更新，等待 Worker 修復 CI"
    
    echo "RESULT=approved_ci_failed"
  fi
else
  log "發布 GitHub Review: REQUEST_CHANGES"
  gh_with_timeout pr review "$PR_NUMBER" --request-changes --body "AWK Review: CHANGES REQUESTED (score: $SCORE/10)"
  
  # 移除 pr-ready，加回 ai-task
  gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "ai-task" 2>/dev/null || true
  
  # 在 Issue 上留下審查意見，讓 Worker 知道要改什麼
  gh_with_timeout issue comment "$ISSUE_NUMBER" --body "## AWK Review 不通過 (score: $SCORE/10)

$REVIEW_BODY

---
**Worker 請根據以上意見修改後重新提交。**
PR: #$PR_NUMBER" 2>/dev/null || true
  
  log "✓ Issue 標籤已更新，審查意見已留下，等待 Worker 重做"
  
  echo "RESULT=changes_requested"
fi

log "✓ 審查提交完成"
