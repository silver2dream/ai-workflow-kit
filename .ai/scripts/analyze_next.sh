#!/usr/bin/env bash
# Analyze Next - 決定下一個待處理的任務
# stdout: 只輸出可 eval 的變數賦值
# stderr: 所有 log
#
# 輸出變數:
#   NEXT_ACTION: generate_tasks | create_task | dispatch_worker | check_result | review_pr | all_complete | none
#   ISSUE_NUMBER, PR_NUMBER, SPEC_NAME, TASK_LINE, EXIT_REASON

set -euo pipefail

# ============================================================
# 初始化
# ============================================================
log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  echo "$msg" >&2
  printf '%s\n' "$msg" >> .ai/exe-logs/analyze_next.log 2>/dev/null || true
}

# 初始化輸出變數
NEXT_ACTION=""
ISSUE_NUMBER=""
PR_NUMBER=""
SPEC_NAME=""
TASK_LINE=""
EXIT_REASON=""

# ============================================================
# 讀取配置
# ============================================================
CONFIG_FILE=".ai/config/workflow.yaml"

if [[ ! -f "$CONFIG_FILE" ]]; then
  log "✗ workflow.yaml 不存在"
  NEXT_ACTION="none"
  EXIT_REASON="config_not_found"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

SPEC_BASE_PATH=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c.get('specs',{}).get('base_path', '.ai/specs'))" 2>/dev/null || echo ".ai/specs")
ACTIVE_SPECS=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(','.join(c.get('specs',{}).get('active', [])))" 2>/dev/null || echo "")
LABEL_TASK=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c.get('github',{}).get('labels',{}).get('task', 'ai-task'))" 2>/dev/null || echo "ai-task")
LABEL_IN_PROGRESS=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c.get('github',{}).get('labels',{}).get('in_progress', 'in-progress'))" 2>/dev/null || echo "in-progress")
LABEL_PR_READY=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c.get('github',{}).get('labels',{}).get('pr_ready', 'pr-ready'))" 2>/dev/null || echo "pr-ready")

log "配置已載入"

# ============================================================
# Loop Safety 檢查
# ============================================================
LOOP_COUNT_FILE=".ai/state/loop_count"
CONSECUTIVE_FAILURES_FILE=".ai/state/consecutive_failures"
MAX_LOOP=1000
MAX_CONSECUTIVE_FAILURES=5

# 先更新 loop_count（每次呼叫 analyze_next 就是一次 loop）
LOOP_COUNT=$(cat "$LOOP_COUNT_FILE" 2>/dev/null || echo "0")
LOOP_COUNT=$((LOOP_COUNT + 1))
echo "$LOOP_COUNT" > "$LOOP_COUNT_FILE"

CONSECUTIVE_FAILURES=$(cat "$CONSECUTIVE_FAILURES_FILE" 2>/dev/null || echo "0")

if [[ "$LOOP_COUNT" -ge "$MAX_LOOP" ]]; then
  log "✗ 達到最大循環次數 ($MAX_LOOP)"
  NEXT_ACTION="none"
  EXIT_REASON="max_loop_reached"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

if [[ "$CONSECUTIVE_FAILURES" -ge "$MAX_CONSECUTIVE_FAILURES" ]]; then
  log "✗ 連續失敗次數過多 ($CONSECUTIVE_FAILURES)"
  NEXT_ACTION="none"
  EXIT_REASON="max_consecutive_failures"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

# ============================================================
# Step 1: 檢查 in-progress Issues
# ============================================================
log "檢查 in-progress issues..."

IN_PROGRESS_ISSUES=$(gh issue list --label "$LABEL_IN_PROGRESS" --state open --json number --jq '.[].number' 2>/dev/null || echo "")

if [[ -n "$IN_PROGRESS_ISSUES" ]]; then
  ISSUE_NUMBER=$(echo "$IN_PROGRESS_ISSUES" | head -1)
  log "✓ 發現 in-progress issue: #$ISSUE_NUMBER"
  NEXT_ACTION="check_result"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

log "無 in-progress issues"

# ============================================================
# Step 2: 檢查 pr-ready Issues
# ============================================================
log "檢查 pr-ready issues..."

PR_READY_ISSUES=$(gh issue list --label "$LABEL_PR_READY" --state open --json number,body --jq '.[] | "\(.number)|\(.body)"' 2>/dev/null || echo "")

if [[ -n "$PR_READY_ISSUES" ]]; then
  ISSUE_LINE=$(echo "$PR_READY_ISSUES" | head -1)
  ISSUE_NUMBER=$(echo "$ISSUE_LINE" | cut -d'|' -f1)
  ISSUE_BODY=$(echo "$ISSUE_LINE" | cut -d'|' -f2-)
  
  log "✓ 發現 pr-ready issue: #$ISSUE_NUMBER"
  
  # 從 result.json 或 Issue body 提取 PR 編號
  RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"
  if [[ -f "$RESULT_FILE" ]]; then
    PR_URL=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('pr_url',''))" 2>/dev/null || echo "")
    if [[ -n "$PR_URL" ]]; then
      PR_NUMBER=$(echo "$PR_URL" | grep -oP '(?<=pull/)\d+' || echo "")
    fi
  fi
  
  if [[ -z "$PR_NUMBER" ]]; then
    PR_NUMBER=$(echo "$ISSUE_BODY" | grep -oP '(?<=#)\d+(?=\s|$)' | head -1 || echo "")
  fi
  
  if [[ -z "$PR_NUMBER" ]]; then
    PR_NUMBER=$(echo "$ISSUE_BODY" | grep -oP '(?<=pull/)\d+' | head -1 || echo "")
  fi
  
  if [[ -n "$PR_NUMBER" ]]; then
    log "✓ 提取到 PR 編號: #$PR_NUMBER"
    NEXT_ACTION="review_pr"
    echo "NEXT_ACTION=$NEXT_ACTION"
    echo "ISSUE_NUMBER=$ISSUE_NUMBER"
    echo "PR_NUMBER=$PR_NUMBER"
    echo "SPEC_NAME=$SPEC_NAME"
    echo "TASK_LINE=$TASK_LINE"
    echo "EXIT_REASON=$EXIT_REASON"
    exit 0
  else
    log "⚠ 無法提取 PR 編號，移除 pr-ready 標籤"
    gh issue edit "$ISSUE_NUMBER" --remove-label "$LABEL_PR_READY" 2>/dev/null || true
    ISSUE_NUMBER=""
  fi
