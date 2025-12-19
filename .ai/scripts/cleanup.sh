#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# cleanup.sh - 皜?撌脣??? worktrees ????# ============================================================================
# ?冽?:
#   bash .ai/scripts/cleanup.sh [--dry-run] [--days N] [--force]
#
# ?賊?:
#   --dry-run   ?芷＊蝷箸?皜?隞暻潘?銝祕?銵?#   --days N    ?芣???N 憭拙?撌脣?雿??????身 7嚗?#   --force     撘瑕皜?嚗?瑼Ｘ PR ???# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

DRY_RUN=false
DAYS=7
FORCE=false

# 閫???
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
# 1. 皜? Worktrees
# ============================================================
echo "## Checking worktrees..."

# ?脣????worktrees
WORKTREES=$(git worktree list --porcelain 2>/dev/null | grep "^worktree" | sed 's/worktree //' || true)

for wt in $WORKTREES; do
  # 頝喲?銝?worktree
  if [[ "$wt" == "$MONO_ROOT" ]] || [[ "$wt" == "$(pwd)" ]]; then
    continue
  fi
  
  # 瑼Ｘ?臬??issue worktree
  if [[ "$wt" == *"issue-"* ]] || [[ "$wt" == *".worktrees"* ]]; then
    # ?? issue 蝺刻?
    ISSUE_NUM=$(echo "$wt" | grep -oP 'issue-\K\d+' || echo "")
    
    if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
      # 瑼Ｘ issue ???      ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
      
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

# 皜? worktree 閮?銝剔??⊥?璇
if [[ "$DRY_RUN" == "false" ]]; then
  git worktree prune 2>/dev/null || true
fi

# ============================================================
# 2. 皜??垢?
# ============================================================
echo ""
echo "## Checking remote branches..."

# ?脣?撌脣?雿萇??垢?
git fetch --prune 2>/dev/null || true

# ? feat/ai-issue- ???蝡臬???REMOTE_BRANCHES=$(git branch -r --list 'origin/feat/ai-issue-*' 2>/dev/null || true)

for branch in $REMOTE_BRANCHES; do
  # 蝘駁 origin/ ?韌
  BRANCH_NAME="${branch#origin/}"
  
  # ?? issue 蝺刻? (from feat/ai-issue-N)
  ISSUE_NUM=$(echo "$BRANCH_NAME" | grep -oP 'ai-issue-\K\d+' || echo "")
  
  if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
    # 瑼Ｘ撠???PR ???    PR_STATE=$(gh pr list --head "$BRANCH_NAME" --json state -q '.[0].state' 2>/dev/null || echo "")
    
    if [[ "$PR_STATE" == "OPEN" ]]; then
      echo "  SKIP: $BRANCH_NAME (PR is still open)"
      continue
    fi
    
    # 瑼Ｘ issue ???    ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
    
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
# 3. 皜??砍?
# ============================================================
echo ""
echo "## Checking local branches..."

LOCAL_BRANCHES=$(git branch --list 'feat/ai-issue-*' 2>/dev/null | sed 's/^[* ]*//' || true)

for branch in $LOCAL_BRANCHES; do
  # ?? issue 蝺刻? (from feat/ai-issue-N)
  ISSUE_NUM=$(echo "$branch" | grep -oP 'ai-issue-\K\d+' || echo "")
  
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
# 4. 皜??? run 閮?
# ============================================================
echo ""
echo "## Checking old run records..."

RUNS_DIR="$AI_ROOT/runs"
if [[ -d "$RUNS_DIR" ]]; then
  # ?曉頞? N 憭拍??桅?
  OLD_RUNS=$(find "$RUNS_DIR" -maxdepth 1 -type d -name "issue-*" -mtime +"$DAYS" 2>/dev/null || true)
  
  for run_dir in $OLD_RUNS; do
    # ?? issue 蝺刻?
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
# 5. 皜??? result ?辣
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
