#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_analyze_failure.sh - 測試錯誤分析功能
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"

PASSED=0
FAILED=0

log_pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo "✗ $1"; FAILED=$((FAILED + 1)); }

echo "============================================"
echo "Test: analyze_failure.sh"
echo "============================================"

# Test 1: Go compile error
echo ""
echo "## Go compile error"
RESULT=$(echo "cannot find package foo" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"type": "compile_error"'; then
  log_pass "Detected Go compile error"
else
  log_fail "Failed to detect Go compile error: $RESULT"
fi

# Test 2: Node test failure
echo ""
echo "## Node test failure"
RESULT=$(echo "FAIL src/test.js" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"type": "test_failure"'; then
  log_pass "Detected Node test failure"
else
  log_fail "Failed to detect Node test failure: $RESULT"
fi

# Test 3: Network timeout (retryable)
echo ""
echo "## Network timeout"
RESULT=$(echo "ETIMEDOUT connection refused" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"retryable": true'; then
  log_pass "Detected retryable network error"
else
  log_fail "Failed to detect retryable network error: $RESULT"
fi

# Test 4: Rate limit (retryable)
echo ""
echo "## Rate limit"
RESULT=$(echo "API rate limit exceeded" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"retryable": true'; then
  log_pass "Detected retryable rate limit"
else
  log_fail "Failed to detect rate limit: $RESULT"
fi

# Test 5: Git conflict (not retryable)
echo ""
echo "## Git conflict"
RESULT=$(echo "CONFLICT (content): Merge conflict in file.txt" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"type": "git_error"' && echo "$RESULT" | grep -q '"retryable": false'; then
  log_pass "Detected non-retryable git conflict"
else
  log_fail "Failed to detect git conflict: $RESULT"
fi

# Test 6: Unknown error
echo ""
echo "## Unknown error"
RESULT=$(echo "some random text" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"matched": false'; then
  log_pass "Unknown error returns matched=false"
else
  log_fail "Unknown error should return matched=false: $RESULT"
fi

# Test 7: Python syntax error
echo ""
echo "## Python syntax error"
RESULT=$(echo "SyntaxError: invalid syntax" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"type": "compile_error"'; then
  log_pass "Detected Python syntax error"
else
  log_fail "Failed to detect Python syntax error: $RESULT"
fi

# Test 8: Rust compile error
echo ""
echo "## Rust compile error"
RESULT=$(echo "error[E0425]: cannot find value" | bash "$AI_ROOT/scripts/analyze_failure.sh" -)
if echo "$RESULT" | grep -q '"pattern_id": "rust_compile_error"'; then
  log_pass "Detected Rust compile error"
else
  log_fail "Failed to detect Rust compile error: $RESULT"
fi

echo ""
echo "============================================"
echo "Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [[ $FAILED -gt 0 ]]; then
  exit 1
fi
exit 0
