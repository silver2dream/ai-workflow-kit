#!/usr/bin/env bash
# Check Result - 檢查 Worker 執行結果
# stdout: 只輸出可 eval 的變數賦值
# stderr: 所有 log
#
# 用法: bash check_result.sh <ISSUE_NUMBER>
# 輸出變數: CHECK_RESULT_STATUS, WORKER_STATUS, PR_NUMBER

set -euo pipefail

# ============================================================
# 初始化
# ============================================================
log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  echo "$msg" >> .ai/exe-logs/principal.log 2>/dev/null || true
}

ISSUE_NUMBER="${1:-}"
CHECK_RESULT_STATUS="not_found"
WORKER_STATUS="not_found"
PR_NUMBER=""

if [[ -z "$ISSUE_NUMBER" ]]; then
  log "✗ 缺少 Issue 編號"
  echo "CHECK_RESULT_STATUS=$CHECK_RESULT_STATUS"
  echo "WORKER_STATUS=$WORKER_STATUS"
  echo "PR_NUMBER=$PR_NUMBER"
  exit 1
fi

log "檢查 Issue #$ISSUE_NUMBER 的執行結果..."

# ============================================================
# 獲取 Session ID
# ============================================================
PRINCIPAL_SESSION_ID="${PRINCIPAL_SESSION_ID:-}"
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
fi

if [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  log "Session: $PRINCIPAL_SESSION_ID"
fi

# ============================================================
# 讀取 result.json
# ============================================================
RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"

if [[ ! -f "$RESULT_FILE" ]]; then
  log "⚠ Result 文件不存在: $RESULT_FILE"
  echo "CHECK_RESULT_STATUS=$CHECK_RESULT_STATUS"
  echo "WORKER_STATUS=$WORKER_STATUS"
  echo "PR_NUMBER=$PR_NUMBER"
  exit 0
fi

log "讀取 result 文件: $RESULT_FILE"

# ============================================================
# 提取結果信息
# ============================================================
WORKER_SESSION_ID=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('session',{}).get('worker_session_id',''))" 2>/dev/null || echo "")
if [[ -z "$WORKER_SESSION_ID" ]]; then
  WORKER_SESSION_ID=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('worker_session_id',''))" 2>/dev/null || echo "")
fi

WORKER_STATUS=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('status',''))" 2>/dev/null || echo "")
PR_URL=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('pr_url',''))" 2>/dev/null || echo "")

log "Worker Session: $WORKER_SESSION_ID"
log "Status: $WORKER_STATUS"
log "PR URL: $PR_URL"

# ============================================================
# 記錄 Worker 完成
# ============================================================
if [[ -n "$WORKER_SESSION_ID" ]] && [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  bash .ai/scripts/session_manager.sh update_worker_completion "$PRINCIPAL_SESSION_ID" "$ISSUE_NUMBER" "$WORKER_SESSION_ID" "$WORKER_STATUS" "$PR_URL" 2>/dev/null || true
  log "✓ 已記錄 worker completion"
fi

# ============================================================
# 更新 Principal Session ID
# ============================================================
if [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  bash .ai/scripts/session_manager.sh update_result_with_principal_session "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" 2>/dev/null || true
  log "✓ 已更新 result.json 的 principal_session_id"
fi

# ============================================================
# 處理成功情況
# ============================================================
if [[ "$WORKER_STATUS" == "success" ]] && [[ -n "$PR_URL" ]]; then
  log "✓ Worker 成功，PR 已創建: $PR_URL"
  
  # 提取 PR 編號
  PR_NUMBER=$(echo "$PR_URL" | sed -n 's|.*/pull/\([0-9]*\).*|\1|p')
  
  if [[ -n "$PR_NUMBER" ]]; then
    log "PR Number: #$PR_NUMBER"
  fi
  
  # 更新 issue 標籤
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" --add-label "pr-ready" 2>/dev/null || true
  
  log "✓ Issue 標籤已更新 (in-progress → pr-ready)"
  
  CHECK_RESULT_STATUS="success"
  
  # 重置 consecutive_failures
  echo "0" > .ai/state/consecutive_failures 2>/dev/null || true
  
  echo "CHECK_RESULT_STATUS=$CHECK_RESULT_STATUS"
  echo "WORKER_STATUS=$WORKER_STATUS"
  echo "PR_NUMBER=$PR_NUMBER"
  exit 0
fi

# ============================================================
# 處理失敗情況
# ============================================================
log "✗ Worker 失敗或無 PR 創建"

# 讀取失敗次數（由 attempt_guard.sh 管理，這裡只讀取）
FAIL_COUNT_FILE=".ai/runs/issue-$ISSUE_NUMBER/fail_count.txt"
mkdir -p ".ai/runs/issue-$ISSUE_NUMBER"

FAIL_COUNT=0
if [[ -f "$FAIL_COUNT_FILE" ]]; then
  FAIL_COUNT=$(cat "$FAIL_COUNT_FILE" 2>/dev/null || echo "0")
fi

# 注意：不要在這裡遞增 fail_count，attempt_guard.sh 已經處理了
# 注意：不要在這裡遞增 consecutive_failures，dispatch_worker.sh 已經處理了

log "失敗次數: $FAIL_COUNT / 3"

if [[ "$FAIL_COUNT" -ge 3 ]]; then
  log "✗ 達到最大重試次數 (3)"
  
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" --add-label "worker-failed" 2>/dev/null || true
  
  gh issue comment "$ISSUE_NUMBER" --body "Worker 已失敗 3 次，需要人工介入。

請檢查：
1. 任務描述是否清晰
2. 是否有技術難點
3. 是否需要調整任務範圍

執行日誌位置：\`.ai/runs/issue-$ISSUE_NUMBER/\`" 2>/dev/null || true
  
  log "✓ 已標記為 worker-failed"
  
  CHECK_RESULT_STATUS="failed_max_retries"
else
  log "將在下一輪重試 (attempt $FAIL_COUNT/3)"
  
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" 2>/dev/null || true
  
  CHECK_RESULT_STATUS="failed_will_retry"
fi

WORKER_STATUS="failed"

echo "CHECK_RESULT_STATUS=$CHECK_RESULT_STATUS"
echo "WORKER_STATUS=$WORKER_STATUS"
echo "PR_NUMBER=$PR_NUMBER"
exit 0
