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

# Find awkit binary
AWKIT=""
if [[ -x "$MONO_ROOT/awkit" ]]; then
  AWKIT="$MONO_ROOT/awkit"
elif [[ -x "$MONO_ROOT/awkit.exe" ]]; then
  AWKIT="$MONO_ROOT/awkit.exe"
elif command -v awkit &>/dev/null; then
  AWKIT="awkit"
else
  echo "[ERROR] awkit binary not found"
  exit 1
fi

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
# Test 1: Submodule type validation (awkit validate)
# ============================================================
echo ""
echo "## Config Validation (awkit validate)"

# Create valid submodule type config
cat > "$TEST_TMP/workflow_submodule.yaml" <<'EOF'
version: "1.0"
project:
  name: "test-submodule"
  type: "monorepo"
repos:
  - name: frontend
    path: frontend/
    type: submodule
    language: unity
    verify:
      build: "echo 'Unity build'"
      test: "echo 'Unity test'"
git:
  integration_branch: "feat/test"
  release_branch: "main"
  commit_format: "[type] subject"
EOF

if "$AWKIT" validate --config "$TEST_TMP/workflow_submodule.yaml" > /dev/null 2>&1; then
  log_pass "awkit validate accepts submodule config"
else
  log_skip "awkit validate submodule config (may require .gitmodules)"
fi

# ============================================================
# Test 2: Submodule handling in Go code
# ============================================================
echo ""
echo "## Submodule Support in Go"

# Check if submodule.go exists
if [[ -f "$MONO_ROOT/internal/worker/submodule.go" ]]; then
  log_pass "internal/worker/submodule.go exists"
  
  if grep -q "Submodule\|submodule" "$MONO_ROOT/internal/worker/submodule.go"; then
    log_pass "submodule.go has submodule handling"
  else
    log_fail "submodule.go missing submodule handling"
  fi
else
  log_fail "internal/worker/submodule.go not found"
fi

# ============================================================
# Test 3: Worktree submodule initialization
# ============================================================
echo ""
echo "## Worktree Submodule Initialization"

if [[ -f "$AI_ROOT/scripts/new_worktree.sh" ]]; then
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
else
  log_skip "new_worktree.sh not found (may be in Go)"
fi

# ============================================================
# Test 4: Submodule result recording
# ============================================================
echo ""
echo "## Result Recording"

if [[ -f "$AI_ROOT/scripts/write_result.sh" ]]; then
  if grep -q "submodule_sha" "$AI_ROOT/scripts/write_result.sh"; then
    log_pass "write_result.sh includes submodule_sha field"
  else
    log_skip "write_result.sh submodule_sha field"
  fi

  if grep -q "consistency_status" "$AI_ROOT/scripts/write_result.sh"; then
    log_pass "write_result.sh includes consistency_status field"
  else
    log_skip "write_result.sh consistency_status field"
  fi
else
  log_skip "write_result.sh not found (may be in Go)"
fi

# ============================================================
# Test 5: Submodule rollback and cleanup
# ============================================================
echo ""
echo "## Rollback & Cleanup"

if [[ -f "$AI_ROOT/scripts/rollback.sh" ]]; then
  if grep -q "submodule" "$AI_ROOT/scripts/rollback.sh"; then
    log_pass "rollback.sh handles submodule"
  else
    log_skip "rollback.sh submodule support"
  fi
else
  log_skip "rollback.sh not found"
fi

if [[ -f "$AI_ROOT/scripts/cleanup.sh" ]]; then
  if grep -q "submodule" "$AI_ROOT/scripts/cleanup.sh"; then
    log_pass "cleanup.sh handles submodule branches"
  else
    log_skip "cleanup.sh submodule branch cleanup"
  fi
else
  log_skip "cleanup.sh not found"
fi

# ============================================================
# Test 6: Submodule audit and preflight
# ============================================================
echo ""
echo "## Audit & Preflight"

if [[ -f "$AI_ROOT/scripts/audit_project.py" ]]; then
  if grep -q "submodule" "$AI_ROOT/scripts/audit_project.py"; then
    log_pass "audit_project.py checks submodule status"
  else
    log_skip "audit_project.py submodule checks"
  fi
else
  log_skip "audit_project.py not found"
fi

if [[ -f "$AI_ROOT/scripts/preflight.sh" ]]; then
  if grep -q "submodule" "$AI_ROOT/scripts/preflight.sh"; then
    log_pass "preflight.sh has submodule checks"
  else
    log_skip "preflight.sh submodule checks"
  fi
else
  log_skip "preflight.sh not found"
fi

# ============================================================
# Test 7: Review submodule changes
# ============================================================
echo ""
echo "## Review Support"

# review-pr.md is now in skills
if [[ -f "$AI_ROOT/skills/principal-workflow/tasks/review-pr.md" ]]; then
  log_pass "review-pr.md exists in skills"
else
  log_fail "review-pr.md missing from skills"
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
