#!/usr/bin/env bash
# Submit Review - 提交 PR 審查結果
# stdout: 執行結果
# stderr: log
#
# 用法: bash submit_review.sh <PR_NUMBER> <ISSUE_NUMBER> <SCORE> <CI_STATUS> "<REVIEW_BODY>"

set -euo pipefail

log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  echo "$msg" >&2
  echo "$msg" >> .ai/exe-logs/submit_review.log 2>/dev/null || true
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
DIFF_HASH=$(gh pr diff "$PR_NUMBER" 2>/dev/null | sha256sum | cut -c1-16)
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

gh pr comment "$PR_NUMBER" --body "$COMMENT_BODY"
log "✓ AWK Review Comment 已發布"

# ============================================================
# 發布 GitHub Review
# ============================================================
if [[ "$SCORE" -ge 7 ]]; then
  log "發布 GitHub Review: APPROVE"
  gh pr review "$PR_NUMBER" --approve --body "AWK Review: APPROVED (score: $SCORE/10)"
  
  # ============================================================
  # 審查通過：合併 PR（如果 CI 通過）
  # ============================================================
  if [[ "$CI_STATUS" == "passed" ]]; then
    log "CI 通過，合併 PR..."
    
    if gh pr merge "$PR_NUMBER" --squash --delete-branch; then
      log "✓ PR 已合併"
      
      # 關閉 Issue
      gh issue close "$ISSUE_NUMBER" 2>/dev/null || true
      log "✓ Issue #$ISSUE_NUMBER 已關閉"
      
      # 移除 pr-ready 標籤
      gh issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" 2>/dev/null || true
      
      # 更新 tasks.md
      RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"
      if [[ -f "$RESULT_FILE" ]]; then
        SPEC_NAME=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('spec_name',''))" 2>/dev/null || echo "")
        TASK_LINE=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('task_line',''))" 2>/dev/null || echo "")
        
        if [[ -n "$SPEC_NAME" && -n "$TASK_LINE" ]]; then
          TASKS_FILE=".ai/specs/$SPEC_NAME/tasks.md"
          if [[ -f "$TASKS_FILE" ]]; then
            sed -i "${TASK_LINE}s/\[ \]/[x]/" "$TASKS_FILE"
            log "✓ 已更新 $TASKS_FILE 第 $TASK_LINE 行為完成"
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
      log "✗ PR 合併失敗"
      echo "RESULT=merge_failed"
    fi
  else
    log "⚠ CI 未通過，審查通過但不合併"
    
    # 在 Issue 上留言說明 CI 失敗
    gh issue comment "$ISSUE_NUMBER" --body "## AWK Review 通過，但 CI 失敗

審查評分: $SCORE/10 ✅

$REVIEW_BODY

---
**CI 狀態**: ❌ 失敗

請檢查 CI 日誌並修復問題後重新提交。
PR: #$PR_NUMBER" 2>/dev/null || true
    
    # 移除 pr-ready，加回 ai-task 讓 Worker 重做
    gh issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "ai-task" 2>/dev/null || true
    
    log "✓ Issue 標籤已更新，等待 Worker 修復 CI"
    
    echo "RESULT=approved_ci_failed"
  fi
else
  log "發布 GitHub Review: REQUEST_CHANGES"
  gh pr review "$PR_NUMBER" --request-changes --body "AWK Review: CHANGES REQUESTED (score: $SCORE/10)"
  
  # 移除 pr-ready，加回 ai-task
  gh issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "ai-task" 2>/dev/null || true
  
  # 在 Issue 上留下審查意見，讓 Worker 知道要改什麼
  gh issue comment "$ISSUE_NUMBER" --body "## AWK Review 不通過 (score: $SCORE/10)

$REVIEW_BODY

---
**Worker 請根據以上意見修改後重新提交。**
PR: #$PR_NUMBER" 2>/dev/null || true
  
  log "✓ Issue 標籤已更新，審查意見已留下，等待 Worker 重做"
  
  echo "RESULT=changes_requested"
fi

log "✓ 審查提交完成"
