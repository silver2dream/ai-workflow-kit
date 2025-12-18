#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# stats.sh - AI 工作流統計報告
# ============================================================================
# 用法:
#   bash scripts/ai/stats.sh              # 顯示統計
#   bash scripts/ai/stats.sh --json       # JSON 格式輸出
#   bash scripts/ai/stats.sh --html       # 生成 HTML 報告
# ============================================================================

MONO_ROOT="$(git rev-parse --show-toplevel)"
cd "$MONO_ROOT"

OUTPUT_FORMAT="text"
for arg in "$@"; do
  case "$arg" in
    --json) OUTPUT_FORMAT="json" ;;
    --html) OUTPUT_FORMAT="html" ;;
  esac
done

# ----------------------------------------------------------------------------
# 收集數據
# ----------------------------------------------------------------------------

# GitHub Issues 統計
ISSUES_TOTAL=$(gh issue list --label ai-task --state all --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
ISSUES_OPEN=$(gh issue list --label ai-task --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
ISSUES_CLOSED=$((ISSUES_TOTAL - ISSUES_OPEN))
ISSUES_FAILED=$(gh issue list --label worker-failed --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
ISSUES_IN_PROGRESS=$(gh issue list --label in-progress --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
ISSUES_PR_READY=$(gh issue list --label pr-ready --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

# GitHub PRs 統計
PRS_OPEN=$(gh pr list --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
PRS_MERGED=$(gh pr list --state merged --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

# 本地結果統計
RESULTS_DIR="$MONO_ROOT/.ai/results"
LOCAL_SUCCESS=0
LOCAL_FAILED=0
if [[ -d "$RESULTS_DIR" ]]; then
  LOCAL_SUCCESS=$(grep -l '"status": "success"' "$RESULTS_DIR"/*.json 2>/dev/null | wc -l | tr -d ' ')
  LOCAL_FAILED=$(grep -l '"status": "failed"' "$RESULTS_DIR"/*.json 2>/dev/null | wc -l | tr -d ' ')
fi

# 計算成功率
TOTAL_EXECUTED=$((LOCAL_SUCCESS + LOCAL_FAILED))
if [[ "$TOTAL_EXECUTED" -gt 0 ]]; then
  SUCCESS_RATE=$(python3 -c "print(f'{$LOCAL_SUCCESS / $TOTAL_EXECUTED * 100:.1f}')")
else
  SUCCESS_RATE="N/A"
fi

# 最近執行時間
KICKOFF_TIME="N/A"
if [[ -f "$MONO_ROOT/.ai/state/kickoff_time.txt" ]]; then
  KICKOFF_TIME=$(cat "$MONO_ROOT/.ai/state/kickoff_time.txt")
fi

# 停止狀態
STOP_STATUS="運行中"
if [[ -f "$MONO_ROOT/.ai/state/STOP" ]]; then
  STOP_STATUS="已停止"
fi

TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# ----------------------------------------------------------------------------
# 輸出
# ----------------------------------------------------------------------------

if [[ "$OUTPUT_FORMAT" == "json" ]]; then
  cat <<EOF
{
  "timestamp": "$TIMESTAMP",
  "status": "$STOP_STATUS",
  "last_kickoff": "$KICKOFF_TIME",
  "issues": {
    "total": $ISSUES_TOTAL,
    "open": $ISSUES_OPEN,
    "closed": $ISSUES_CLOSED,
    "in_progress": $ISSUES_IN_PROGRESS,
    "pr_ready": $ISSUES_PR_READY,
    "failed": $ISSUES_FAILED
  },
  "prs": {
    "open": $PRS_OPEN,
    "merged": $PRS_MERGED
  },
  "local_results": {
    "success": $LOCAL_SUCCESS,
    "failed": $LOCAL_FAILED,
    "success_rate": "$SUCCESS_RATE"
  }
}
EOF

elif [[ "$OUTPUT_FORMAT" == "html" ]]; then
  HTML_FILE="$MONO_ROOT/.ai/state/stats.html"
  cat > "$HTML_FILE" <<EOF
<!DOCTYPE html>
<html>
<head>
  <title>AI Workflow Stats</title>
  <meta charset="utf-8">
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; }
    h1 { color: #333; }
    .card { background: #f5f5f5; border-radius: 8px; padding: 20px; margin: 20px 0; }
    .stat { display: inline-block; margin: 10px 20px 10px 0; }
    .stat-value { font-size: 2em; font-weight: bold; color: #2563eb; }
    .stat-label { color: #666; font-size: 0.9em; }
    .success { color: #16a34a; }
    .warning { color: #ca8a04; }
    .error { color: #dc2626; }
    .timestamp { color: #999; font-size: 0.8em; }
  </style>
</head>
<body>
  <h1>🤖 AI Workflow 統計報告</h1>
  <p class="timestamp">生成時間: $TIMESTAMP | 狀態: $STOP_STATUS</p>
  
  <div class="card">
    <h2>📋 Issues</h2>
    <div class="stat"><span class="stat-value">$ISSUES_TOTAL</span><br><span class="stat-label">總計</span></div>
    <div class="stat"><span class="stat-value success">$ISSUES_CLOSED</span><br><span class="stat-label">已完成</span></div>
    <div class="stat"><span class="stat-value warning">$ISSUES_OPEN</span><br><span class="stat-label">待處理</span></div>
    <div class="stat"><span class="stat-value error">$ISSUES_FAILED</span><br><span class="stat-label">失敗</span></div>
  </div>
  
  <div class="card">
    <h2>🔀 Pull Requests</h2>
    <div class="stat"><span class="stat-value">$PRS_OPEN</span><br><span class="stat-label">待審查</span></div>
    <div class="stat"><span class="stat-value success">$PRS_MERGED</span><br><span class="stat-label">已合併</span></div>
  </div>
  
  <div class="card">
    <h2>📊 執行統計</h2>
    <div class="stat"><span class="stat-value success">$LOCAL_SUCCESS</span><br><span class="stat-label">成功</span></div>
    <div class="stat"><span class="stat-value error">$LOCAL_FAILED</span><br><span class="stat-label">失敗</span></div>
    <div class="stat"><span class="stat-value">$SUCCESS_RATE%</span><br><span class="stat-label">成功率</span></div>
  </div>
  
  <p class="timestamp">上次啟動: $KICKOFF_TIME</p>
</body>
</html>
EOF
  echo "HTML 報告已生成: $HTML_FILE"

else
  # Text 格式
  echo ""
  echo "═══════════════════════════════════════════════════════════════"
  echo "                   🤖 AI Workflow 統計報告"
  echo "═══════════════════════════════════════════════════════════════"
  echo ""
  echo "  生成時間: $TIMESTAMP"
  echo "  狀態: $STOP_STATUS"
  echo "  上次啟動: $KICKOFF_TIME"
  echo ""
  echo "───────────────────────────────────────────────────────────────"
  echo "  📋 Issues"
  echo "───────────────────────────────────────────────────────────────"
  echo "    總計:       $ISSUES_TOTAL"
  echo "    已完成:     $ISSUES_CLOSED"
  echo "    待處理:     $ISSUES_OPEN"
  echo "      - 進行中: $ISSUES_IN_PROGRESS"
  echo "      - PR就緒: $ISSUES_PR_READY"
  echo "    失敗:       $ISSUES_FAILED"
  echo ""
  echo "───────────────────────────────────────────────────────────────"
  echo "  🔀 Pull Requests"
  echo "───────────────────────────────────────────────────────────────"
  echo "    待審查:     $PRS_OPEN"
  echo "    已合併:     $PRS_MERGED"
  echo ""
  echo "───────────────────────────────────────────────────────────────"
  echo "  📊 本地執行統計"
  echo "───────────────────────────────────────────────────────────────"
  echo "    成功:       $LOCAL_SUCCESS"
  echo "    失敗:       $LOCAL_FAILED"
  echo "    成功率:     $SUCCESS_RATE%"
  echo ""
  echo "═══════════════════════════════════════════════════════════════"
  echo ""
  
  # 顯示需要關注的項目
  if [[ "$ISSUES_FAILED" -gt 0 ]] || [[ "$PRS_OPEN" -gt 0 ]]; then
    echo "⚠️  需要關注:"
    if [[ "$ISSUES_FAILED" -gt 0 ]]; then
      echo "    - $ISSUES_FAILED 個失敗的 issues: gh issue list --label worker-failed"
    fi
    if [[ "$PRS_OPEN" -gt 0 ]]; then
      echo "    - $PRS_OPEN 個待審查的 PRs: gh pr list"
    fi
    echo ""
  fi
fi
