#!/bin/bash
set -euo pipefail

# ============================================================================
# orchestrate.sh - 閮剔蔭 tmux ??啣?
# ============================================================================
# ?冽?:
#   bash scripts/ai/orchestrate.sh          # ?萄遣 tmux session
#   bash scripts/ai/orchestrate.sh --attach # ?萄遣銝阡???# ============================================================================

SESSION="ai-dev-flow"
MONO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

ATTACH=false
for arg in "$@"; do
  case "$arg" in
    --attach|-a) ATTACH=true ;;
  esac
done

# 瑼Ｘ tmux
if ! command -v tmux &>/dev/null; then
  echo "ERROR: tmux ?芸?鋆?
  exit 1
fi

# 憒? session 撌脣??剁??湔??
if tmux has-session -t "$SESSION" 2>/dev/null; then
  echo "Session '$SESSION' 撌脣???
  if [[ "$ATTACH" == "true" ]]; then
    tmux attach -t "$SESSION"
  else
    echo "?瑁?: tmux attach -t $SESSION"
  fi
  exit 0
fi

# ?萄遣??session
tmux new-session -d -s "$SESSION" -n 'Main' -c "$MONO_ROOT"

# 銝餉?蝒?撌血 Principal嚗??Stats
tmux split-window -h -t "$SESSION:Main" -c "$MONO_ROOT"

# 撌血 (Principal)
tmux select-pane -t "$SESSION:Main.0"
tmux send-keys -t "$SESSION:Main.0" "cd '$MONO_ROOT'" C-m
tmux send-keys -t "$SESSION:Main.0" "# Principal - ?瑁? /start-work ??bash scripts/ai/kickoff.sh" C-m
tmux send-keys -t "$SESSION:Main.0" "clear" C-m

# ?喳 (Stats/Monitor)
tmux select-pane -t "$SESSION:Main.1"
tmux send-keys -t "$SESSION:Main.1" "cd '$MONO_ROOT'" C-m
tmux send-keys -t "$SESSION:Main.1" "# Monitor - ?瑁? watch 'bash scripts/ai/stats.sh'" C-m
tmux send-keys -t "$SESSION:Main.1" "clear" C-m

# ?萄遣蝚砌???蝒?潭隤?tmux new-window -t "$SESSION" -n 'Logs' -c "$MONO_ROOT"
tmux send-keys -t "$SESSION:Logs" "cd '$MONO_ROOT'" C-m
tmux send-keys -t "$SESSION:Logs" "# Logs - ?瑁? tail -f .ai/exe-logs/*.log" C-m

# ?銝餉?蝒?tmux select-window -t "$SESSION:Main"
tmux select-pane -t "$SESSION:Main.0"

echo ""
echo "????????????????????????????????????????????????????????????????
echo "??             AI Dev Flow tmux Session                       ??
echo "????????????????????????????????????????????????????????????????
echo "?? Session: $SESSION                                          ??
echo "??                                                            ??
echo "?? 閬? 1 (Main):                                             ??
echo "??   撌血: Principal - ?瑁?撌乩?瘚?                            ??
echo "??   ?喳: Monitor - ?亦?蝯梯?                                 ??
echo "??                                                            ??
echo "?? 閬? 2 (Logs):                                             ??
echo "??   ?亦??瑁??亥?                                             ??
echo "??                                                            ??
echo "?? 敹急??                                                    ??
echo "??   Ctrl+b n  - 銝???蝒?                                  ??
echo "??   Ctrl+b p  - 銝???蝒?                                  ??
echo "??   Ctrl+b o  - ?? pane                                    ??
echo "??   Ctrl+b d  - ? session                                 ??
echo "????????????????????????????????????????????????????????????????
echo ""

if [[ "$ATTACH" == "true" ]]; then
  tmux attach -t "$SESSION"
else
  echo "?瑁?: tmux attach -t $SESSION"
fi
