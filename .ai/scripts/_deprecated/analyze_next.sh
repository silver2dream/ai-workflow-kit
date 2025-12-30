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
# Timeout helpers
# ============================================================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/timeout.sh"

# ============================================================
# Ensure we operate from the main worktree root (even if called inside a git worktree)
# ============================================================
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

MAIN_ROOT="$(resolve_main_root)"
cd "$MAIN_ROOT" 2>/dev/null || true

# ============================================================
# 初始化
# ============================================================
log() {
  local msg="[PRINCIPAL] $(date +%H:%M:%S) | $*"
  printf '%s\n' "$msg" >> .ai/exe-logs/principal.log 2>/dev/null || true
}

# 初始化輸出變數
NEXT_ACTION=""
ISSUE_NUMBER=""
PR_NUMBER=""
SPEC_NAME=""
TASK_LINE=""
EXIT_REASON=""

# ============================================================
# Session action recording (best-effort, no stdout)
# ============================================================
PRINCIPAL_SESSION_ID="${PRINCIPAL_SESSION_ID:-}"
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
fi

record_next_action() {
  local sid="${PRINCIPAL_SESSION_ID:-}"
  if [[ -z "$sid" ]]; then
    sid=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
  fi
  if [[ -z "$sid" ]]; then
    return 0
  fi

  export NEXT_ACTION ISSUE_NUMBER PR_NUMBER SPEC_NAME TASK_LINE EXIT_REASON

  local data
  data=$(python3 - <<'PY' 2>/dev/null || echo "{}"
import json
import os

print(json.dumps(
    {
        "next_action": os.environ.get("NEXT_ACTION", ""),
        "issue_number": os.environ.get("ISSUE_NUMBER", ""),
        "pr_number": os.environ.get("PR_NUMBER", ""),
        "spec_name": os.environ.get("SPEC_NAME", ""),
        "task_line": os.environ.get("TASK_LINE", ""),
        "exit_reason": os.environ.get("EXIT_REASON", ""),
    },
    ensure_ascii=True,
))
PY
  )

  bash .ai/scripts/session_manager.sh append_session_action "$sid" "next_action" "$data" 2>/dev/null || true
}

trap record_next_action EXIT

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
LABEL_WORKER_FAILED=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c.get('github',{}).get('labels',{}).get('worker_failed', 'worker-failed'))" 2>/dev/null || echo "worker-failed")
LABEL_NEEDS_REVIEW=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c.get('github',{}).get('labels',{}).get('needs_human_review', 'needs-human-review'))" 2>/dev/null || echo "needs-human-review")

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

IN_PROGRESS_ISSUES=$(gh_with_timeout issue list --label "$LABEL_IN_PROGRESS" --state open --json number --jq '.[].number' 2>/dev/null || echo "")

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

PR_READY_ISSUES=$(gh_with_timeout issue list --label "$LABEL_PR_READY" --state open --json number,body --jq '.[] | "\(.number)|\(.body)"' 2>/dev/null || echo "")

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
      PR_NUMBER=$(echo "$PR_URL" | sed -n 's|.*/pull/\([0-9]*\).*|\1|p')
    fi
  fi
  
  if [[ -z "$PR_NUMBER" ]]; then
    PR_NUMBER=$(echo "$ISSUE_BODY" | sed -n 's/.*#\([0-9][0-9]*\).*/\1/p' | head -1)
  fi
  
  if [[ -z "$PR_NUMBER" ]]; then
    PR_NUMBER=$(echo "$ISSUE_BODY" | sed -n 's|.*/pull/\([0-9]*\).*|\1|p' | head -1)
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
    gh_with_timeout issue edit "$ISSUE_NUMBER" --remove-label "$LABEL_PR_READY" 2>/dev/null || true
    ISSUE_NUMBER=""
  fi
fi

log "無 pr-ready issues"

# ============================================================
# Step 2.5: 檢查需要人工介入的 issues
# ============================================================
log "檢查需要人工介入的 issues..."

WORKER_FAILED_ISSUES=$(gh_with_timeout issue list --label "$LABEL_WORKER_FAILED" --state open --json number --jq '.[].number' 2>/dev/null || echo "")
if [[ -n "$WORKER_FAILED_ISSUES" ]]; then
  ISSUE_NUMBER=$(echo "$WORKER_FAILED_ISSUES" | head -1)
  log "✗ 發現 worker-failed issue: #$ISSUE_NUMBER"
  NEXT_ACTION="none"
  EXIT_REASON="worker_failed"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

NEEDS_REVIEW_ISSUES=$(gh_with_timeout issue list --label "$LABEL_NEEDS_REVIEW" --state open --json number --jq '.[].number' 2>/dev/null || echo "")
if [[ -n "$NEEDS_REVIEW_ISSUES" ]]; then
  ISSUE_NUMBER=$(echo "$NEEDS_REVIEW_ISSUES" | head -1)
  log "✗ 發現需要人工審查的 issue: #$ISSUE_NUMBER"
  NEXT_ACTION="none"
  EXIT_REASON="needs_human_review"
  echo "NEXT_ACTION=$NEXT_ACTION"
  echo "ISSUE_NUMBER=$ISSUE_NUMBER"
  echo "PR_NUMBER=$PR_NUMBER"
  echo "SPEC_NAME=$SPEC_NAME"
  echo "TASK_LINE=$TASK_LINE"
  echo "EXIT_REASON=$EXIT_REASON"
  exit 0
fi

# ============================================================
# Step 3: 檢查 pending Issues
# ============================================================
log "檢查 pending issues..."

PENDING_ISSUES=$(gh_with_timeout issue list --label "$LABEL_TASK" --state open --json number,labels --jq '.[] | select(.labels | map(.name) | (any(. == "'"$LABEL_IN_PROGRESS"'") or any(. == "'"$LABEL_PR_READY"'") or any(. == "'"$LABEL_WORKER_FAILED"'") or any(. == "'"$LABEL_NEEDS_REVIEW"'")) | not) | .number' 2>/dev/null || echo "")

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
    UNCOMPLETED_TASK=$(grep -n '^\- \[ \]' "$TASKS_FILE" | grep -v '<!-- Issue #' | head -1 || echo "")
    
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

OPEN_TASK_COUNT=$(gh_with_timeout issue list --label "$LABEL_TASK" --state open --json number --jq '. | length' 2>/dev/null || echo "0")

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
