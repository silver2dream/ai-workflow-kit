#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# audit_merged_prs.sh - 事後審計工具
# 檢查已合併的 PR 是否符合 AWK Review 規範
# Requirements: 7.1, 7.2, 7.3, 7.4
#
# Usage: audit_merged_prs.sh [--since <date>] [--limit <n>]
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY_REVIEW="$SCRIPT_DIR/verify_review.sh"
SESSION_LOG_DIR=".ai/state/principal/sessions"

# Cross-platform jq wrapper
_jq() {
  if command -v jq &>/dev/null; then
    jq "$@"
  elif command -v jq.exe &>/dev/null; then
    jq.exe "$@"
  elif [[ -x "/mnt/c/Users/user/bin/jq.exe" ]]; then
    /mnt/c/Users/user/bin/jq.exe "$@"
  else
    echo "[ERROR] jq not found" >&2
    return 1
  fi
}

# Parse arguments
SINCE=""
LIMIT=50
while [[ $# -gt 0 ]]; do
  case "$1" in
    --since)
      SINCE="$2"
      shift 2
      ;;
    --limit)
      LIMIT="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [--since <date>] [--limit <n>]"
      echo ""
      echo "Options:"
      echo "  --since <date>  Only audit PRs merged after this date (YYYY-MM-DD)"
      echo "  --limit <n>     Maximum number of PRs to audit (default: 50)"
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

echo "============================================"
echo "AWK Post-Merge Audit"
echo "============================================"
echo ""

# Build gh command
GH_CMD="gh pr list --state merged --limit $LIMIT --json number,title,mergedAt,body,comments"
if [[ -n "$SINCE" ]]; then
  echo "Auditing PRs merged since: $SINCE"
else
  echo "Auditing last $LIMIT merged PRs"
fi
echo ""

# Get merged PRs
MERGED_PRS=$($GH_CMD 2>/dev/null || echo "[]")
PR_COUNT=$(echo "$MERGED_PRS" | _jq 'length')

if [[ "$PR_COUNT" -eq 0 ]]; then
  echo "No merged PRs found."
  exit 0
fi

echo "Found $PR_COUNT merged PRs to audit"
echo ""

# Audit results
TOTAL=0
PASSED=0
SUSPICIOUS=0
MISSING_REVIEW=0

SUSPICIOUS_PRS=()

