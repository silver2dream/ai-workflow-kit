#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_submodule_workflow.sh - Submodule Type Workflow Integration Test
# ============================================================================
# Tests the complete workflow for submodule type projects.
# Requirements: 4.1, 4.2, 4.3, 4.4, 4.5
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

PASSED=0
FAILED=0
SKIPPED=0

log_pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo "✗ $1"; FAILED=$((FAILED + 1)); }
log_skip() { echo "○ $1 (skipped)"; SKIPPED=$((SKIPPED + 1)); }

echo "============================================"
echo "Test: Submodule Type Workflow"
echo "============================================"

# Create temp dir for test files
TEST_TMP=$(mktemp -d)
trap "rm -rf $TEST_TMP" EXIT

# ============================================================
# Test 1: Submodule type validation
# ============================================================
echo ""
echo "## Config Validation"

if grep -q "repo_type == 'submodule'" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py checks submodule type"
else
  log_fail "validate_config.py missing submodule type check"
fi

if grep -q "gitmodules" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py checks .gitmodules"
else
  log_fail "validate_config.py missing .gitmodules check"
fi

# ============================================================
# Test 2: Submodule initialization in worktree
# ============================================================
echo ""
echo "## Worktree Submodule Initialization"

if grep -q "submodule" "$AI_ROOT/scripts/new_worktree.sh"; then
  log_pass "new_worktree.sh handles submodule type"
else
  log_fail "new_worktree.sh missing submodule type handling"
fi

if grep -q "submodule update" "$AI_ROOT/scripts/new_worktree.sh"; then
  log_pass "new_worktree.sh runs git submodule update"
else
  log_fail "new_worktree.sh missing git submodule update"
fi

# ============================================================
# Test 3: Submodule git operations
# ============================================================
echo ""
echo "## Submodule Git Operations"

if grep -q "git_commit_submodule" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "git_commit_submodule function exists"
else
  log_fail "git_commit_submodule function missing"
fi

if grep -q "git_push_submodule" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "git_push_submodule function exists"
else
  log_fail "git_push_submodule function missing"
fi

if grep -q "check_submodule_boundary" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "check_submodule_boundary function exists"
else
  log_fail "check_submodule_boundary function missing"
fi

if grep -q "setup_submodule_branch" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "setup_submodule_branch function exists"
else
  log_fail "setup_submodule_branch function missing"
fi

# ============================================================
# Test 4: Submodule result recording
# ============================================================
echo ""
echo "## Result Recording"

if grep -q "submodule_sha" "$AI_ROOT/scripts/write_result.sh"; then
  log_pass "write_result.sh includes submodule_sha field"
else
  log_fail "write_result.sh missing submodule_sha field"
fi

if grep -q "consistency_status" "$AI_ROOT/scripts/write_result.sh"; then
  log_pass "write_result.sh includes consistency_status field"
else
  log_fail "write_result.sh missing consistency_status field"
fi

if grep -q "recovery_command" "$AI_ROOT/scripts/write_result.sh"; then
  log_pass "write_result.sh includes recovery_command field"
else
  log_fail "write_result.sh missing recovery_command field"
fi

# ============================================================
# Test 5: Submodule rollback and cleanup
# ============================================================
echo ""
echo "## Rollback & Cleanup"

if grep -q "submodule" "$AI_ROOT/scripts/rollback.sh"; then
  log_pass "rollback.sh handles submodule"
else
  log_fail "rollback.sh missing submodule support"
fi

if grep -q "submodule" "$AI_ROOT/scripts/cleanup.sh"; then
  log_pass "cleanup.sh handles submodule branches"
else
  log_fail "cleanup.sh missing submodule branch cleanup"
fi

# ============================================================
# Test 6: Submodule audit and preflight
# ============================================================
echo ""
echo "## Audit & Preflight"

if grep -q "submodule" "$AI_ROOT/scripts/audit_project.py"; then
  log_pass "audit_project.py checks submodule status"
else
  log_fail "audit_project.py missing submodule checks"
fi

if grep -q "submodule" "$AI_ROOT/scripts/preflight.sh"; then
  log_pass "preflight.sh has submodule checks"
else
  log_fail "preflight.sh missing submodule checks"
fi

# ============================================================
# Test 7: Review submodule changes
# ============================================================
echo ""
echo "## Review Support"

if grep -q "submodule" "$AI_ROOT/commands/review-pr.md"; then
  log_pass "review-pr.md handles submodule changes"
else
  log_fail "review-pr.md missing submodule support"
fi

# ============================================================
# Summary
# ============================================================
echo ""
echo "============================================"
echo "Results: $PASSED passed, $FAILED failed, $SKIPPED skipped"
echo "============================================"

if [[ $FAILED -gt 0 ]]; then
  exit 1
fi
exit 0
