#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# rollback.sh - Create a revert PR for a merged PR.
# ============================================================================
# Usage:
#   bash .ai/scripts/rollback.sh <PR_NUMBER> [--dry-run]
#
# Steps:
#   1. Fetch PR info
#   2. Revert merge commit
#   3. Create revert PR
#   4. Reopen original issue (if any)
#   5. Send notification
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"

PR_NUMBER="${1:?usage: rollback.sh <PR_NUMBER> [--dry-run]}"
DRY_RUN="${2:-}"

echo "[rollback] Starting rollback for PR #$PR_NUMBER"

# Require gh CLI.
if ! command -v gh &> /dev/null; then
  echo "[rollback] ERROR: gh CLI not found"
  exit 1
fi

# Require auth.
if ! gh auth status &> /dev/null; then
  echo "[rollback] ERROR: gh not authenticated. Run 'gh auth login'"
  exit 1
fi

# Fetch PR info.
echo "[rollback] Fetching PR info..."
PR_INFO=$(gh pr view "$PR_NUMBER" --json title,body,mergeCommit,headRefName,state,mergedAt 2>/dev/null)

if [[ -z "$PR_INFO" ]]; then
  echo "[rollback] ERROR: PR #$PR_NUMBER not found"
  exit 1
fi

# Parse PR fields.
PR_TITLE=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('title',''))")
PR_BODY=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('body',''))")
MERGE_COMMIT=$(echo "$PR_INFO" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('mergeCommit',{}).get('oid','') if d.get('mergeCommit') else '')")
PR_STATE=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('state',''))")
MERGED_AT=$(echo "$PR_INFO" | python3 -c "import sys,json; print(json.load(sys.stdin).get('mergedAt',''))")

echo "[rollback] PR Title: $PR_TITLE"
echo "[rollback] PR State: $PR_STATE"
echo "[rollback] Merge Commit: $MERGE_COMMIT"

# Ensure PR is merged.
if [[ "$PR_STATE" != "MERGED" ]]; then
  echo "[rollback] ERROR: PR #$PR_NUMBER is not merged (state: $PR_STATE)"
  exit 1
fi

if [[ -z "$MERGE_COMMIT" ]]; then
  echo "[rollback] ERROR: Cannot find merge commit for PR #$PR_NUMBER"
  exit 1
fi

# Extract issue number from PR body.
ISSUE_NUMBER=$(echo "$PR_BODY" | grep -iE '(closes|fixes|resolves)\s*#[0-9]+' | sed -n 's/.*#\([0-9]*\).*/\1/p' | head -1)
echo "[rollback] Original Issue: ${ISSUE_NUMBER:-none}"

# Dry run mode.
if [[ "$DRY_RUN" == "--dry-run" ]]; then
  echo ""
  echo "[rollback] DRY RUN - Would execute:"
  echo "  1. git revert $MERGE_COMMIT --no-edit"
  echo "  2. git push origin HEAD"
  echo "  3. gh pr create --title 'Revert: $PR_TITLE'"
  if [[ -n "$ISSUE_NUMBER" ]]; then
    echo "  4. gh issue reopen $ISSUE_NUMBER"
  fi
  echo "  5. Send notification"
  exit 0
fi

# Ensure working tree is clean.
if [[ -n "$(git status --porcelain)" ]]; then
  echo "[rollback] ERROR: Working directory not clean"
  exit 1
fi

# Capture current branch.
CURRENT_BRANCH=$(git branch --show-current)
echo "[rollback] Current branch: $CURRENT_BRANCH"

# Create revert branch.
REVERT_BRANCH="revert-pr-$PR_NUMBER"
echo "[rollback] Creating revert branch: $REVERT_BRANCH"

git fetch origin "$CURRENT_BRANCH"
git checkout -b "$REVERT_BRANCH" "origin/$CURRENT_BRANCH"

# Revert merge commit.
echo "[rollback] Reverting commit $MERGE_COMMIT..."

# Check if this is a submodule-related commit by looking at the diff
SUBMODULE_CHANGES=""
if git show "$MERGE_COMMIT" --name-only | grep -q "^Subproject commit"; then
  SUBMODULE_CHANGES="true"
  echo "[rollback] Detected submodule changes in merge commit"
fi

