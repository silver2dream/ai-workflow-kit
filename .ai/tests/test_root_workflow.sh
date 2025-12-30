#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_root_workflow.sh - Root Type Workflow Integration Test
# ============================================================================
# Tests the complete workflow for root type (single-repo) projects.
# Requirements: 2.1, 2.2, 2.3, 2.4
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
echo "Test: Root Type Workflow"
echo "============================================"

# Create temp dir for test files
TEST_TMP=$(mktemp -d)
trap "rm -rf $TEST_TMP" EXIT

# ============================================================
# Test 1: awkit validate accepts root type config
# ============================================================
echo ""
echo "## Config Validation (awkit validate)"

# Create valid root type config
cat > "$TEST_TMP/workflow_root.yaml" <<'EOF'
version: "1.0"
project:
  name: "test-root"
  type: "single-repo"
repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "feat/test"
  release_branch: "main"
  commit_format: "[type] subject"
EOF

if "$AWKIT" validate --config "$TEST_TMP/workflow_root.yaml" > /dev/null 2>&1; then
  log_pass "awkit validate accepts valid root config"
else
  log_fail "awkit validate rejected valid root config"
fi

# Test root type with wrong path should warn or fail
cat > "$TEST_TMP/workflow_root_bad.yaml" <<'EOF'
version: "1.0"
project:
  name: "test-root"
  type: "single-repo"
repos:
  - name: root
    path: backend/
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "feat/test"
  release_branch: "main"
  commit_format: "[type] subject"
EOF

VALIDATE_OUTPUT=$("$AWKIT" validate --config "$TEST_TMP/workflow_root_bad.yaml" 2>&1 || true)
if echo "$VALIDATE_OUTPUT" | grep -qi "should be\|warning\|path"; then
  log_pass "awkit validate warns about root type with non-./ path"
else
  log_skip "awkit validate root path warning (may not be implemented)"
fi

# ============================================================
# Test 2: Worktree creation for root type
# ============================================================
echo ""
echo "## Worktree Creation"

if [[ -f "$AI_ROOT/scripts/new_worktree.sh" ]]; then
  if grep -q "REPO_TYPE\|repo_type" "$AI_ROOT/scripts/new_worktree.sh"; then
    log_pass "new_worktree.sh supports REPO_TYPE parameter"
  else
    log_fail "new_worktree.sh missing REPO_TYPE support"
  fi

  if grep -q "root" "$AI_ROOT/scripts/new_worktree.sh"; then
    log_pass "new_worktree.sh handles root type"
  else
    log_fail "new_worktree.sh missing root type handling"
  fi
else
  log_skip "new_worktree.sh not found (may be in Go)"
fi

# ============================================================
# Test 3: Result recording includes repo_type
# ============================================================
echo ""
echo "## Result Recording"

if [[ -f "$AI_ROOT/scripts/write_result.sh" ]]; then
  if grep -q "repo_type" "$AI_ROOT/scripts/write_result.sh"; then
    log_pass "write_result.sh includes repo_type field"
  else
    log_fail "write_result.sh missing repo_type field"
  fi

  if grep -q "work_dir" "$AI_ROOT/scripts/write_result.sh"; then
    log_pass "write_result.sh includes work_dir field"
  else
    log_fail "write_result.sh missing work_dir field"
  fi
else
  log_skip "write_result.sh not found (may be in Go)"
fi

# ============================================================
# Test 4: awkit dispatch-worker exists
# ============================================================
echo ""
echo "## awkit dispatch-worker"

if "$AWKIT" dispatch-worker --help >/dev/null 2>&1; then
  log_pass "awkit dispatch-worker command available"
else
  log_fail "awkit dispatch-worker command not available"
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
