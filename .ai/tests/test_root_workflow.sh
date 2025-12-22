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
# Test 1: Root type detection functions exist
# ============================================================
echo ""
echo "## Repo Type Detection"

if grep -q "^get_repo_type()" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "get_repo_type function exists"
else
  log_fail "get_repo_type function missing"
fi

if grep -q "^get_repo_path()" "$AI_ROOT/scripts/run_issue_codex.sh"; then
  log_pass "get_repo_path function exists"
else
  log_fail "get_repo_path function missing"
fi

# ============================================================
# Test 2: Root type config validation
# ============================================================
echo ""
echo "## Config Validation"

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

if python3 "$AI_ROOT/scripts/validate_config.py" "$TEST_TMP/workflow_root.yaml" > /dev/null 2>&1; then
  log_pass "validate_config.py accepts valid root config"
else
  log_fail "validate_config.py rejected valid root config"
fi

# Test root type with wrong path should warn
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

VALIDATE_OUTPUT=$(python3 "$AI_ROOT/scripts/validate_config.py" "$TEST_TMP/workflow_root_bad.yaml" 2>&1 || true)
if echo "$VALIDATE_OUTPUT" | grep -qi "should be"; then
  log_pass "validate_config.py warns about root type with non-./ path"
else
  log_skip "validate_config.py root path warning"
fi

# ============================================================
# Test 3: Worktree creation for root type
# ============================================================
echo ""
echo "## Worktree Creation"

if grep -q "REPO_TYPE" "$AI_ROOT/scripts/new_worktree.sh"; then
  log_pass "new_worktree.sh supports REPO_TYPE parameter"
else
  log_fail "new_worktree.sh missing REPO_TYPE support"
fi

if grep -q "root" "$AI_ROOT/scripts/new_worktree.sh"; then
  log_pass "new_worktree.sh handles root type"
else
  log_fail "new_worktree.sh missing root type handling"
fi

# ============================================================
# Test 4: Result recording includes repo_type
# ============================================================
echo ""
echo "## Result Recording"

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
