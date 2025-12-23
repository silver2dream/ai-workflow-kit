#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_github_comment.sh - GitHub Comment Manager 單元測試
# Property 3: GitHub Comment Format Consistency
# Property 10: Issue Comment Source Traceability
# Property 11: Worker Completion Duration Tracking
# Validates: Requirements 4.1, 4.2, 4.3, 4.4
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
GITHUB_COMMENT="$AI_ROOT/scripts/github_comment.sh"

# Test state
PASSED=0
FAILED=0

log_pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo "✗ $1"; FAILED=$((FAILED + 1)); }

echo "============================================"
echo "Test: github_comment.sh"
echo "============================================"

# Source the script to use functions directly
source "$GITHUB_COMMENT"

# ============================================================
# Property 3: GitHub Comment Format Consistency
# All AWK comments SHALL contain the session ID marker
# ============================================================
echo ""
echo "## Property 3: GitHub Comment Format Consistency"

# Test 3.1: AWK comment prefix constant is defined
if [[ -n "$AWK_COMMENT_PREFIX" ]]; then
  log_pass "AWK_COMMENT_PREFIX is defined: $AWK_COMMENT_PREFIX"
else
  log_fail "AWK_COMMENT_PREFIX is not defined"
fi

# Test 3.2: AWK comment suffix constant is defined
if [[ -n "$AWK_COMMENT_SUFFIX" ]]; then
  log_pass "AWK_COMMENT_SUFFIX is defined: $AWK_COMMENT_SUFFIX"
else
  log_fail "AWK_COMMENT_SUFFIX is not defined"
fi

# ============================================================
# Property 10: Issue Comment Source Traceability
# Issue comments SHALL include source field when provided
# ============================================================
echo ""
echo "## Property 10: Issue Comment Source Traceability"

# Test 10.1: extract_session_from_comment extracts valid session ID
TEST_COMMENT="<!-- AWK:session:principal-20251223-120000-abcd --> Some content"
EXTRACTED=$(extract_session_from_comment "$TEST_COMMENT")
if [[ "$EXTRACTED" == "principal-20251223-120000-abcd" ]]; then
  log_pass "extract_session_from_comment extracts principal session ID"
else
  log_fail "Failed to extract session ID: got '$EXTRACTED'"
fi

# Test 10.2: extract_session_from_comment extracts worker session ID
TEST_COMMENT="<!-- AWK:session:worker-20251223-120000-1234 --> Worker comment"
EXTRACTED=$(extract_session_from_comment "$TEST_COMMENT")
if [[ "$EXTRACTED" == "worker-20251223-120000-1234" ]]; then
  log_pass "extract_session_from_comment extracts worker session ID"
else
  log_fail "Failed to extract worker session ID: got '$EXTRACTED'"
fi

# Test 10.3: extract_session_from_comment returns empty for invalid format
TEST_COMMENT="This is a regular comment without AWK marker"
EXTRACTED=$(extract_session_from_comment "$TEST_COMMENT")
if [[ -z "$EXTRACTED" ]]; then
  log_pass "extract_session_from_comment returns empty for non-AWK comment"
else
  log_fail "Should return empty for non-AWK comment: got '$EXTRACTED'"
fi

# Test 10.4: extract_session_from_comment handles malformed session ID
TEST_COMMENT="<!-- AWK:session:invalid-format --> Bad format"
EXTRACTED=$(extract_session_from_comment "$TEST_COMMENT")
if [[ -z "$EXTRACTED" ]]; then
  log_pass "extract_session_from_comment rejects malformed session ID"
else
  log_fail "Should reject malformed session ID: got '$EXTRACTED'"
fi

# ============================================================
# Property 11: Worker Completion Duration Tracking
# Worker completion comments SHALL include duration
# ============================================================
echo ""
echo "## Property 11: Worker Completion Duration Tracking"

# Test 11.1: format_duration handles seconds
FORMATTED=$(format_duration 45)
if [[ "$FORMATTED" == "45s" ]]; then
  log_pass "format_duration handles seconds: $FORMATTED"
else
  log_fail "format_duration seconds failed: expected '45s', got '$FORMATTED'"
fi

# Test 11.2: format_duration handles minutes
FORMATTED=$(format_duration 150)
if [[ "$FORMATTED" == "2m 30s" ]]; then
  log_pass "format_duration handles minutes: $FORMATTED"
else
  log_fail "format_duration minutes failed: expected '2m 30s', got '$FORMATTED'"
fi

# Test 11.3: format_duration handles hours
FORMATTED=$(format_duration 3900)
if [[ "$FORMATTED" == "1h 5m" ]]; then
  log_pass "format_duration handles hours: $FORMATTED"
else
  log_fail "format_duration hours failed: expected '1h 5m', got '$FORMATTED'"
fi

# Test 11.4: build_worker_complete_extra includes PR URL
EXTRA=$(build_worker_complete_extra "https://github.com/test/repo/pull/123" "")
if [[ "$EXTRA" == *"| PR | https://github.com/test/repo/pull/123 |"* ]]; then
  log_pass "build_worker_complete_extra includes PR URL"
else
  log_fail "build_worker_complete_extra missing PR URL"
fi

# Test 11.5: build_worker_complete_extra includes duration
EXTRA=$(build_worker_complete_extra "" "120")
if [[ "$EXTRA" == *"| Duration | 2m 0s |"* ]]; then
  log_pass "build_worker_complete_extra includes formatted duration"
else
  log_fail "build_worker_complete_extra missing duration: got '$EXTRA'"
fi

# Test 11.6: build_worker_complete_extra includes both PR and duration
EXTRA=$(build_worker_complete_extra "https://github.com/test/repo/pull/456" "90")
if [[ "$EXTRA" == *"| PR |"* ]] && [[ "$EXTRA" == *"| Duration |"* ]]; then
  log_pass "build_worker_complete_extra includes both PR and duration"
else
  log_fail "build_worker_complete_extra missing fields"
fi

# ============================================================
# Additional Unit Tests: Comment Format Validation
# ============================================================
echo ""
echo "## Comment Format Validation Tests"

# Test: Session ID regex pattern validation
VALID_SESSIONS=(
  "principal-20251223-120000-abcd"
  "worker-20240101-235959-0000"
  "principal-20251231-000000-ffff"
)

for sid in "${VALID_SESSIONS[@]}"; do
  TEST_COMMENT="<!-- AWK:session:${sid} -->"
  EXTRACTED=$(extract_session_from_comment "$TEST_COMMENT")
  if [[ "$EXTRACTED" == "$sid" ]]; then
    log_pass "Valid session ID accepted: $sid"
  else
    log_fail "Valid session ID rejected: $sid"
  fi
done

# Test: Invalid session ID patterns
INVALID_SESSIONS=(
  "principal-2025123-120000-abcd"   # Wrong date format (7 digits)
  "worker-20251223-12000-abcd"      # Wrong time format (5 digits)
  "principal-20251223-120000-abc"   # Wrong hex format (3 chars)
  "principal-20251223-120000-ABCD"  # Uppercase hex (should be lowercase)
  "admin-20251223-120000-abcd"      # Invalid role
)

for sid in "${INVALID_SESSIONS[@]}"; do
  TEST_COMMENT="<!-- AWK:session:${sid} -->"
  EXTRACTED=$(extract_session_from_comment "$TEST_COMMENT")
  if [[ -z "$EXTRACTED" ]]; then
    log_pass "Invalid session ID rejected: $sid"
  else
    log_fail "Invalid session ID should be rejected: $sid (got: $EXTRACTED)"
  fi
done

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