# For submodule commits, we need to handle the revert carefully (Req 18.1-18.4)
if [[ "$SUBMODULE_CHANGES" == "true" ]]; then
  echo "[rollback] Handling submodule revert..."
  
  # Get list of changed submodules
  CHANGED_SUBMODULES=$(git diff-tree --no-commit-id --name-only -r "$MERGE_COMMIT" | while read -r path; do
    if git ls-tree "$MERGE_COMMIT" "$path" 2>/dev/null | grep -q "^160000"; then
      echo "$path"
    fi
  done)
  
  # Revert in submodules first (Req 18.1)
  for submodule_path in $CHANGED_SUBMODULES; do
    if [[ -d "$submodule_path" && -e "$submodule_path/.git" ]]; then
      echo "[rollback] Reverting in submodule: $submodule_path"
      
      # Get the submodule commit before and after the merge
      OLD_SUBMODULE_SHA=$(git rev-parse "$MERGE_COMMIT^:$submodule_path" 2>/dev/null || true)
      NEW_SUBMODULE_SHA=$(git rev-parse "$MERGE_COMMIT:$submodule_path" 2>/dev/null || true)
      
      if [[ -n "$OLD_SUBMODULE_SHA" && -n "$NEW_SUBMODULE_SHA" && "$OLD_SUBMODULE_SHA" != "$NEW_SUBMODULE_SHA" ]]; then
        echo "[rollback] Submodule $submodule_path: $NEW_SUBMODULE_SHA -> $OLD_SUBMODULE_SHA"
        
        # Try to revert in submodule (Req 18.2)
        if ! git -C "$submodule_path" revert "$NEW_SUBMODULE_SHA" --no-edit 2>/dev/null; then
          echo "[rollback] WARNING: Could not revert submodule commit. Will reset to old SHA."
          git -C "$submodule_path" checkout "$OLD_SUBMODULE_SHA" 2>/dev/null || true
        fi
      fi
    fi
  done
fi

# Revert the parent commit (Req 18.3)
if ! git revert "$MERGE_COMMIT" --no-edit; then
  echo "[rollback] ERROR: Revert failed. Manual intervention required." >&2
  
  # Handle revert failure (Req 18.4)
  if [[ "$SUBMODULE_CHANGES" == "true" ]]; then
    echo "[rollback] HINT: Submodule revert may have partially succeeded." >&2
    echo "[rollback] Check submodule states and consider manual cleanup." >&2
  fi
  
  git revert --abort 2>/dev/null || true
  git checkout "$CURRENT_BRANCH"
  git branch -D "$REVERT_BRANCH" 2>/dev/null || true
  exit 1
fi

# Push revert branch.
echo "[rollback] Pushing revert branch..."
git push origin "$REVERT_BRANCH"

# Create revert PR.
echo "[rollback] Creating revert PR..."
REVERT_PR_URL=$(gh pr create \
  --title "Revert: $PR_TITLE" \
  --body "This reverts PR #$PR_NUMBER (commit $MERGE_COMMIT).

**Reason:** [Please add reason for rollback]

**Original PR:** #$PR_NUMBER
**Original Issue:** ${ISSUE_NUMBER:-N/A}
**Reverted at:** $(date -u +"%Y-%m-%dT%H:%M:%SZ")

---
_This revert was created automatically by rollback.sh_" \
  --head "$REVERT_BRANCH" \
  2>&1)

echo "[rollback] Revert PR created: $REVERT_PR_URL"

# Reopen original issue if present.
if [[ -n "$ISSUE_NUMBER" ]]; then
  echo "[rollback] Reopening issue #$ISSUE_NUMBER..."
  gh issue reopen "$ISSUE_NUMBER" --comment "Reopened due to rollback of PR #$PR_NUMBER.

Revert PR: $REVERT_PR_URL" 2>/dev/null || echo "[rollback] WARN: Could not reopen issue #$ISSUE_NUMBER"
fi

# Send notification if available.
if [[ -f "$SCRIPT_DIR/notify.sh" ]]; then
  echo "[rollback] Sending notification..."
  bash "$SCRIPT_DIR/notify.sh" "Rollback: PR #$PR_NUMBER has been reverted. Revert PR: $REVERT_PR_URL" 2>/dev/null || true
fi

# Return to the original branch.
git checkout "$CURRENT_BRANCH"

echo ""
echo "[rollback] Rollback complete!"
echo "  Revert PR: $REVERT_PR_URL"
echo "  Original Issue: ${ISSUE_NUMBER:-N/A}"
echo ""
echo "Next steps:"
echo "  1. Review and merge the revert PR"
echo "  2. Investigate the issue"
echo "  3. Create a fix PR"
