#!/usr/bin/env bash
# Stop Work - 停止工作流並生成報告
# stderr: 所有 log
#
# 用法: bash stop_work.sh <EXIT_REASON>
# EXIT_REASON: all_tasks_complete | user_stopped | error_exit | max_failures |
#              escalation_triggered | interrupted | max_loop_reached |
#              max_consecutive_failures | contract_violation | none

set -euo pipefail

# ============================================================
# 初始化
# ============================================================
log() {
  echo "[PRINCIPAL] $(date +%H:%M:%S) | $*" >&2
}

EXIT_REASON="${1:-unknown}"

log "停止工作流"
log "退出原因: $EXIT_REASON"

# ============================================================
# 獲取 Session 信息
# ============================================================
PRINCIPAL_SESSION_ID="${PRINCIPAL_SESSION_ID:-}"
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
fi

if [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  log "Session ID: $PRINCIPAL_SESSION_ID"
  
  SESSION_FILE=".ai/state/principal/sessions/${PRINCIPAL_SESSION_ID}.json"
  
  if [[ -f "$SESSION_FILE" ]]; then
    SESSION_START=$(python3 -c "import json; print(json.load(open('$SESSION_FILE')).get('started_at',''))" 2>/dev/null || echo "")
    SESSION_ACTIONS=$(python3 -c "import json; print(len(json.load(open('$SESSION_FILE')).get('actions',[])))" 2>/dev/null || echo "0")
    
    log "Session 開始: $SESSION_START"
    log "執行動作數: $SESSION_ACTIONS"
  fi
fi

# ============================================================
# 統計工作流結果
# ============================================================
log "統計工作流結果..."

TOTAL_ISSUES=$(gh issue list --label "ai-task" --json number --jq '. | length' 2>/dev/null || echo "0")
OPEN_ISSUES=$(gh issue list --label "ai-task" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
CLOSED_ISSUES=$((TOTAL_ISSUES - OPEN_ISSUES))

IN_PROGRESS=$(gh issue list --label "in-progress" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
PR_READY=$(gh issue list --label "pr-ready" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
WORKER_FAILED=$(gh issue list --label "worker-failed" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
NEEDS_REVIEW=$(gh issue list --label "needs-human-review" --state open --json number --jq '. | length' 2>/dev/null || echo "0")

log "總 Issues: $TOTAL_ISSUES"
log "已關閉: $CLOSED_ISSUES"
log "進行中: $IN_PROGRESS"
log "待審查: $PR_READY"
log "Worker 失敗: $WORKER_FAILED"
log "需要人工: $NEEDS_REVIEW"

# ============================================================
# 生成報告
# ============================================================
log "生成報告..."

REPORT_FILE=".ai/state/workflow-report-$(date +%Y%m%d-%H%M%S).md"
mkdir -p .ai/state

cat > "$REPORT_FILE" << EOF
# AWK Workflow Report

**Generated**: $(date '+%Y-%m-%d %H:%M:%S')
**Session ID**: ${PRINCIPAL_SESSION_ID:-N/A}
**Exit Reason**: $EXIT_REASON

---

## Summary

- **Total Issues**: $TOTAL_ISSUES
- **Closed Issues**: $CLOSED_ISSUES
- **Open Issues**: $OPEN_ISSUES

### Open Issues Breakdown

| Status | Count |
|--------|-------|
| In Progress | $IN_PROGRESS |
| PR Ready | $PR_READY |
| Worker Failed | $WORKER_FAILED |
| Needs Human Review | $NEEDS_REVIEW |

---

## Exit Reason Details

EOF

case "$EXIT_REASON" in
  "all_tasks_complete")
    cat >> "$REPORT_FILE" << EOF
✓ **All tasks completed successfully!**

No further action required.
EOF
    ;;
  "user_stopped")
    cat >> "$REPORT_FILE" << EOF
⏸ **Workflow was stopped by user.**

To resume:
1. Remove stop marker: \`rm .ai/state/STOP\`
2. Run: \`awkit kickoff\`
EOF
    ;;
  "max_loop_reached")
    cat >> "$REPORT_FILE" << EOF
⚠ **Workflow stopped: maximum loop count reached (1000).**

This may indicate an infinite loop or stuck state. Please investigate.
EOF
    ;;
  "max_consecutive_failures")
    cat >> "$REPORT_FILE" << EOF
⚠ **Workflow stopped: too many consecutive failures.**

Please review failed issues and fix underlying problems.
EOF
    ;;
  "contract_violation")
    cat >> "$REPORT_FILE" << EOF
✗ **Workflow stopped: variable contract violation.**

A required variable was missing. Check analyze_next.sh output.
EOF
    ;;
  *)
    cat >> "$REPORT_FILE" << EOF
? **Workflow stopped for reason: $EXIT_REASON**

Please check logs for details.
EOF
    ;;
esac

cat >> "$REPORT_FILE" << EOF

---

## Next Steps

EOF

if [[ "$WORKER_FAILED" -gt 0 ]] || [[ "$NEEDS_REVIEW" -gt 0 ]]; then
  cat >> "$REPORT_FILE" << EOF
### ⚠ Attention Required

EOF
  [[ "$WORKER_FAILED" -gt 0 ]] && echo "- **$WORKER_FAILED** issues failed (worker-failed) - need investigation" >> "$REPORT_FILE"
  [[ "$NEEDS_REVIEW" -gt 0 ]] && echo "- **$NEEDS_REVIEW** issues need human review" >> "$REPORT_FILE"
  echo "" >> "$REPORT_FILE"
fi

if [[ "$PR_READY" -gt 0 ]]; then
  cat >> "$REPORT_FILE" << EOF
### PRs Ready for Review

There are **$PR_READY** PRs ready for review. Run \`awkit kickoff\` to continue processing.

EOF
fi

log "✓ 報告已生成：$REPORT_FILE"

# ============================================================
# 清理狀態
# ============================================================
log "清理狀態..."

# 清理 loop_count
rm -f .ai/state/loop_count 2>/dev/null || true

# 清理 consecutive_failures
rm -f .ai/state/consecutive_failures 2>/dev/null || true

log "✓ 狀態已清理"

# ============================================================
# 結束 Session
# ============================================================
if [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  log "結束 session..."
  
  bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON" 2>/dev/null || true
  
  log "✓ Session 已結束"
fi

# ============================================================
# 顯示摘要
# ============================================================
cat >&2 << EOF

==========================================
  AWK Workflow Stopped
==========================================

Exit Reason: $EXIT_REASON

Summary:
  - Total Issues: $TOTAL_ISSUES
  - Closed: $CLOSED_ISSUES
  - Open: $OPEN_ISSUES

EOF

if [[ "$WORKER_FAILED" -gt 0 ]] || [[ "$NEEDS_REVIEW" -gt 0 ]]; then
  cat >&2 << EOF
⚠ Attention Required:
EOF
  [[ "$WORKER_FAILED" -gt 0 ]] && echo "  - $WORKER_FAILED issues failed (worker-failed)" >&2
  [[ "$NEEDS_REVIEW" -gt 0 ]] && echo "  - $NEEDS_REVIEW issues need human review" >&2
  echo "" >&2
fi

cat >&2 << EOF
Report: $REPORT_FILE

==========================================
EOF

exit 0
