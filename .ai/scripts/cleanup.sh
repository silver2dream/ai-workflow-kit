#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# cleanup.sh - Clean up worktrees, branches, runs, and results.
# ============================================================================
# Usage:
#   bash .ai/scripts/cleanup.sh [--dry-run] [--days N] [--force]
#
# Options:
#   --dry-run   Show what would be removed
#   --days N    Age threshold in days (default: 7)
#   --force     Skip issue/PR open checks
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

DRY_RUN=false
DAYS=7
FORCE=false

# Parse arguments.
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
# 1. Worktrees
# ============================================================
echo "## Checking worktrees..."

# List worktrees.
WORKTREES=$(git worktree list --porcelain 2>/dev/null | grep "^worktree" | sed 's/worktree //' || true)

for wt in $WORKTREES; do
  # Skip main worktree.
  if [[ "$wt" == "$MONO_ROOT" ]] || [[ "$wt" == "$(pwd)" ]]; then
    continue
  fi
  
  # Only clean issue worktrees.
  if [[ "$wt" == *"issue-"* ]] || [[ "$wt" == *".worktrees"* ]]; then
    # Extract issue number.
    ISSUE_NUM=$(echo "$wt" | grep -oP 'issue-\K\d+' || echo "")
    
    if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
      # Check issue state.
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

# Prune stale worktree entries.
if [[ "$DRY_RUN" == "false" ]]; then
  git worktree prune 2>/dev/null || true
fi

# ============================================================
# 2. Remote branches (including submodule branches)
# ============================================================
echo ""
echo "## Checking remote branches..."

# Prune remote branches.
git fetch --prune 2>/dev/null || true

# Find remote branches that match feat/ai-issue-*.
REMOTE_BRANCHES=$(git branch -r --list 'origin/feat/ai-issue-*' 2>/dev/null || true)

for branch in $REMOTE_BRANCHES; do
  # Trim origin/ prefix.
  BRANCH_NAME="${branch#origin/}"
  
  # Extract issue number (from feat/ai-issue-N).
  ISSUE_NUM=$(echo "$BRANCH_NAME" | grep -oP 'ai-issue-\K\d+' || echo "")
  
  if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
    # Check PR state.
    PR_STATE=$(gh pr list --head "$BRANCH_NAME" --json state -q '.[0].state' 2>/dev/null || echo "")
    
    if [[ "$PR_STATE" == "OPEN" ]]; then
      echo "  SKIP: $BRANCH_NAME (PR is still open)"
      continue
    fi
    
    # Check issue state.
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
# 2.1 Submodule branches cleanup (Req 12.1-12.3, 16.3)
# ============================================================
echo ""
echo "## Checking submodule branches..."

# Get list of submodules
GITMODULES_PATH="$MONO_ROOT/.gitmodules"
if [[ -f "$GITMODULES_PATH" ]]; then
  SUBMODULE_PATHS=$(git config -f "$GITMODULES_PATH" --get-regexp path 2>/dev/null | awk '{print $2}' || true)
  
  for submodule_path in $SUBMODULE_PATHS; do
    SUBMODULE_DIR="$MONO_ROOT/$submodule_path"
    
    if [[ -d "$SUBMODULE_DIR" && -e "$SUBMODULE_DIR/.git" ]]; then
      echo "  Checking submodule: $submodule_path"
      
      # Fetch and prune in submodule
      git -C "$SUBMODULE_DIR" fetch --prune 2>/dev/null || true
      
      # Find remote branches in submodule that match feat/ai-issue-*
      SUBMODULE_REMOTE_BRANCHES=$(git -C "$SUBMODULE_DIR" branch -r --list 'origin/feat/ai-issue-*' 2>/dev/null || true)
      
      for branch in $SUBMODULE_REMOTE_BRANCHES; do
        BRANCH_NAME="${branch#origin/}"
        ISSUE_NUM=$(echo "$BRANCH_NAME" | grep -oP 'ai-issue-\K\d+' || echo "")
        
        if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
          ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
          
          if [[ "$ISSUE_STATE" == "OPEN" ]]; then
            echo "    SKIP: $submodule_path:$BRANCH_NAME (issue #$ISSUE_NUM is still open)"
            continue
          fi
        fi
        
        echo "    CLEAN: $submodule_path:$BRANCH_NAME"
        if [[ "$DRY_RUN" == "false" ]]; then
          git -C "$SUBMODULE_DIR" push origin --delete "$BRANCH_NAME" 2>/dev/null || echo "      WARN: Could not delete submodule remote branch"
          CLEANED_BRANCHES=$((CLEANED_BRANCHES + 1))
        fi
      done
      
      # Clean local branches in submodule
      SUBMODULE_LOCAL_BRANCHES=$(git -C "$SUBMODULE_DIR" branch --list 'feat/ai-issue-*' 2>/dev/null | sed 's/^[* ]*//' || true)
      
      for branch in $SUBMODULE_LOCAL_BRANCHES; do
        ISSUE_NUM=$(echo "$branch" | grep -oP 'ai-issue-\K\d+' || echo "")
        
        if [[ -n "$ISSUE_NUM" ]] && [[ "$FORCE" != "true" ]]; then
          ISSUE_STATE=$(gh issue view "$ISSUE_NUM" --json state -q .state 2>/dev/null || echo "UNKNOWN")
          
          if [[ "$ISSUE_STATE" == "OPEN" ]]; then
            echo "    SKIP: $submodule_path:$branch (issue #$ISSUE_NUM is still open)"
            continue
          fi
        fi
        
        echo "    CLEAN: $submodule_path:$branch (local)"
        if [[ "$DRY_RUN" == "false" ]]; then
          git -C "$SUBMODULE_DIR" branch -D "$branch" 2>/dev/null || echo "      WARN: Could not delete submodule local branch"
          CLEANED_BRANCHES=$((CLEANED_BRANCHES + 1))
        fi
      done
    fi
  done
fi

# ============================================================
# 3. Local branches
# ============================================================
echo ""
echo "## Checking local branches..."

LOCAL_BRANCHES=$(git branch --list 'feat/ai-issue-*' 2>/dev/null | sed 's/^[* ]*//' || true)

for branch in $LOCAL_BRANCHES; do
  # Extract issue number (from feat/ai-issue-N).
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
# 4. Run records
# ============================================================
echo ""
echo "## Checking old run records..."

RUNS_DIR="$AI_ROOT/runs"
if [[ -d "$RUNS_DIR" ]]; then
  # Find runs older than N days.
  OLD_RUNS=$(find "$RUNS_DIR" -maxdepth 1 -type d -name "issue-*" -mtime +"$DAYS" 2>/dev/null || true)
  
  for run_dir in $OLD_RUNS; do
    # Extract issue number.
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
# 5. Result files
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
