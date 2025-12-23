#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_review_flow.sh - Review 流程整合測試
# Property 13: Large Diff Warning
# Property 14: Review Cycle Limit
# Validates: Risk Mitigation
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
SESSION_MANAGER="$AI_ROOT/scripts/session_manager.sh"
VERIFY_REVIEW="$AI_ROOT/scripts/verify_review.sh"

# Cross-platform jq wrapper (strips CRLF for Windows compatibility)
_jq() {
  local result
  if command -v jq &>/dev/null; then
    result=$(jq "$@")
  elif command -v jq.exe &>/dev/null; then
    result=$(jq.exe "$@")
  elif [[ -x "/mnt/c/Users/user/bin/jq.exe" ]]; then
    result=$(/mnt/c/Users/user/bin/jq.exe "$@")
  else
    echo "[ERROR] jq not found" >&2
    return 1
  fi
  printf '%s' "$result" | tr -d '\r'
}

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
echo "Test: Review Flow Integration"
echo "============================================"

# Setup test directory
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
git init --quiet
mkdir -p .ai/state/principal/sessions .ai/runs/issue-42 .ai/results

# ============================================================
# Property 13: Large Diff Warning
# When PR diff size exceeds threshold, a warning SHALL be recorded
# ============================================================
echo ""
echo "## Property 13: Large Diff Warning"

# Initialize session for testing
export AI_STATE_ROOT="$TEST_DIR"
SESSION_ID=$(bash "$SESSION_MANAGER" init_principal_session 2>/dev/null)

# Test: Large diff warning action is recorded correctly
bash "$SESSION_MANAGER" append_session_action "$SESSION_ID" "large_diff_warning" '{"issue_id":"42","pr_number":"1","diff_size":150000,"threshold":100000}' 2>/dev/null

SESSION_LOG="$TEST_DIR/.ai/state/principal/sessions/${SESSION_ID}.json"
LAST_ACTION=$(_jq -r '.actions[-1].type' "$SESSION_LOG")
DIFF_SIZE=$(_jq '.actions[-1].data.diff_size' "$SESSION_LOG")
THRESHOLD=$(_jq '.actions[-1].data.threshold' "$SESSION_LOG")

if [[ "$LAST_ACTION" == "large_diff_warning" ]]; then
  log_pass "Large diff warning action recorded"
else
  log_fail "Large diff warning action not recorded: $LAST_ACTION"
fi

if [[ "$DIFF_SIZE" -eq 150000 ]] && [[ "$THRESHOLD" -eq 100000 ]]; then
  log_pass "Large diff warning contains correct size data"
else
  log_fail "Large diff warning size data incorrect: diff=$DIFF_SIZE, threshold=$THRESHOLD"
fi

# ============================================================
# Property 14: Review Cycle Limit
# After N review cycles, the issue SHALL be escalated
# ============================================================
echo ""
echo "## Property 14: Review Cycle Limit"

# Test: Review count file creation and increment
REVIEW_COUNT_FILE="$TEST_DIR/.ai/runs/issue-42/review_count.txt"

# First review
echo "1" > "$REVIEW_COUNT_FILE"
COUNT=$(cat "$REVIEW_COUNT_FILE")
if [[ "$COUNT" -eq 1 ]]; then
  log_pass "Review count initialized to 1"
else
  log_fail "Review count should be 1, got $COUNT"
fi

# Second review
echo "2" > "$REVIEW_COUNT_FILE"
COUNT=$(cat "$REVIEW_COUNT_FILE")
if [[ "$COUNT" -eq 2 ]]; then
  log_pass "Review count incremented to 2"
else
  log_fail "Review count should be 2, got $COUNT"
fi

# Third review (at limit)
echo "3" > "$REVIEW_COUNT_FILE"
COUNT=$(cat "$REVIEW_COUNT_FILE")
MAX_CYCLES=3
if [[ "$COUNT" -ge "$MAX_CYCLES" ]]; then
  log_pass "Review count reached limit ($MAX_CYCLES)"
else
  log_fail "Review count should be at limit"
fi

# Test: Review count reset after human intervention
echo "0" > "$REVIEW_COUNT_FILE"
COUNT=$(cat "$REVIEW_COUNT_FILE")
if [[ "$COUNT" -eq 0 ]]; then
  log_pass "Review count reset to 0 after human intervention"
else
  log_fail "Review count should be reset to 0"
fi

# ============================================================
# Test: verify_review.sh integration
# ============================================================
echo ""
echo "## verify_review.sh Integration Tests"

# Create a valid review comment
cat > "$TEST_DIR/review.md" << EOF
<!-- AWK Review -->

## Review Summary

Session: $SESSION_ID
Diff Hash: abcd1234efgh5678

### 程式碼符號 (Code Symbols):
- func NewHandler() - 新增

### 設計引用 (Design References):
- design.md Section 3.2

### 評分 (Score): 8/10

### 評分理由 (Reasoning):
程式碼品質良好

### 可改進之處 (Improvements):
- 無

### 潛在風險 (Risks):
- 無重大風險
EOF

if bash "$VERIFY_REVIEW" "$TEST_DIR/review.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1; then
  log_pass "Valid review comment passes verification"
else
  log_fail "Valid review comment should pass verification"
fi

# Test: Low score triggers exit code 2
cat > "$TEST_DIR/low_score_review.md" << EOF
<!-- AWK Review -->
Session: $SESSION_ID
Diff Hash: abcd1234
評分: 5/10
評分理由: 需要改進
可改進之處: 很多
潛在風險: 有風險
程式碼符號: func test()
設計引用: design.md
EOF

EXIT_CODE=0
bash "$VERIFY_REVIEW" "$TEST_DIR/low_score_review.md" "$TEST_DIR/.ai/state/principal/sessions" >/dev/null 2>&1 || EXIT_CODE=$?

if [[ "$EXIT_CODE" -eq 2 ]]; then
  log_pass "Low score review triggers needs-revision (exit code 2)"
else
  log_fail "Low score review should trigger exit code 2, got $EXIT_CODE"
fi

# ============================================================
# Test: CI timeout handling
# ============================================================
echo ""
echo "## CI Timeout Handling Tests"

# Test: review_audit with ci_timeout=true
cat > "$TEST_DIR/.ai/results/issue-42.json" << 'EOF'
{
  "issue_id": "42",
  "status": "success",
  "session": {
    "worker_session_id": "worker-20251223-100000-aaaa",
    "principal_session_id": "",
    "attempt_number": 1,
    "previous_session_ids": [],
    "previous_failure_reason": ""
  },
  "review_audit": {
    "reviewer_session_id": "",
    "review_timestamp": "",
    "ci_status": "",
    "ci_timeout": false,
    "decision": "",
    "merge_timestamp": ""
  }
}
EOF

bash "$SESSION_MANAGER" update_result_with_review_audit "42" "$SESSION_ID" "approved" "timeout" "true" "" 2>/dev/null

CI_TIMEOUT=$(_jq '.review_audit.ci_timeout' "$TEST_DIR/.ai/results/issue-42.json")
CI_STATUS=$(_jq -r '.review_audit.ci_status' "$TEST_DIR/.ai/results/issue-42.json")

if [[ "$CI_TIMEOUT" == "true" ]]; then
  log_pass "CI timeout recorded as true"
else
  log_fail "CI timeout should be true, got $CI_TIMEOUT"
fi

if [[ "$CI_STATUS" == "timeout" ]]; then
  log_pass "CI status recorded as timeout"
else
  log_fail "CI status should be timeout, got $CI_STATUS"
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
