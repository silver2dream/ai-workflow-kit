#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# init.sh - 初始化新專案的 AI Workflow
# ============================================================================
# 用法:
#   bash .ai/scripts/init.sh
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

echo "┌─────────────────────────────────────────────────────────────┐"
echo "│              AI Workflow Kit - 初始化                       │"
echo "└─────────────────────────────────────────────────────────────┘"
echo ""

# 檢查配置文件
CONFIG_FILE="$AI_ROOT/config/workflow.yaml"
if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "[init] 創建預設配置文件..."
  mkdir -p "$AI_ROOT/config"
  cat > "$CONFIG_FILE" <<'YAML'
# AI Workflow Kit Configuration
version: "1.0"

project:
  name: "my-project"
  description: "My Project"
  type: "single-repo"  # monorepo | single-repo

repos:
  - name: root
    path: .
    type: root
    language: node  # go | node | python | unity | etc.
    rules:
      - git-workflow
    verify:
      build: "npm run build"
      test: "npm test"

git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree

notifications:
  system_notify: true
YAML
  echo "[init] 配置文件已創建: $CONFIG_FILE"
  echo "[init] 請編輯配置文件後重新執行此腳本"
  exit 0
fi

# 創建必要目錄
echo "[init] 創建目錄結構..."
mkdir -p "$AI_ROOT/state"
mkdir -p "$AI_ROOT/results"
mkdir -p "$AI_ROOT/runs"
mkdir -p "$AI_ROOT/exe-logs"
mkdir -p "$AI_ROOT/specs"

# 創建 .claude 符號連結（如果不存在）
CLAUDE_DIR="$MONO_ROOT/.claude"
if [[ ! -d "$CLAUDE_DIR" ]]; then
  echo "[init] 創建 .claude 目錄..."
  mkdir -p "$CLAUDE_DIR"
fi

# 注意：Windows 上符號連結需要管理員權限，這裡用複製代替
if [[ ! -d "$CLAUDE_DIR/commands" ]]; then
  echo "[init] 複製 commands 到 .claude/..."
  cp -r "$AI_ROOT/commands" "$CLAUDE_DIR/"
fi

if [[ ! -d "$CLAUDE_DIR/rules" ]]; then
  echo "[init] 複製 rules 到 .claude/..."
  cp -r "$AI_ROOT/rules" "$CLAUDE_DIR/"
fi

# 生成 CLAUDE.md 和 AGENTS.md
echo "[init] 生成 Agent 指南..."
bash "$AI_ROOT/scripts/generate.sh"

# 創建 .gitignore 條目
GITIGNORE="$MONO_ROOT/.gitignore"
if [[ -f "$GITIGNORE" ]]; then
  if ! grep -q ".ai/state/" "$GITIGNORE" 2>/dev/null; then
    echo "[init] 更新 .gitignore..."
    echo "" >> "$GITIGNORE"
    echo "# AI Workflow" >> "$GITIGNORE"
    echo ".ai/state/" >> "$GITIGNORE"
    echo ".ai/results/" >> "$GITIGNORE"
    echo ".ai/runs/" >> "$GITIGNORE"
    echo ".ai/exe-logs/" >> "$GITIGNORE"
    echo ".worktrees/" >> "$GITIGNORE"
  fi
fi

echo ""
echo "┌─────────────────────────────────────────────────────────────┐"
echo "│              初始化完成！                                   │"
echo "├─────────────────────────────────────────────────────────────┤"
echo "│                                                             │"
echo "│  下一步:                                                    │"
echo "│  1. 編輯 .ai/config/workflow.yaml 配置專案                  │"
echo "│  2. 執行 bash .ai/scripts/generate.sh 重新生成指南          │"
echo "│  3. 執行 bash .ai/scripts/kickoff.sh 啟動工作流             │"
echo "│                                                             │"
echo "└─────────────────────────────────────────────────────────────┘"
