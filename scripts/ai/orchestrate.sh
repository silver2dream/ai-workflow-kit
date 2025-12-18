#!/bin/bash
set -euo pipefail

# ============================================================================
# orchestrate.sh - 設置 tmux 開發環境
# ============================================================================
# 用法:
#   bash scripts/ai/orchestrate.sh          # 創建 tmux session
#   bash scripts/ai/orchestrate.sh --attach # 創建並附加
# ============================================================================

SESSION="ai-dev-flow"
MONO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

ATTACH=false
for arg in "$@"; do
  case "$arg" in
    --attach|-a) ATTACH=true ;;
  esac
done

# 檢查 tmux
if ! command -v tmux &>/dev/null; then
  echo "ERROR: tmux 未安裝"
  exit 1
fi

# 如果 session 已存在，直接附加
if tmux has-session -t "$SESSION" 2>/dev/null; then
  echo "Session '$SESSION' 已存在"
  if [[ "$ATTACH" == "true" ]]; then
    tmux attach -t "$SESSION"
  else
    echo "執行: tmux attach -t $SESSION"
  fi
  exit 0
fi

# 創建新 session
tmux new-session -d -s "$SESSION" -n 'Main' -c "$MONO_ROOT"

# 主視窗：左側 Principal，右側 Stats
tmux split-window -h -t "$SESSION:Main" -c "$MONO_ROOT"

# 左側 (Principal)
tmux select-pane -t "$SESSION:Main.0"
tmux send-keys -t "$SESSION:Main.0" "cd '$MONO_ROOT'" C-m
tmux send-keys -t "$SESSION:Main.0" "# Principal - 執行 /start-work 或 bash scripts/ai/kickoff.sh" C-m
tmux send-keys -t "$SESSION:Main.0" "clear" C-m

# 右側 (Stats/Monitor)
tmux select-pane -t "$SESSION:Main.1"
tmux send-keys -t "$SESSION:Main.1" "cd '$MONO_ROOT'" C-m
tmux send-keys -t "$SESSION:Main.1" "# Monitor - 執行 watch 'bash scripts/ai/stats.sh'" C-m
tmux send-keys -t "$SESSION:Main.1" "clear" C-m

# 創建第二個視窗用於日誌
tmux new-window -t "$SESSION" -n 'Logs' -c "$MONO_ROOT"
tmux send-keys -t "$SESSION:Logs" "cd '$MONO_ROOT'" C-m
tmux send-keys -t "$SESSION:Logs" "# Logs - 執行 tail -f .ai/exe-logs/*.log" C-m

# 回到主視窗
tmux select-window -t "$SESSION:Main"
tmux select-pane -t "$SESSION:Main.0"

echo ""
echo "┌─────────────────────────────────────────────────────────────┐"
echo "│              AI Dev Flow tmux Session                       │"
echo "├─────────────────────────────────────────────────────────────┤"
echo "│  Session: $SESSION                                          │"
echo "│                                                             │"
echo "│  視窗 1 (Main):                                             │"
echo "│    左側: Principal - 執行工作流                             │"
echo "│    右側: Monitor - 查看統計                                 │"
echo "│                                                             │"
echo "│  視窗 2 (Logs):                                             │"
echo "│    查看執行日誌                                             │"
echo "│                                                             │"
echo "│  快捷鍵:                                                    │"
echo "│    Ctrl+b n  - 下一個視窗                                   │"
echo "│    Ctrl+b p  - 上一個視窗                                   │"
echo "│    Ctrl+b o  - 切換 pane                                    │"
echo "│    Ctrl+b d  - 分離 session                                 │"
echo "└─────────────────────────────────────────────────────────────┘"
echo ""

if [[ "$ATTACH" == "true" ]]; then
  tmux attach -t "$SESSION"
else
  echo "執行: tmux attach -t $SESSION"
fi
