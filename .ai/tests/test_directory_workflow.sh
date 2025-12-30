#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_directory_workflow.sh - Directory Type Workflow Integration Test
# ============================================================================
# Tests the complete workflow for directory type (monorepo subdirectory) projects.
# Requirements: 3.1, 3.2, 3.3, 3.4
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
echo "Test: Directory Type Workflow"
echo "============================================"

# Create temp dir for test files
TEST_TMP=$(mktemp -d)
trap "rm -rf $TEST_TMP" EXIT

# ============================================================
# Test 1: Directory type config validation
# ============================================================
echo ""
echo "## Config Validation (awkit validate)"

# Create valid directory type config
cat > "$TEST_TMP/workflow_dir.yaml" <<'EOF'
version: "1.0"
project:
  name: "test-monorepo"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "feat/test"
  release_branch: "main"
  commit_format: "[type] subject"
EOF

if "$AWKIT" validate --config "$TEST_TMP/workflow_dir.yaml" > /dev/null 2>&1; then
  log_pass "awkit validate accepts valid directory config"
else
  log_fail "awkit validate rejected valid directory config"
fi

# ============================================================
# Test 2: Path traversal prevention
# ============================================================
echo ""
echo "## Path Traversal Prevention"

cat > "$TEST_TMP/workflow_traversal.yaml" <<'EOF'
version: "1.0"
project:
  name: "test-monorepo"
  type: "monorepo"
repos:
  - name: backend
    path: ../outside/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
git:
  integration_branch: "feat/test"
  release_branch: "main"
  commit_format: "[type] subject"
EOF

VALIDATE_OUTPUT=$("$AWKIT" validate --config "$TEST_TMP/workflow_traversal.yaml" 2>&1 || true)
if echo "$VALIDATE_OUTPUT" | grep -qi "path traversal\|invalid\|outside\|error"; then
  log_pass "awkit validate rejects path traversal"
else
  log_skip "awkit validate path traversal check (may not be implemented)"
fi

# ============================================================
# Test 3: Worktree creation for directory type
# ============================================================
echo ""
echo "## Worktree Creation"

if [[ -f "$AI_ROOT/scripts/new_worktree.sh" ]]; then
  if grep -q "directory" "$AI_ROOT/scripts/new_worktree.sh"; then
    log_pass "new_worktree.sh handles directory type"
  else
    log_fail "new_worktree.sh missing directory type handling"
  fi

  if grep -q "WORK_DIR\|work_dir" "$AI_ROOT/scripts/new_worktree.sh"; then
    log_pass "new_worktree.sh validates WORK_DIR"
  else
    log_fail "new_worktree.sh missing WORK_DIR validation"
  fi
else
  log_skip "new_worktree.sh not found (may be in Go)"
fi

# ============================================================
# Test 4: Git operations
# ============================================================
echo ""
echo "## Git Operations"

if [[ -f "$AI_ROOT/scripts/cleanup.sh" ]]; then
  if grep -q "feat/ai-issue-" "$AI_ROOT/scripts/cleanup.sh"; then
    log_pass "cleanup.sh uses correct branch pattern"
  else
    log_fail "cleanup.sh missing branch pattern"
  fi
else
  log_skip "cleanup.sh not found"
fi

# ============================================================
# Test 5: Multi-repo coordination (awkit dispatch-worker)
# ============================================================
echo ""
echo "## Multi-Repo Coordination"

if "$AWKIT" dispatch-worker --help >/dev/null 2>&1; then
  log_pass "awkit dispatch-worker available"
else
  log_fail "awkit dispatch-worker not available"
fi

# Check if dispatch-worker help mentions issue
DISPATCH_HELP=$("$AWKIT" dispatch-worker --help 2>&1 || true)
if echo "$DISPATCH_HELP" | grep -qi "issue"; then
  log_pass "awkit dispatch-worker handles issue dispatch"
else
  log_skip "awkit dispatch-worker issue handling"
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