# Process each PR
echo "$MERGED_PRS" | _jq -c '.[]' | while read -r pr; do
  PR_NUMBER=$(echo "$pr" | _jq -r '.number')
  PR_TITLE=$(echo "$pr" | _jq -r '.title')
  MERGED_AT=$(echo "$pr" | _jq -r '.mergedAt')
  
  # Filter by date if specified
  if [[ -n "$SINCE" ]]; then
    MERGE_DATE=$(echo "$MERGED_AT" | cut -d'T' -f1)
    if [[ "$MERGE_DATE" < "$SINCE" ]]; then
      continue
    fi
  fi
  
  TOTAL=$((TOTAL + 1))
  echo "Auditing PR #$PR_NUMBER: $PR_TITLE"
  
  # Get PR comments to find AWK Review
  COMMENTS=$(echo "$pr" | _jq -r '.comments[].body // empty' 2>/dev/null || echo "")
  PR_BODY=$(echo "$pr" | _jq -r '.body // ""')
  
  # Check for AWK Review marker in comments or body
  AWK_REVIEW=""
  if echo "$COMMENTS" | grep -q "<!-- AWK Review -->"; then
    AWK_REVIEW=$(echo "$COMMENTS" | grep -A 100 "<!-- AWK Review -->" | head -100)
  elif echo "$PR_BODY" | grep -q "<!-- AWK Review -->"; then
    AWK_REVIEW="$PR_BODY"
  fi
  
  if [[ -z "$AWK_REVIEW" ]]; then
    echo "  ⚠ Missing AWK Review comment"
    MISSING_REVIEW=$((MISSING_REVIEW + 1))
    SUSPICIOUS_PRS+=("PR #$PR_NUMBER: Missing AWK Review")
    continue
  fi
  
  # Extract Session ID
  SESSION_ID=$(echo "$AWK_REVIEW" | sed -n 's/.*Session: \([a-z]*-[0-9]*-[0-9]*-[a-f0-9]*\).*/\1/p' | head -1)
  if [[ -z "$SESSION_ID" ]]; then
    echo "  ⚠ Missing or invalid Session ID"
    SUSPICIOUS=$((SUSPICIOUS + 1))
    SUSPICIOUS_PRS+=("PR #$PR_NUMBER: Invalid Session ID")
    continue
  fi
  
  # Local verification: check if session exists (Req 7.4)
  if [[ -d "$SESSION_LOG_DIR" ]] && [[ -f "$SESSION_LOG_DIR/${SESSION_ID}.json" ]]; then
    echo "  ✓ Session ID verified locally: $SESSION_ID"
  else
    echo "  ⚠ Session ID not found in local logs: $SESSION_ID"
    # This is a warning, not necessarily suspicious (could be from different machine)
  fi
  
  # Extract Diff Hash
  DIFF_HASH=$(echo "$AWK_REVIEW" | sed -n 's/.*Diff Hash: \([a-f0-9]*\).*/\1/p' | head -1)
  if [[ -z "$DIFF_HASH" ]]; then
    echo "  ⚠ Missing Diff Hash"
    SUSPICIOUS=$((SUSPICIOUS + 1))
    SUSPICIOUS_PRS+=("PR #$PR_NUMBER: Missing Diff Hash")
    continue
  fi
  echo "  ✓ Diff Hash: $DIFF_HASH"
  
  # Check for code symbols
  if echo "$AWK_REVIEW" | grep -qE '(程式碼符號|Code Symbols|Symbols):'; then
    echo "  ✓ Code symbols present"
  else
    echo "  ⚠ Missing code symbols"
  fi
  
  # Extract and check score
  SCORE=$(echo "$AWK_REVIEW" | sed -n 's/.*[評分Score][:\s]*\([0-9]*\).*/\1/p' | head -1)
  if [[ -z "$SCORE" ]]; then
    SCORE=$(echo "$AWK_REVIEW" | sed -n 's/.*\([0-9][0-9]*\)\/10.*/\1/p' | head -1)
  fi
  
  if [[ -z "$SCORE" ]]; then
    echo "  ⚠ Missing score"
    SUSPICIOUS=$((SUSPICIOUS + 1))
    SUSPICIOUS_PRS+=("PR #$PR_NUMBER: Missing score")
    continue
  fi
  
  if [[ "$SCORE" -lt 7 ]]; then
    echo "  ⚠ Low score ($SCORE) but PR was merged"
    SUSPICIOUS=$((SUSPICIOUS + 1))
    SUSPICIOUS_PRS+=("PR #$PR_NUMBER: Low score ($SCORE) merged")
    continue
  fi
  
  echo "  ✓ Score: $SCORE/10"
  PASSED=$((PASSED + 1))
  echo ""
done

# Output summary
echo ""
echo "============================================"
echo "Audit Summary"
echo "============================================"
echo "Total PRs audited: $TOTAL"
echo "Passed: $PASSED"
echo "Missing AWK Review: $MISSING_REVIEW"
echo "Suspicious: $SUSPICIOUS"
echo ""

if [[ ${#SUSPICIOUS_PRS[@]} -gt 0 ]]; then
  echo "Suspicious PRs:"
  for item in "${SUSPICIOUS_PRS[@]}"; do
    echo "  - $item"
  done
  echo ""
  echo "⚠ Some PRs require manual investigation"
  exit 1
fi

echo "✓ All audited PRs passed verification"
exit 0
