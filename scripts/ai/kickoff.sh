#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# kickoff.sh - 一鍵啟動 AI 自動化工作流
# ============================================================================
# 用法:
#   bash scripts/ai/kickoff.sh              # 啟動完整工作流
#   bash scripts/ai/kickoff.sh --dry-run    # 只做前置檢查，不啟動
#   bash scripts/ai/kickoff.sh --background # 背景執行 (nohup)
# ============================================================================

MONO_ROOT="$(git rev-parse --show-toplevel)"
cd "$MONO_ROOT"

DRY_RUN=false
BACKGROUND=false

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --background) BACKGROUND=true ;;
    --help|-h)
      echo "用法: bash scripts/ai/kickoff.sh [--dry-run] [--background]"
      echo ""
      echo "選項:"
      echo "  --dry-run     只做前置檢查，不啟動工作流"
      echo "  --background  背景執行 (使用 nohup)"
      exit 0
      ;;
  esac
done

# ----------------------------------------------------------------------------
# 顏色輸出
# ----------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
ok()    { echo -e "${GREEN}[OK]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ----------------------------------------------------------------------------
# 前置檢查
# ----------------------------------------------------------------------------
info "執行前置檢查..."

# 1. 檢查 gh CLI
if ! command -v gh &>/dev/null; then
  error "gh CLI 未安裝。請執行: brew install gh (macOS) 或參考 https://cli.github.com/"
  exit 1
fi
ok "gh CLI 已安裝"

# 2. 檢查 gh 認證
if ! gh auth status &>/dev/null; then
  error "gh 未認證。請執行: gh auth login"
  exit 1
fi
ok "gh 已認證"

# 3. 檢查 claude CLI
if ! command -v claude &>/dev/null; then
  error "claude CLI 未安裝。請確認 Claude Code Pro 已安裝並在 PATH 中。"
  exit 1
fi
ok "claude CLI 已安裝"

# 4. 檢查 codex CLI
if ! command -v codex &>/dev/null; then
  warn "codex CLI 未安裝。Worker 執行時會失敗。"
else
  ok "codex CLI 已安裝"
fi

# 5. 檢查工作目錄乾淨
if [[ -n "$(git status --porcelain)" ]]; then
  error "工作目錄不乾淨。請先 commit 或 stash 變更。"
  git status --short
  exit 1
fi
ok "工作目錄乾淨"

# 6. 檢查停止標記
if [[ -f ".ai/state/STOP" ]]; then
  warn "發現停止標記 .ai/state/STOP"
  read -p "是否刪除並繼續? [y/N] " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -f ".ai/state/STOP"
    ok "已刪除停止標記"
  else
    error "請手動刪除 .ai/state/STOP 後重試"
    exit 1
  fi
fi
ok "無停止標記"

# 7. 執行專案審計
info "執行專案審計..."
bash "$MONO_ROOT/scripts/ai/scan_repo.sh" >/dev/null
bash "$MONO_ROOT/scripts/ai/audit_project.sh" >/dev/null

AUDIT_FILE="$MONO_ROOT/.ai/state/audit.json"
if [[ -f "$AUDIT_FILE" ]]; then
  P0_COUNT=$(python3 -c "import json; print(json.load(open('$AUDIT_FILE'))['summary']['p0'])" 2>/dev/null || echo "0")
  P1_COUNT=$(python3 -c "import json; print(json.load(open('$AUDIT_FILE'))['summary']['p1'])" 2>/dev/null || echo "0")
  
  if [[ "$P0_COUNT" -gt 0 ]]; then
    error "發現 $P0_COUNT 個 P0 問題，必須先修復！"
    python3 -c "
import json
audit = json.load(open('$AUDIT_FILE'))
for f in audit['findings']:
    if f['severity'] == 'P0':
        print(f\"  - [{f['severity']}] {f['title']}\")
"
    exit 1
  fi
  
  if [[ "$P1_COUNT" -gt 0 ]]; then
    warn "發現 $P1_COUNT 個 P1 問題，建議修復"
  fi
fi
ok "專案審計完成"

# ----------------------------------------------------------------------------
# Dry Run 模式
# ----------------------------------------------------------------------------
if [[ "$DRY_RUN" == "true" ]]; then
  echo ""
  info "=== Dry Run 完成 ==="
  info "所有前置檢查通過，可以啟動工作流。"
  info "執行 'bash scripts/ai/kickoff.sh' 啟動。"
  exit 0
fi

# ----------------------------------------------------------------------------
# 準備啟動
# ----------------------------------------------------------------------------
mkdir -p "$MONO_ROOT/.ai/state" "$MONO_ROOT/.ai/results" "$MONO_ROOT/.ai/runs" "$MONO_ROOT/.ai/exe-logs"

TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "$TIMESTAMP" > "$MONO_ROOT/.ai/state/kickoff_time.txt"

info "準備啟動 AI 工作流..."
echo ""
echo "┌─────────────────────────────────────────────────────────────┐"
echo "│                    AI 自動化工作流                          │"
echo "├─────────────────────────────────────────────────────────────┤"
echo "│  啟動時間: $TIMESTAMP                      │"
echo "│  工作目錄: $MONO_ROOT"
echo "│                                                             │"
echo "│  停止方式:                                                  │"
echo "│    1. touch .ai/state/STOP                                  │"
echo "│    2. 在 Claude Code 中說「停止」                           │"
echo "│    3. Ctrl+C                                                │"
echo "│                                                             │"
echo "│  查看進度:                                                  │"
echo "│    gh issue list --label ai-task                            │"
echo "│    gh pr list                                               │"
echo "└─────────────────────────────────────────────────────────────┘"
echo ""

# ----------------------------------------------------------------------------
# 啟動 Claude Code
# ----------------------------------------------------------------------------

BOOT_PROMPT="$MONO_ROOT/scripts/ai/principal_boot.txt"

if [[ "$BACKGROUND" == "true" ]]; then
  info "背景模式啟動..."
  LOG_FILE="$MONO_ROOT/.ai/exe-logs/kickoff-$(date +%Y%m%d-%H%M%S).log"
  
  nohup bash -c "
    cd '$MONO_ROOT'
    if [[ -f '$BOOT_PROMPT' ]]; then
      claude --print < '$BOOT_PROMPT' >> '$LOG_FILE' 2>&1
    else
      echo '/start-work' | claude --print >> '$LOG_FILE' 2>&1
    fi
  " > /dev/null 2>&1 &
  
  CLAUDE_PID=$!
  echo "$CLAUDE_PID" > "$MONO_ROOT/.ai/state/claude_pid.txt"
  
  ok "Claude Code 已在背景啟動 (PID: $CLAUDE_PID)"
  info "日誌文件: $LOG_FILE"
  info "停止: kill $CLAUDE_PID 或 touch .ai/state/STOP"
else
  info "前台模式啟動..."
  info "Claude Code 將接管終端，執行 /start-work 指令"
  echo ""
  
  # 使用 principal_boot.txt 如果存在，否則直接執行 /start-work
  if [[ -f "$BOOT_PROMPT" ]]; then
    info "使用 principal_boot.txt 作為啟動 prompt"
    claude < "$BOOT_PROMPT"
  else
    # 直接啟動 claude 並讓用戶輸入 /start-work
    info "請在 Claude Code 中輸入: /start-work"
    claude
  fi
fi
