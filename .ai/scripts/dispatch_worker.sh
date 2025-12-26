#!/usr/bin/env bash
# Dispatch Worker - 派工給 Worker 執行 Issue
# stdout: 只輸出可 eval 的變數賦值
# stderr: 所有 log
#
# 用法: bash dispatch_worker.sh <ISSUE_NUMBER>
# 輸出變數: WORKER_STATUS (success | failed | in_progress)

set -euo pipefail

# ============================================================
# 初始化
# ============================================================
log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  echo "$msg" >> .ai/exe-logs/principal.log 2>/dev/null || true
}

ISSUE_NUMBER="${1:-}"
WORKER_STATUS="failed"

if [[ -z "$ISSUE_NUMBER" ]]; then
  log "✗ 缺少 Issue 編號"
  echo "WORKER_STATUS=$WORKER_STATUS"
  exit 1
fi

log "派工 Issue #$ISSUE_NUMBER"

# ============================================================
# 獲取 Session ID
# ============================================================
PRINCIPAL_SESSION_ID="${PRINCIPAL_SESSION_ID:-}"
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
fi

if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  log "✗ 無法獲取 Principal Session ID"
  echo "WORKER_STATUS=$WORKER_STATUS"
  exit 1
fi

log "Session ID: $PRINCIPAL_SESSION_ID"

# ============================================================
# 獲取並驗證 Issue 信息
# ============================================================
log "獲取 Issue 信息..."

ISSUE_DATA=$(gh issue view "$ISSUE_NUMBER" --json number,title,body,labels,state 2>&1) || {
  log "✗ 無法獲取 Issue 信息"
  echo "WORKER_STATUS=$WORKER_STATUS"
  exit 1
}

ISSUE_STATE=$(echo "$ISSUE_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('state',''))" 2>/dev/null || echo "")
ISSUE_LABELS=$(echo "$ISSUE_DATA" | python3 -c "import json,sys; print(','.join([l['name'] for l in json.load(sys.stdin).get('labels',[])]))" 2>/dev/null || echo "")

if [[ "$ISSUE_STATE" != "OPEN" ]]; then
  log "✗ Issue 不是 open 狀態：$ISSUE_STATE"
  echo "WORKER_STATUS=$WORKER_STATUS"
  exit 1
fi

if [[ ! "$ISSUE_LABELS" =~ "ai-task" ]]; then
  log "✗ Issue 沒有 ai-task 標籤"
  echo "WORKER_STATUS=$WORKER_STATUS"
  exit 1
fi

if [[ "$ISSUE_LABELS" =~ "in-progress" ]]; then
  log "⚠ Issue 已經在執行中"
  WORKER_STATUS="in_progress"
  echo "WORKER_STATUS=$WORKER_STATUS"
  exit 0
fi

log "✓ Issue 驗證通過"

# ============================================================
# 準備 Ticket 文件
# ============================================================
log "準備 ticket 文件..."

ISSUE_BODY=$(echo "$ISSUE_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('body',''))" 2>/dev/null || echo "")

TICKET_FILE=".ai/temp/ticket-$ISSUE_NUMBER.md"
mkdir -p .ai/temp

echo "$ISSUE_BODY" > "$TICKET_FILE"

log "✓ Ticket 文件已保存：$TICKET_FILE"

# 解析 ticket metadata
REPO=$(echo "$ISSUE_BODY" | grep -oP '(?<=\*\*Repo\*\*: )[^\n]+' | head -1 || echo "")
if [[ -z "$REPO" ]]; then
  REPO=$(echo "$ISSUE_BODY" | grep -oP '(?<=- Repo: )[^\n]+' | head -1 || echo "root")
fi

log "Repo: $REPO"

# ============================================================
# 標記 Issue 為 in-progress
# ============================================================
log "標記 Issue 為 in-progress..."

gh issue edit "$ISSUE_NUMBER" --add-label "in-progress" 2>/dev/null || true

log "✓ Issue 已標記為 in-progress"

# ============================================================
# 記錄 worker_dispatched
# ============================================================
log "記錄 worker_dispatched..."

bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "worker_dispatched" "{\"issue_id\":\"$ISSUE_NUMBER\",\"repo\":\"$REPO\"}" 2>/dev/null || true

log "✓ 已記錄 worker_dispatched"

# ============================================================
# 執行 Worker
# ============================================================
log "執行 Worker..."

REPOS_CLEAN=$(echo "$REPO" | tr -d ' ')

bash .ai/scripts/run_issue_codex.sh "$ISSUE_NUMBER" "$TICKET_FILE" "$REPOS_CLEAN" >&2
WORKER_EXIT_CODE=$?

log "Worker 執行完成 (exit code: $WORKER_EXIT_CODE)"

# ============================================================
# 檢查執行結果
# ============================================================
RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"

if [[ ! -f "$RESULT_FILE" ]]; then
  log "⚠ Result 文件不存在：$RESULT_FILE"
  WORKER_STATUS="failed"
else
  WORKER_STATUS=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('status','failed'))" 2>/dev/null || echo "failed")
  PR_URL=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('pr_url',''))" 2>/dev/null || echo "")
  
  log "Worker status: $WORKER_STATUS"
  
  if [[ -n "$PR_URL" ]]; then
    log "PR URL: $PR_URL"
  fi
fi

# ============================================================
# 處理結果
# ============================================================
if [[ "$WORKER_STATUS" == "success" ]] && [[ -n "$PR_URL" ]]; then
  log "✓ Worker 成功"
  
  # 更新 Issue 標籤
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" --add-label "pr-ready" 2>/dev/null || true
  
  log "✓ Issue 標籤已更新 (in-progress → pr-ready)"
  
  # 記錄 worker_completed
  bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "worker_completed" "{\"issue_id\":\"$ISSUE_NUMBER\",\"status\":\"success\",\"pr_url\":\"$PR_URL\"}" 2>/dev/null || true
  
  # 重置 consecutive_failures
  echo "0" > .ai/state/consecutive_failures 2>/dev/null || true
else
  log "✗ Worker 失敗"
  
  # 讀取失敗次數
  FAIL_COUNT_FILE=".ai/runs/issue-$ISSUE_NUMBER/fail_count.txt"
  mkdir -p ".ai/runs/issue-$ISSUE_NUMBER"
  
  FAIL_COUNT=0
  if [[ -f "$FAIL_COUNT_FILE" ]]; then
    FAIL_COUNT=$(cat "$FAIL_COUNT_FILE" 2>/dev/null || echo "0")
  fi
  
  FAIL_COUNT=$((FAIL_COUNT + 1))
  echo "$FAIL_COUNT" > "$FAIL_COUNT_FILE"
  
  log "失敗次數: $FAIL_COUNT / 3"
  
  # 更新 consecutive_failures
  CONSECUTIVE_FAILURES=$(cat .ai/state/consecutive_failures 2>/dev/null || echo "0")
  echo "$((CONSECUTIVE_FAILURES + 1))" > .ai/state/consecutive_failures
  
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
    
    bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "worker_failed" "{\"issue_id\":\"$ISSUE_NUMBER\",\"attempts\":$FAIL_COUNT}" 2>/dev/null || true
  else
    log "將在下一輪重試 (attempt $FAIL_COUNT/3)"
    
    gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" 2>/dev/null || true
    
    log "✓ 已移除 in-progress 標籤"
  fi
  
  WORKER_STATUS="failed"
fi

echo "WORKER_STATUS=$WORKER_STATUS"
exit 0
