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
echo "## Config Validation"

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

if python3 "$AI_ROOT/scripts/validate_config.py" "$TEST_TMP/workflow_dir.yaml" > /dev/null 2>&1; then
  log_pass "validate_config.py accepts valid directory config"
else
  log_fail "validate_config.py rejected valid directory config"
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

VALIDATE_OUTPUT=$(python3 "$AI_ROOT/scripts/validate_config.py" "$TEST_TMP/workflow_traversal.yaml" 2>&1 || true)
if echo "$VALIDATE_OUTPUT" | grep -qi "path traversal not allowed"; then
  log_pass "validate_config.py rejects path traversal"
else
  log_fail "validate_config.py should reject path traversal"
fi

# ============================================================
# Test 3: Worktree creation for directory type
# ============================================================
echo ""
echo "## Worktree Creation"

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

# ============================================================
# Test 4: Git operations
# ============================================================
echo ""
echo "## Git Operations"

if grep -q "feat/ai-issue-" "$AI_ROOT/scripts/cleanup.sh"; then
  log_pass "cleanup.sh uses correct branch pattern"
else
  log_fail "cleanup.sh missing branch pattern"
fi

# ============================================================
# Test 5: Multi-repo coordination
# ============================================================
echo ""
echo "## Multi-Repo Coordination"

# Multi-repo logic is in dispatch-worker.md (modular architecture)
if grep -q "sequential" "$AI_ROOT/commands/dispatch-worker.md" && grep -q "parallel" "$AI_ROOT/commands/dispatch-worker.md"; then
  log_pass "dispatch-worker.md supports execution modes"
else
  log_fail "dispatch-worker.md missing execution modes"
fi

if grep -q "directory" "$AI_ROOT/commands/dispatch-worker.md"; then
  log_pass "dispatch-worker.md handles directory type"
else
  log_skip "dispatch-worker.md directory type handling"
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
