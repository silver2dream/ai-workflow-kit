#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# cleanup.sh - 清理已完成的 worktrees 和分支
# ============================================================================
# 用法:
#   bash .ai/scripts/cleanup.sh [--dry-run] [--days N] [--force]
#
# 選項:
#   --dry-run   只顯示會清理什麼，不實際執行
#   --days N    只清理 N 天前已合併/關閉的（預設 7）
#   --force     強制清理，不檢查 PR 狀態
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

DRY_RUN=false
DAYS=7
FORCE=false

# 解析參數
while [[ $# -gt 0 ]]; do
  case $1 in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --days)
      DAYS="$2"
      shift 2
      ;;
    --force)
      FORCE=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

echo "============================================"
echo "AI Workflow Kit - Cleanup"
echo "============================================"
echo "Dry run: $DRY_RUN"
echo "Days threshold: $DAYS"
echo "Force: $FORCE"
echo ""

CLEANED_WORKTREES=0
CLEANED_BRANCHES=0
CLEANED_RUNS=0

# ============================================================
# 1. 清理 Worktrees
# ============================================================
echo "## Checking worktrees..."

# 獲取所有 worktrees
WORKTREES=$(git worktree list --porcelain 2>/dev/null | grep "^worktree" | sed 's/worktree //' || true)

for wt in $WORKTREES; do
  # 跳過主 worktree
  if [[ "$wt" == "$MONO_ROOT" ]] || [[ "$wt" == "$(pwd)" ]]; then
    continue
  fi
  
  # 檢查是否是 issue worktree
  if [[ "$wt" == *"issue-"* ]] || [[ "$wt" == *".worktrees"* ]]; then
    # 提取 issue 編號
    ISSUE_NUM=$(echo "$wt" | grep -oP 'issue-\K\d+' || echo "")
    
    if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
      # 檢查 issue 狀態
      ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
      
      if [[ "$ISSUE_STATE" == "OPEN" ]]; then
        echo "  SKIP: $wt (issue #$ISSUE_NUM is still open)"
        continue
      fi
    fi
    
    echo "  CLEAN: $wt"
    if [[ "$DRY_RUN" == "false" ]]; then
      git worktree remove "$wt" --force 2>/dev/null || echo "    WARN: Could not remove worktree"
      CLEANED_WORKTREES=$((CLEANED_WORKTREES + 1))
    fi
  fi
done

# 清理 worktree 記錄中的無效條目
if [[ "$DRY_RUN" == "false" ]]; then
  git worktree prune 2>/dev/null || true
fi

# ============================================================
# 2. 清理遠端分支
# ============================================================
echo ""
echo "## Checking remote branches..."

# 獲取已合併的遠端分支
git fetch --prune 2>/dev/null || true

# 列出 issue- 開頭的遠端分支
REMOTE_BRANCHES=$(git branch -r --list 'origin/issue-*' 2>/dev/null || true)

for branch in $REMOTE_BRANCHES; do
  # 移除 origin/ 前綴
  BRANCH_NAME="${branch#origin/}"
  
  # 提取 issue 編號
  ISSUE_NUM=$(echo "$BRANCH_NAME" | grep -oP 'issue-\K\d+' || echo "")
  
  if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
    # 檢查對應的 PR 狀態
    PR_STATE=$(gh pr list --head "$BRANCH_NAME" --json state -q '.[0].state' 2>/dev/null || echo "")
    
    if [[ "$PR_STATE" == "OPEN" ]]; then
      echo "  SKIP: $BRANCH_NAME (PR is still open)"
      continue
    fi
    
    # 檢查 issue 狀態
    ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
    
    if [[ "$ISSUE_STATE" == "OPEN" ]]; then
      echo "  SKIP: $BRANCH_NAME (issue #$ISSUE_NUM is still open)"
      continue
    fi
  fi
  
  echo "  CLEAN: $BRANCH_NAME"
  if [[ "$DRY_RUN" == "false" ]]; then
    git push origin --delete "$BRANCH_NAME" 2>/dev/null || echo "    WARN: Could not delete remote branch"
    CLEANED_BRANCHES=$((CLEANED_BRANCHES + 1))
  fi
done

# ============================================================
# 3. 清理本地分支
# ============================================================
echo ""
echo "## Checking local branches..."

LOCAL_BRANCHES=$(git branch --list 'issue-*' 2>/dev/null | sed 's/^[* ]*//' || true)

for branch in $LOCAL_BRANCHES; do
  # 提取 issue 編號
  ISSUE_NUM=$(echo "$branch" | grep -oP 'issue-\K\d+' || echo "")
  
  if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
    ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
    
    if [[ "$ISSUE_STATE" == "OPEN" ]]; then
      echo "  SKIP: $branch (issue #$ISSUE_NUM is still open)"
      continue
    fi
  fi
  
  echo "  CLEAN: $branch"
  if [[ "$DRY_RUN" == "false" ]]; then
    git branch -D "$branch" 2>/dev/null || echo "    WARN: Could not delete local branch"
    CLEANED_BRANCHES=$((CLEANED_BRANCHES + 1))
  fi
done

# ============================================================
# 4. 清理舊的 run 記錄
# ============================================================
echo ""
echo "## Checking old run records..."

RUNS_DIR="$AI_ROOT/runs"
if [[ -d "$RUNS_DIR" ]]; then
  # 找出超過 N 天的目錄
  OLD_RUNS=$(find "$RUNS_DIR" -maxdepth 1 -type d -name "issue-*" -mtime +"$DAYS" 2>/dev/null || true)
  
  for run_dir in $OLD_RUNS; do
    # 提取 issue 編號
    ISSUE_NUM=$(basename "$run_dir" | grep -oP 'issue-\K\d+' || echo "")
    
    if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
      ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
      
      if [[ "$ISSUE_STATE" == "OPEN" ]]; then
        echo "  SKIP: $run_dir (issue #$ISSUE_NUM is still open)"
        continue
      fi
    fi
    
    echo "  CLEAN: $run_dir"
    if [[ "$DRY_RUN" == "false" ]]; then
      rm -rf "$run_dir"
      CLEANED_RUNS=$((CLEANED_RUNS + 1))
    fi
  done
fi

# ============================================================
# 5. 清理舊的 result 文件
# ============================================================
echo ""
echo "## Checking old result files..."

RESULTS_DIR="$AI_ROOT/results"
if [[ -d "$RESULTS_DIR" ]]; then
  OLD_RESULTS=$(find "$RESULTS_DIR" -maxdepth 1 -type f -name "issue-*.json" -mtime +"$DAYS" 2>/dev/null || true)
  
  for result_file in $OLD_RESULTS; do
    echo "  CLEAN: $result_file"
    if [[ "$DRY_RUN" == "false" ]]; then
      rm -f "$result_file"
    fi
  done
fi

# ============================================================
# Summary
# ============================================================
echo ""
echo "============================================"
if [[ "$DRY_RUN" == "true" ]]; then
  echo "DRY RUN - No changes made"
else
  echo "Cleanup complete!"
  echo "  Worktrees removed: $CLEANED_WORKTREES"
  echo "  Branches deleted: $CLEANED_BRANCHES"
  echo "  Run records cleaned: $CLEANED_RUNS"
fi
echo "============================================"
