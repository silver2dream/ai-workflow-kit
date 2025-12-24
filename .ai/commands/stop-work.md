# Stop Work Command

停止工作流並生成報告。

**用途：**
- 在 start-work.md 結束時自動調用
- 可獨立執行：`/stop-work [EXIT_REASON]`

**參數：**
- `[EXIT_REASON]`: 退出原因（可選）
  - `all_tasks_complete` - 所有任務完成
  - `user_stopped` - 用戶手動停止
  - `error_exit` - 錯誤退出
  - `max_failures` - 達到最大失敗次數
  - `escalation_triggered` - 升級觸發
  - `interrupted` - 被中斷

**輸出：**
- 生成工作流報告
- 結束 Principal session
- 返回 0

---

## Step 1: 確定退出原因

```bash
# 確定退出原因
if [[ -z "$EXIT_REASON" ]]; then
  if [[ -n "$1" ]]; then
    EXIT_REASON="$1"
  else
    EXIT_REASON="unknown"
  fi
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 停止工作流"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 退出原因: $EXIT_REASON"
```

---

## Step 2: 獲取 Session 信息

```bash
# 獲取 Principal session ID
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
fi

if [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | Session ID: $PRINCIPAL_SESSION_ID"
  
  # 讀取 session 數據
  SESSION_FILE=".ai/state/sessions/principal-$PRINCIPAL_SESSION_ID.json"
  
  if [[ -f "$SESSION_FILE" ]]; then
    SESSION_START=$(python3 -c "import json; print(json.load(open('$SESSION_FILE')).get('started_at',''))" 2>/dev/null || echo "")
    SESSION_ACTIONS=$(python3 -c "import json; print(len(json.load(open('$SESSION_FILE')).get('actions',[])))" 2>/dev/null || echo "0")
    
    echo "[PRINCIPAL] $(date +%H:%M:%S) | Session 開始: $SESSION_START"
    echo "[PRINCIPAL] $(date +%H:%M:%S) | 執行動作數: $SESSION_ACTIONS"
  fi
fi
```

---

## Step 3: 統計工作流結果

```bash
# 統計 Issues 和 PRs
echo "[PRINCIPAL] $(date +%H:%M:%S) | 統計工作流結果..."

TOTAL_ISSUES=$(gh issue list --label "ai-task" --json number --jq '. | length' 2>/dev/null || echo "0")
OPEN_ISSUES=$(gh issue list --label "ai-task" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
CLOSED_ISSUES=$((TOTAL_ISSUES - OPEN_ISSUES))

IN_PROGRESS=$(gh issue list --label "in-progress" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
PR_READY=$(gh issue list --label "pr-ready" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
WORKER_FAILED=$(gh issue list --label "worker-failed" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
NEEDS_REVIEW=$(gh issue list --label "needs-human-review" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
SECURITY_REVIEW=$(gh issue list --label "security-review" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
CI_FAILED=$(gh issue list --label "ci-failed" --state open --json number --jq '. | length' 2>/dev/null || echo "0")
CI_TIMEOUT=$(gh issue list --label "ci-timeout" --state open --json number --jq '. | length' 2>/dev/null || echo "0")

echo "[PRINCIPAL] $(date +%H:%M:%S) | 總 Issues: $TOTAL_ISSUES"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 已關閉: $CLOSED_ISSUES"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 進行中: $IN_PROGRESS"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 待審查: $PR_READY"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Worker 失敗: $WORKER_FAILED"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 需要人工: $NEEDS_REVIEW"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 安全審查: $SECURITY_REVIEW"
echo "[PRINCIPAL] $(date +%H:%M:%S) | CI 失敗: $CI_FAILED"
echo "[PRINCIPAL] $(date +%H:%M:%S) | CI 超時: $CI_TIMEOUT"
```


---

## Step 4: 生成報告

