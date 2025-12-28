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
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# ============================================================
# Verify review evidence against diff
# ============================================================
PY_BIN=""
if command -v python3 >/dev/null 2>&1; then
  PY_BIN="python3"
elif command -v python >/dev/null 2>&1; then
  PY_BIN="python"
fi

REVIEW_MD="$REVIEW_DIR/review.md"
printf '%s' "$REVIEW_BODY" >"$REVIEW_MD"

EVIDENCE_COUNT=$(grep -c '^EVIDENCE:' "$REVIEW_MD" 2>/dev/null || echo "0")
EVIDENCE_LOG="$REVIEW_DIR/evidence_check.log"

if [[ -z "$PY_BIN" ]]; then
  log "✗ python not found; cannot verify review evidence"
  gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "needs-human-review" 2>/dev/null || true
  gh_with_timeout issue comment "$ISSUE_NUMBER" --body "## AWK Review blocked: missing python

無法驗證審查內容是否包含可核對的 evidence（需要 python3/python）。

PR: #$PR_NUMBER
Diff Hash: \`$DIFF_HASH\`

Next step: 安裝 python3 後重新執行 review。" 2>/dev/null || true
  bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "blocked" "$CI_STATUS" "false" "" 2>/dev/null || true
  echo "RESULT=review_blocked"
  exit 0
fi

if ! "$PY_BIN" "$MAIN_ROOT/.ai/scripts/verify_review_evidence.py" "$DIFF_PATH" "$REVIEW_MD" 2>"$EVIDENCE_LOG"; then
  log "✗ Review evidence verification failed"
  gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "needs-human-review" 2>/dev/null || true
  gh_with_timeout issue comment "$ISSUE_NUMBER" --body "## AWK Review blocked: evidence verification failed

審查內容缺少可核對的 \`EVIDENCE:\` 行，或 evidence 與 PR diff 不一致。

PR: #$PR_NUMBER
Diff Hash: \`$DIFF_HASH\`

Verifier output:
\`\`\`
$(tail -n 120 "$EVIDENCE_LOG" 2>/dev/null || echo '(no output)')
\`\`\`

Next step: 重新產生審查內容，並加入 \`EVIDENCE:\` 行（必須是 diff 中可直接搜尋到的字串）。" 2>/dev/null || true
  bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "blocked" "$CI_STATUS" "false" "" 2>/dev/null || true
  echo "RESULT=review_blocked"
  exit 0
fi

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
| Diff Bytes | \`$DIFF_BYTES\` |
| Evidence Lines | \`$EVIDENCE_COUNT\` |
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
      MERGE_TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
      bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "merged" "$CI_STATUS" "false" "$MERGE_TIMESTAMP" 2>/dev/null || true
      
      # 關閉 Issue
      gh_with_timeout issue close "$ISSUE_NUMBER" 2>/dev/null || true
      log "✓ Issue #$ISSUE_NUMBER 已關閉"
      
      # 移除 pr-ready 標籤
      gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" 2>/dev/null || true
      
      # 更新 tasks.md
      RESULT_FILE="$MAIN_ROOT/.ai/results/issue-$ISSUE_NUMBER.json"
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
      WT_DIR="$MAIN_ROOT/.worktrees/issue-$ISSUE_NUMBER"
      if [[ -d "$WT_DIR" ]]; then
        git worktree remove "$WT_DIR" --force 2>/dev/null || true
        log "✓ 已清理 worktree: $WT_DIR"
      fi
      
      echo "RESULT=merged"
    else
      MERGE_STATE_STATUS="$(gh_with_timeout pr view "$PR_NUMBER" --json mergeStateStatus --jq '.mergeStateStatus' 2>/dev/null || echo "unknown")"
      log "✗ PR 合併失敗 (mergeStateStatus: $MERGE_STATE_STATUS)"
      bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "merge_failed" "$CI_STATUS" "false" "" 2>/dev/null || true

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
    bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "approved_ci_failed" "$CI_STATUS" "false" "" 2>/dev/null || true
    
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
  bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "changes_requested" "$CI_STATUS" "false" "" 2>/dev/null || true
  
  echo "RESULT=changes_requested"
fi

log "✓ 審查提交完成"
