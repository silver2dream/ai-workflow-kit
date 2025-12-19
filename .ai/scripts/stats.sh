#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# stats.sh - AI 工作流統計報告
# ============================================================================
# 用法:
#   bash .ai/scripts/stats.sh              # 顯示統計
#   bash .ai/scripts/stats.sh --json       # JSON 格式輸出
#   bash .ai/scripts/stats.sh --html       # 生成 HTML 報告
#   bash .ai/scripts/stats.sh --no-save    # 不保存歷史記錄
# ============================================================================

MONO_ROOT="$(git rev-parse --show-toplevel)"
cd "$MONO_ROOT"

OUTPUT_FORMAT="text"
SAVE_HISTORY="true"
for arg in "$@"; do
  case "$arg" in
    --json) OUTPUT_FORMAT="json" ;;
    --html) OUTPUT_FORMAT="html" ;;
    --no-save) SAVE_HISTORY="false" ;;
  esac
done

# History file path
HISTORY_FILE="$MONO_ROOT/.ai/state/stats_history.jsonl"

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
TOTAL_DURATION=0
AVG_DURATION=0
if [[ -d "$RESULTS_DIR" ]]; then
  LOCAL_SUCCESS=$(grep -l '"status": "success"' "$RESULTS_DIR"/*.json 2>/dev/null | wc -l | tr -d ' ')
  LOCAL_FAILED=$(grep -l '"status": "failed"' "$RESULTS_DIR"/*.json 2>/dev/null | wc -l | tr -d ' ')
  
  # Calculate total and average duration from metrics
  METRICS_DATA=$(python3 - "$RESULTS_DIR" <<'PYTHON_SCRIPT'
import json
import os
import sys

results_dir = sys.argv[1]
total_duration = 0
count = 0

try:
    for f in os.listdir(results_dir):
        if f.endswith('.json'):
            try:
                with open(os.path.join(results_dir, f)) as fp:
                    data = json.load(fp)
                    metrics = data.get('metrics', {})
                    duration = metrics.get('duration_seconds', 0)
                    if duration > 0:
                        total_duration += duration
                        count += 1
            except:
                continue
except:
    pass

avg = round(total_duration / count, 1) if count > 0 else 0
print(f"{total_duration},{avg},{count}")
PYTHON_SCRIPT
  )
  TOTAL_DURATION=$(echo "$METRICS_DATA" | cut -d',' -f1)
  AVG_DURATION=$(echo "$METRICS_DATA" | cut -d',' -f2)
  METRICS_COUNT=$(echo "$METRICS_DATA" | cut -d',' -f3)
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
# 歷史趨勢計算
# ----------------------------------------------------------------------------

calculate_trends() {
  if [[ ! -f "$HISTORY_FILE" ]] || [[ ! -s "$HISTORY_FILE" ]]; then
    echo '{"daily_avg_closed":"N/A","success_rate_7d":"N/A","avg_time_to_merge":"N/A","data_points":0}'
    return
  fi
  
  python3 - "$HISTORY_FILE" <<'PYTHON_SCRIPT'
import json
import sys
from datetime import datetime, timedelta

history_file = sys.argv[1]
records = []

try:
    with open(history_file, 'r') as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    records.append(json.loads(line))
                except json.JSONDecodeError:
                    continue
except Exception:
    pass

if not records:
    print(json.dumps({"daily_avg_closed":"N/A","success_rate_7d":"N/A","avg_time_to_merge":"N/A","data_points":0}))
    sys.exit(0)

# Filter last 7 days
now = datetime.utcnow()
seven_days_ago = now - timedelta(days=7)
recent = []
for r in records:
    try:
        ts = datetime.fromisoformat(r.get("timestamp", "").replace("Z", "+00:00").replace("+00:00", ""))
        if ts >= seven_days_ago.replace(tzinfo=None):
            recent.append(r)
    except:
        continue

data_points = len(recent)

# Daily average closed
if data_points >= 2:
    first_closed = recent[0].get("issues", {}).get("closed", 0)
    last_closed = recent[-1].get("issues", {}).get("closed", 0)
    days_span = max(1, (data_points - 1))
    daily_avg = round((last_closed - first_closed) / days_span, 2)
    daily_avg_closed = str(daily_avg) if daily_avg >= 0 else "N/A"
else:
    daily_avg_closed = "N/A"

# Success rate (7d)
total_success = 0
total_failed = 0
for r in recent:
    lr = r.get("local_results", {})
    total_success += lr.get("success", 0)
    total_failed += lr.get("failed", 0)

if total_success + total_failed > 0:
    rate = round(total_success / (total_success + total_failed) * 100, 1)
    success_rate_7d = f"{rate}%"
else:
    success_rate_7d = "N/A"

# Average time to merge (placeholder - would need PR data)
avg_time_to_merge = "N/A"

result = {
    "daily_avg_closed": daily_avg_closed,
    "success_rate_7d": success_rate_7d,
    "avg_time_to_merge": avg_time_to_merge,
    "data_points": data_points
}
print(json.dumps(result))
PYTHON_SCRIPT
}

TRENDS_JSON=$(calculate_trends)
DAILY_AVG_CLOSED=$(echo "$TRENDS_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('daily_avg_closed','N/A'))")
SUCCESS_RATE_7D=$(echo "$TRENDS_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('success_rate_7d','N/A'))")
AVG_TIME_TO_MERGE=$(echo "$TRENDS_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('avg_time_to_merge','N/A'))")
TREND_DATA_POINTS=$(echo "$TRENDS_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('data_points',0))")

# ----------------------------------------------------------------------------
# 保存歷史記錄
# ----------------------------------------------------------------------------

save_history() {
  mkdir -p "$(dirname "$HISTORY_FILE")"
  
  # Create JSON record
  local record
  record=$(cat <<EOF
{"timestamp":"$TIMESTAMP","issues":{"total":$ISSUES_TOTAL,"open":$ISSUES_OPEN,"closed":$ISSUES_CLOSED,"in_progress":$ISSUES_IN_PROGRESS,"pr_ready":$ISSUES_PR_READY,"failed":$ISSUES_FAILED},"prs":{"open":$PRS_OPEN,"merged":$PRS_MERGED},"local_results":{"success":$LOCAL_SUCCESS,"failed":$LOCAL_FAILED}}
EOF
)
  
  # Append to history file
  echo "$record" >> "$HISTORY_FILE"
}

if [[ "$SAVE_HISTORY" == "true" ]]; then
  save_history
fi

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
    "success_rate": "$SUCCESS_RATE",
    "total_duration_seconds": $TOTAL_DURATION,
    "avg_duration_seconds": $AVG_DURATION
  },
  "trends": {
    "daily_avg_closed": "$DAILY_AVG_CLOSED",
    "success_rate_7d": "$SUCCESS_RATE_7D",
    "avg_time_to_merge": "$AVG_TIME_TO_MERGE",
    "data_points": $TREND_DATA_POINTS
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
    <div class="stat"><span class="stat-value">${TOTAL_DURATION}s</span><br><span class="stat-label">總執行時間</span></div>
    <div class="stat"><span class="stat-value">${AVG_DURATION}s</span><br><span class="stat-label">平均執行時間</span></div>
  </div>
  
  <div class="card">
    <h2>📈 趨勢 (7天)</h2>
    <div class="stat"><span class="stat-value">$DAILY_AVG_CLOSED</span><br><span class="stat-label">日均完成</span></div>
    <div class="stat"><span class="stat-value">$SUCCESS_RATE_7D</span><br><span class="stat-label">7天成功率</span></div>
    <div class="stat"><span class="stat-value">$AVG_TIME_TO_MERGE</span><br><span class="stat-label">平均合併時間</span></div>
    <div class="stat"><span class="stat-value">$TREND_DATA_POINTS</span><br><span class="stat-label">數據點</span></div>
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
  echo "    總執行時間: ${TOTAL_DURATION}s"
  echo "    平均執行時間: ${AVG_DURATION}s"
  echo ""
  echo "───────────────────────────────────────────────────────────────"
  echo "  📈 趨勢 (7天)"
  echo "───────────────────────────────────────────────────────────────"
  echo "    日均完成:     $DAILY_AVG_CLOSED"
  echo "    7天成功率:    $SUCCESS_RATE_7D"
  echo "    平均合併時間: $AVG_TIME_TO_MERGE"
  echo "    數據點:       $TREND_DATA_POINTS"
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