```bash
# 生成工作流報告
echo "[PRINCIPAL] $(date +%H:%M:%S) | 生成報告..."

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
| Security Review | $SECURITY_REVIEW |
| CI Failed | $CI_FAILED |
| CI Timeout | $CI_TIMEOUT |

---

## Session Actions

EOF

if [[ -n "$PRINCIPAL_SESSION_ID" ]] && [[ -f "$SESSION_FILE" ]]; then
  python3 << PYEOF >> "$REPORT_FILE"
import json
try:
    session = json.load(open('$SESSION_FILE'))
    actions = session.get('actions', [])
    
    if actions:
        for action in actions:
            action_type = action.get('type', 'unknown')
            timestamp = action.get('timestamp', '')
            data = action.get('data', {})
            print(f"- **{action_type}** at {timestamp}")
            for key, value in data.items():
                print(f"  - {key}: {value}")
    else:
        print("No actions recorded.")
except Exception as e:
    print(f"Unable to read session actions: {e}")
PYEOF
else
  echo "No session data available." >> "$REPORT_FILE"
fi

cat >> "$REPORT_FILE" << EOF

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
  "error_exit"|"max_failures")
    cat >> "$REPORT_FILE" << EOF
✗ **Workflow stopped due to errors.**

Please review:
1. Failed issues (label: worker-failed)
2. Issues needing human review (label: needs-human-review)
3. Execution logs in \`.ai/exe-logs/\`

After fixing issues, run: \`awkit kickoff\`
EOF
    ;;
  "escalation_triggered")
    cat >> "$REPORT_FILE" << EOF
⚠ **Workflow stopped due to escalation trigger.**

Please review:
1. Issues with security-review label
2. Issues with needs-human-review label
3. Check escalation triggers in workflow.yaml

After review, run: \`awkit kickoff\`
EOF
    ;;
  "interrupted")
    cat >> "$REPORT_FILE" << EOF
⚠ **Workflow was interrupted.**

The previous session was interrupted. Please check:
1. Any in-progress issues
2. Partial changes that may need cleanup

To resume, run: \`awkit kickoff\`
EOF
    ;;
  *)
    cat >> "$REPORT_FILE" << EOF
? **Workflow stopped for unknown reason.**

Please check:
1. Open issues status
2. Session logs
3. Execution logs

To resume, run: \`awkit kickoff\`
EOF
    ;;
esac

cat >> "$REPORT_FILE" << EOF

---

## Rollback Information

If you need to rollback any merged PRs:

\`\`\`bash
# Rollback a specific PR
bash .ai/scripts/rollback.sh <PR_NUMBER>

# Preview rollback (dry-run)
bash .ai/scripts/rollback.sh <PR_NUMBER> --dry-run
\`\`\`

**rollback.sh will:**
1. Get the merge commit of the PR
2. Create a revert commit
3. Create a revert PR
4. Reopen the original issue (if linked)
5. Send notifications

---

## Next Steps

EOF

if [[ "$WORKER_FAILED" -gt 0 ]] || [[ "$NEEDS_REVIEW" -gt 0 ]] || [[ "$SECURITY_REVIEW" -gt 0 ]]; then
  cat >> "$REPORT_FILE" << EOF
### ⚠ Attention Required

EOF
  [[ "$WORKER_FAILED" -gt 0 ]] && echo "- **$WORKER_FAILED** issues failed (worker-failed) - need investigation" >> "$REPORT_FILE"
  [[ "$NEEDS_REVIEW" -gt 0 ]] && echo "- **$NEEDS_REVIEW** issues need human review" >> "$REPORT_FILE"
  [[ "$SECURITY_REVIEW" -gt 0 ]] && echo "- **$SECURITY_REVIEW** issues need security review" >> "$REPORT_FILE"
  [[ "$CI_FAILED" -gt 0 ]] && echo "- **$CI_FAILED** issues have CI failures" >> "$REPORT_FILE"
  [[ "$CI_TIMEOUT" -gt 0 ]] && echo "- **$CI_TIMEOUT** issues have CI timeouts" >> "$REPORT_FILE"
  echo "" >> "$REPORT_FILE"
fi

if [[ "$PR_READY" -gt 0 ]]; then
  cat >> "$REPORT_FILE" << EOF
### PRs Ready for Review

There are **$PR_READY** PRs ready for review. Run \`awkit kickoff\` to continue processing.

EOF
fi

if [[ "$IN_PROGRESS" -gt 0 ]]; then
  cat >> "$REPORT_FILE" << EOF
### In Progress

There are **$IN_PROGRESS** issues currently in progress. Check their status before resuming.

EOF
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 報告已生成：$REPORT_FILE"
```

---

## Step 5: 結束 Session (Req 1.6)

```bash
# 結束 Principal session
if [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 結束 session..."
  
  bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON"
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Session 已結束"
fi
```

---

## Step 6: 顯示摘要

```bash
# 顯示摘要
echo ""
echo "=========================================="
echo "  AWK Workflow Stopped"
echo "=========================================="
echo ""
echo "Exit Reason: $EXIT_REASON"
echo ""
echo "Summary:"
echo "  - Total Issues: $TOTAL_ISSUES"
echo "  - Closed: $CLOSED_ISSUES"
echo "  - Open: $OPEN_ISSUES"
echo ""

if [[ "$WORKER_FAILED" -gt 0 ]] || [[ "$NEEDS_REVIEW" -gt 0 ]] || [[ "$SECURITY_REVIEW" -gt 0 ]]; then
  echo "⚠ Attention Required:"
  [[ "$WORKER_FAILED" -gt 0 ]] && echo "  - $WORKER_FAILED issues failed (worker-failed)"
  [[ "$NEEDS_REVIEW" -gt 0 ]] && echo "  - $NEEDS_REVIEW issues need human review"
  [[ "$SECURITY_REVIEW" -gt 0 ]] && echo "  - $SECURITY_REVIEW issues need security review"
  [[ "$CI_FAILED" -gt 0 ]] && echo "  - $CI_FAILED issues have CI failures"
  [[ "$CI_TIMEOUT" -gt 0 ]] && echo "  - $CI_TIMEOUT issues have CI timeouts"
  echo ""
fi

echo "Report: $REPORT_FILE"
echo ""
echo "=========================================="

exit 0
```

---

## 使用範例

### 從 start-work.md 調用

```bash
# 所有任務完成
EXIT_REASON="all_tasks_complete" source .ai/commands/stop-work.md

# 用戶停止
EXIT_REASON="user_stopped" source .ai/commands/stop-work.md

# 錯誤退出
EXIT_REASON="error_exit" source .ai/commands/stop-work.md

# 升級觸發
EXIT_REASON="escalation_triggered" source .ai/commands/stop-work.md
```

### 獨立執行

```bash
# 手動停止
bash .ai/commands/stop-work.md user_stopped

# 無參數（unknown reason）
bash .ai/commands/stop-work.md
```

---

## 依賴項

- `gh` CLI (GitHub CLI)
- `python3` with `json` module
- `.ai/scripts/session_manager.sh`

---

## 輸出文件

- `.ai/state/workflow-report-<timestamp>.md` - 工作流報告

---

## 注意事項

1. **Session 結束**：此命令會結束 Principal session，之後無法繼續使用該 session
2. **報告生成**：每次停止都會生成一個新的報告文件
3. **狀態保留**：Issue 和 PR 的狀態會保留，可以在下次啟動時繼續
4. **重新啟動**：要重新啟動工作流，運行 `awkit kickoff`
5. **Rollback**：報告中包含 rollback 說明，方便回滾已合併的 PR