fi

log "無 pr-ready issues"

# ============================================================
# Step 3: 檢查 pending Issues
# ============================================================
log "檢查 pending issues..."

PENDING_ISSUES=$(gh issue list --label "$LABEL_TASK" --state open --json number,labels --jq '.[] | select(.labels | map(.name) | (contains(["'"$LABEL_IN_PROGRESS"'"]) or contains(["'"$LABEL_PR_READY"'"])) | not) | .number' 2>/dev/null || echo "")

if [[ -n "$PENDING_ISSUES" ]]; then
  ISSUE_NUMBER=$(echo "$PENDING_ISSUES" | head -1)
  log "✓ 發現 pending issue: #$ISSUE_NUMBER"
  NEXT_ACTION="dispatch_worker"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

log "無 pending issues"

# ============================================================
# Step 4: 檢查 tasks.md 中的未完成任務
# ============================================================
log "檢查 tasks.md 中的未完成任務..."

if [[ -n "$ACTIVE_SPECS" ]]; then
  IFS=',' read -ra SPEC_LIST <<< "$ACTIVE_SPECS"
  
  for SPEC in "${SPEC_LIST[@]}"; do
    SPEC=$(echo "$SPEC" | tr -d ' ')
    [[ -z "$SPEC" ]] && continue
    
    TASKS_FILE="$SPEC_BASE_PATH/$SPEC/tasks.md"
    
    # 檢查 tasks.md 是否存在
    if [[ ! -f "$TASKS_FILE" ]]; then
      # 檢查 design.md 是否存在，如果存在則需要生成 tasks
      DESIGN_FILE="$SPEC_BASE_PATH/$SPEC/design.md"
      if [[ -f "$DESIGN_FILE" ]]; then
        log "✓ 發現需要生成 tasks: $SPEC"
        NEXT_ACTION="generate_tasks"
        SPEC_NAME="$SPEC"
        echo "NEXT_ACTION=$NEXT_ACTION"
        echo "ISSUE_NUMBER=$ISSUE_NUMBER"
        echo "PR_NUMBER=$PR_NUMBER"
        echo "SPEC_NAME=$SPEC_NAME"
        echo "TASK_LINE=$TASK_LINE"
        echo "EXIT_REASON=$EXIT_REASON"
        exit 0
      fi
      continue
    fi
    
    # 查找未完成且沒有 Issue 引用的任務
    UNCOMPLETED_TASK=$(grep -n '^\- \[ \] [0-9]' "$TASKS_FILE" | grep -v '<!-- Issue #' | head -1 || echo "")
    
    if [[ -n "$UNCOMPLETED_TASK" ]]; then
      TASK_LINE=$(echo "$UNCOMPLETED_TASK" | cut -d':' -f1)
      TASK_CONTENT=$(echo "$UNCOMPLETED_TASK" | cut -d':' -f2-)
      
      log "✓ 發現未完成任務：$SPEC (line $TASK_LINE)"
      log "  $TASK_CONTENT"
      
      NEXT_ACTION="create_task"
      SPEC_NAME="$SPEC"
      echo "NEXT_ACTION=$NEXT_ACTION"
      echo "ISSUE_NUMBER=$ISSUE_NUMBER"
      echo "PR_NUMBER=$PR_NUMBER"
      echo "SPEC_NAME=$SPEC_NAME"
      echo "TASK_LINE=$TASK_LINE"
      echo "EXIT_REASON=$EXIT_REASON"
      exit 0
    fi
  done
fi

log "無未完成任務"

# ============================================================
# Step 5: 檢查是否全部完成
# ============================================================
log "檢查是否全部完成..."

OPEN_TASK_COUNT=$(gh issue list --label "$LABEL_TASK" --state open --json number --jq '. | length' 2>/dev/null || echo "0")

if [[ "$OPEN_TASK_COUNT" -eq 0 ]]; then
  log "✓ 所有任務已完成！"
  NEXT_ACTION="all_complete"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

# ============================================================
# Step 6: 無待處理任務
# ============================================================
log "⚠ 無待處理任務（可能都需要人工審查）"
NEXT_ACTION="none"
EXIT_REASON="no_actionable_tasks"

echo "NEXT_ACTION=$NEXT_ACTION"
echo "ISSUE_NUMBER=$ISSUE_NUMBER"
echo "PR_NUMBER=$PR_NUMBER"
echo "SPEC_NAME=$SPEC_NAME"
echo "TASK_LINE=$TASK_LINE"
echo "EXIT_REASON=$EXIT_REASON"
exit 0
