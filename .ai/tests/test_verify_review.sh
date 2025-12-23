#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_verify_review.sh - verify_review.sh 單元測試
# Property 12: Session ID Local Verification (Anti-Forgery)
# Validates: Requirements 5.3, 7.4
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
VERIFY_REVIEW="$AI_ROOT/scripts/verify_review.sh"

# Test state
PASSED=0
FAILED=0
TEST_DIR=""

# Cleanup function
cleanup() {
  if [[ -n "$TEST_DIR" ]] && [[ -d "$TEST_DIR" ]]; then
    rm -rf "$TEST_DIR"
  fi
}
trap cleanup EXIT

log_pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo "✗ $1"; FAILED=$((FAILED + 1)); }

echo "============================================"
echo "Test: verify_review.sh"
echo "============================================"

# Setup test directory
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
mkdir -p .ai/state/principal/sessions

# ============================================================
# Test: Valid review comment passes
# ============================================================
echo ""
echo "## Valid Review Tests"

cat > "$TEST_DIR/valid_review.md" << 'EOF'
<!-- AWK Review -->

## Review Summary

Session: principal-20251223-100000-aaaa
Diff Hash: abcd1234efgh5678

### 程式碼符號 (Code Symbols):
- `func NewHandler()` - 新增
- `class UserService` - 修改

### 設計引用 (Design References):
- design.md Section 3.2

### 評分 (Score): 8/10

### 評分理由 (Reasoning):
程式碼品質良好，符合架構規範。

### 可改進之處 (Improvements):
- 可以增加更多單元測試

### 潛在風險 (Risks):
- 無重大風險
EOF

# Create matching session log
cat > "$TEST_DIR/.ai/state/principal/sessions/principal-20251223-100000-aaaa.json" << 'EOF'
{"session_id": "principal-20251223-100000-aaaa", "started_at": "2025-12-23T10:00:00Z"}
EOF

if bash "$VERIFY_REVIEW" "$TEST_DIR/valid_review.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Valid review passes verification"
else
  log_fail "Valid review should pass verification"
fi

# ============================================================
# Test: Missing AWK marker fails
# ============================================================
echo ""
echo "## Missing Fields Tests"

cat > "$TEST_DIR/no_marker.md" << 'EOF'
## Review Summary
Session: principal-20251223-100000-aaaa
Diff Hash: abcd1234
評分: 8/10
EOF

if ! bash "$VERIFY_REVIEW" "$TEST_DIR/no_marker.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Missing AWK marker fails verification"
else
  log_fail "Missing AWK marker should fail verification"
fi

# ============================================================
# Test: Missing Session ID fails
# ============================================================
cat > "$TEST_DIR/no_session.md" << 'EOF'
<!-- AWK Review -->
Diff Hash: abcd1234
評分: 8/10
EOF

if ! bash "$VERIFY_REVIEW" "$TEST_DIR/no_session.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Missing Session ID fails verification"
else
  log_fail "Missing Session ID should fail verification"
fi

# ============================================================
# Test: Missing Diff Hash fails
# ============================================================
cat > "$TEST_DIR/no_hash.md" << 'EOF'
<!-- AWK Review -->
Session: principal-20251223-100000-aaaa
評分: 8/10
EOF

if ! bash "$VERIFY_REVIEW" "$TEST_DIR/no_hash.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Missing Diff Hash fails verification"
else
  log_fail "Missing Diff Hash should fail verification"
fi

# ============================================================
# Test: Missing score fails
# ============================================================
cat > "$TEST_DIR/no_score.md" << 'EOF'
<!-- AWK Review -->
Session: principal-20251223-100000-aaaa
Diff Hash: abcd1234
EOF

if ! bash "$VERIFY_REVIEW" "$TEST_DIR/no_score.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Missing score fails verification"
else
  log_fail "Missing score should fail verification"
fi

# ============================================================
# Test: Low score returns exit code 2
# ============================================================
echo ""
echo "## Score Threshold Tests"

cat > "$TEST_DIR/low_score.md" << 'EOF'
<!-- AWK Review -->
Session: principal-20251223-100000-aaaa
Diff Hash: abcd1234
評分: 5/10
評分理由: 需要改進
可改進之處: 很多
潛在風險: 有風險
程式碼符號: func test()
設計引用: design.md
EOF

EXIT_CODE=0
bash "$VERIFY_REVIEW" "$TEST_DIR/low_score.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1 || EXIT_CODE=$?

if [[ "$EXIT_CODE" -eq 2 ]]; then
  log_pass "Low score (5) returns exit code 2"
else
  log_fail "Low score should return exit code 2, got $EXIT_CODE"
fi

# ============================================================
# Test: Score exactly 7 passes
# ============================================================
cat > "$TEST_DIR/score_7.md" << 'EOF'
<!-- AWK Review -->
Session: principal-20251223-100000-aaaa
Diff Hash: abcd1234
評分: 7/10
評分理由: 可接受
可改進之處: 一些
潛在風險: 低
程式碼符號: func test()
設計引用: design.md
EOF

if bash "$VERIFY_REVIEW" "$TEST_DIR/score_7.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Score 7 passes verification"
else
  log_fail "Score 7 should pass verification"
fi

# ============================================================
# Property 12: Session ID Local Verification (Anti-Forgery)
# ============================================================
echo ""
echo "## Property 12: Session ID Local Verification"

# Test: Session ID format validation
cat > "$TEST_DIR/invalid_session_format.md" << 'EOF'
<!-- AWK Review -->
Session: invalid-session-id
Diff Hash: abcd1234
評分: 8/10
EOF

if ! bash "$VERIFY_REVIEW" "$TEST_DIR/invalid_session_format.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Invalid session ID format fails verification"
else
  log_fail "Invalid session ID format should fail verification"
fi

# Test: Valid format but non-existent session (should warn but not fail)
cat > "$TEST_DIR/unknown_session.md" << 'EOF'
<!-- AWK Review -->
Session: principal-20251223-999999-ffff
Diff Hash: abcd1234
評分: 8/10
評分理由: OK
可改進之處: None
潛在風險: None
程式碼符號: func test()
設計引用: design.md
EOF

# This should pass (with warning) since the session format is valid
if bash "$VERIFY_REVIEW" "$TEST_DIR/unknown_session.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Unknown but valid-format session ID passes (with warning)"
else
  log_fail "Unknown but valid-format session ID should pass with warning"
fi

# ============================================================
# Results
# ============================================================
echo ""
echo "============================================"
echo "Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [[ $FAILED -gt 0 ]]; then
  exit 1
fi
exit 0
