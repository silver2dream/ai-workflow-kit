#!/usr/bin/env bash
# test_review_scripts.sh - Test prepare_review.sh and submit_review.sh
#
# Tests:
# - prepare_review.sh exists and has correct structure
# - submit_review.sh exists and handles all cases
# - run_issue_codex.sh has PR existence check
# - run_issue_codex.sh reads Issue comments for retry

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"

PASS=0
FAIL=0

log_pass() { echo "✓ $1"; PASS=$((PASS + 1)); }
log_fail() { echo "✗ $1"; FAIL=$((FAIL + 1)); }

echo "## Review Scripts Tests"
echo ""

# ============================================================
# Test prepare_review.sh
# ============================================================

# Test 1: prepare_review.sh exists
if [[ -f "$AI_ROOT/scripts/prepare_review.sh" ]]; then
  log_pass "prepare_review.sh exists"
else
  log_fail "prepare_review.sh missing"
fi

# Test 2: prepare_review.sh outputs required fields
if grep -q "PRINCIPAL_SESSION_ID" "$AI_ROOT/scripts/prepare_review.sh" && \
   grep -q "CI_STATUS" "$AI_ROOT/scripts/prepare_review.sh" && \
   grep -q "DIFF_HASH" "$AI_ROOT/scripts/prepare_review.sh" && \
   grep -q "WORKTREE_PATH" "$AI_ROOT/scripts/prepare_review.sh"; then
  log_pass "prepare_review.sh outputs required fields"
else
  log_fail "prepare_review.sh missing required fields"
fi

# Test 3: prepare_review.sh fetches Issue content
if grep -q "gh issue view" "$AI_ROOT/scripts/prepare_review.sh"; then
  log_pass "prepare_review.sh fetches Issue content"
else
  log_fail "prepare_review.sh doesn't fetch Issue content"
fi

# Test 4: prepare_review.sh fetches PR diff
if grep -q "gh pr diff" "$AI_ROOT/scripts/prepare_review.sh"; then
  log_pass "prepare_review.sh fetches PR diff"
else
  log_fail "prepare_review.sh doesn't fetch PR diff"
fi

# ============================================================
# Test submit_review.sh
# ============================================================

# Test 5: submit_review.sh exists
if [[ -f "$AI_ROOT/scripts/submit_review.sh" ]]; then
  log_pass "submit_review.sh exists"
else
  log_fail "submit_review.sh missing"
fi

# Test 6: submit_review.sh handles score < 7 (changes_requested)
if grep -q "changes_requested" "$AI_ROOT/scripts/submit_review.sh"; then
  log_pass "submit_review.sh handles changes_requested"
else
  log_fail "submit_review.sh missing changes_requested handling"
fi

# Test 7: submit_review.sh handles CI failure
if grep -q "approved_ci_failed" "$AI_ROOT/scripts/submit_review.sh"; then
  log_pass "submit_review.sh handles CI failure"
else
  log_fail "submit_review.sh missing CI failure handling"
fi

# Test 8: submit_review.sh posts Issue comment on rejection
if grep -q 'gh issue comment.*AWK Review' "$AI_ROOT/scripts/submit_review.sh"; then
  log_pass "submit_review.sh posts Issue comment on rejection"
else
  log_fail "submit_review.sh doesn't post Issue comment on rejection"
fi

# Test 9: submit_review.sh updates labels on rejection
if grep -q 'remove-label.*pr-ready.*add-label.*ai-task' "$AI_ROOT/scripts/submit_review.sh"; then
  log_pass "submit_review.sh updates labels on rejection"
else
  log_fail "submit_review.sh doesn't update labels on rejection"
fi

# Test 10: submit_review.sh merges PR on success
if grep -q "gh pr merge" "$AI_ROOT/scripts/submit_review.sh"; then
  log_pass "submit_review.sh merges PR on success"
else
  log_fail "submit_review.sh doesn't merge PR"
fi

# Test 11: submit_review.sh updates tasks.md on merge
if grep -q 'sed.*\[x\]' "$AI_ROOT/scripts/submit_review.sh"; then
  log_pass "submit_review.sh updates tasks.md on merge"
else
  log_fail "submit_review.sh doesn't update tasks.md"
fi

# ============================================================
# Test run_issue_codex.sh modifications
# ============================================================

# Test 12: run_issue_codex.sh checks for existing PR
if grep -q "gh pr view.*BRANCH" "$AI_ROOT/scripts/run_issue_codex.sh" || \
   grep -q "EXISTING_PR_URL" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "run_issue_codex.sh checks for existing PR"
else
  log_fail "run_issue_codex.sh doesn't check for existing PR"
fi

# Test 13: run_issue_codex.sh reads Issue comments for retry
if grep -q "AWK_REVIEW_COMMENTS" "$AI_ROOT/scripts/run_issue_codex.sh" || \
   grep -q "PREVIOUS REVIEW FEEDBACK" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "run_issue_codex.sh reads Issue comments for retry"
else
  log_fail "run_issue_codex.sh doesn't read Issue comments"
fi

# Test 14: run_issue_codex.sh only reads AGENTS.md (not CLAUDE.md)
if grep -q "Read and follow AGENTS.md" "$AI_ROOT/scripts/run_issue_codex.sh" && \
   ! grep -q "Read and follow CLAUDE.md" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "run_issue_codex.sh only reads AGENTS.md"
else
  log_fail "run_issue_codex.sh still references CLAUDE.md"
fi

# ============================================================
# Summary
# ============================================================
echo ""
echo "Results: $PASS passed, $FAIL failed"

if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi
exit 0
