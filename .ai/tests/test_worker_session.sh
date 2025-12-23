#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_worker_session.sh - Worker Session 整合測試
# Property 6: Retry Chain Integrity
# Validates: Requirements 8.1, 8.2, 8.4
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
WRITE_RESULT="$AI_ROOT/scripts/write_result.sh"

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
  # Strip CRLF and trailing whitespace
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
echo "Test: Worker Session Integration"
echo "============================================"

# Setup test directory
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
git init --quiet
mkdir -p .ai/results

# ============================================================
# Property 6: Retry Chain Integrity
# For any Issue with multiple Worker attempts, the session chain
# SHALL maintain complete history of all previous session IDs
# ============================================================
echo ""
echo "## Property 6: Retry Chain Integrity"

# Test 6.1: First attempt has attempt_number=1 and empty previous_session_ids
export AI_STATE_ROOT="$TEST_DIR"
export AI_RESULTS_ROOT="$TEST_DIR"
export AI_REPO_NAME="test"
export AI_BRANCH_NAME="feat/test"
export AI_PR_BASE="main"
export WORKER_SESSION_ID="worker-20251223-100000-aaaa"
export AI_ATTEMPT_NUMBER=1
export AI_PREV_SESSION_IDS="[]"
export AI_PREV_FAILURE_REASON=""
export AI_EXEC_DURATION=60
export AI_RETRY_COUNT=0

bash "$WRITE_RESULT" "42" "success" "https://github.com/test/pr/1" "" 2>/dev/null

RESULT_FILE="$TEST_DIR/.ai/results/issue-42.json"
if [[ -f "$RESULT_FILE" ]]; then
  ATTEMPT=$(_jq '.session.attempt_number' "$RESULT_FILE")
  PREV_IDS=$(_jq -c '.session.previous_session_ids' "$RESULT_FILE")
  WORKER_SID=$(_jq -r '.session.worker_session_id' "$RESULT_FILE")
  
  if [[ "$ATTEMPT" -eq 1 ]]; then
    log_pass "First attempt has attempt_number=1"
  else
    log_fail "First attempt should have attempt_number=1, got $ATTEMPT"
  fi
  
  if [[ "$PREV_IDS" == "[]" ]]; then
    log_pass "First attempt has empty previous_session_ids"
  else
    log_fail "First attempt should have empty previous_session_ids, got $PREV_IDS"
  fi
  
  if [[ "$WORKER_SID" == "worker-20251223-100000-aaaa" ]]; then
    log_pass "Worker session ID recorded correctly"
  else
    log_fail "Worker session ID mismatch: $WORKER_SID"
  fi
else
  log_fail "Result file not created"
fi

# Test 6.2: Second attempt increments attempt_number and adds previous session
export WORKER_SESSION_ID="worker-20251223-100100-bbbb"
export AI_ATTEMPT_NUMBER=2
export AI_PREV_SESSION_IDS='["worker-20251223-100000-aaaa"]'
export AI_PREV_FAILURE_REASON="codex exit code 1"

bash "$WRITE_RESULT" "42" "failed" "" "" 2>/dev/null

ATTEMPT=$(_jq '.session.attempt_number' "$RESULT_FILE")
PREV_IDS=$(_jq -c '.session.previous_session_ids' "$RESULT_FILE")
PREV_REASON=$(_jq -r '.session.previous_failure_reason' "$RESULT_FILE")

if [[ "$ATTEMPT" -eq 2 ]]; then
  log_pass "Second attempt has attempt_number=2"
else
  log_fail "Second attempt should have attempt_number=2, got $ATTEMPT"
fi

if [[ "$PREV_IDS" == '["worker-20251223-100000-aaaa"]' ]]; then
  log_pass "Second attempt has previous session in chain"
else
  log_fail "Second attempt should have previous session, got $PREV_IDS"
fi

if [[ "$PREV_REASON" == "codex exit code 1" ]]; then
  log_pass "Previous failure reason recorded"
else
  log_fail "Previous failure reason mismatch: $PREV_REASON"
fi

# Test 6.3: Third attempt maintains full chain
export WORKER_SESSION_ID="worker-20251223-100200-cccc"
export AI_ATTEMPT_NUMBER=3
export AI_PREV_SESSION_IDS='["worker-20251223-100000-aaaa","worker-20251223-100100-bbbb"]'
export AI_PREV_FAILURE_REASON="timeout"

bash "$WRITE_RESULT" "42" "success" "https://github.com/test/pr/2" "" 2>/dev/null

ATTEMPT=$(_jq '.session.attempt_number' "$RESULT_FILE")
PREV_IDS=$(_jq -c '.session.previous_session_ids' "$RESULT_FILE")
CHAIN_LENGTH=$(_jq '.session.previous_session_ids | length' "$RESULT_FILE")

if [[ "$ATTEMPT" -eq 3 ]]; then
  log_pass "Third attempt has attempt_number=3"
else
  log_fail "Third attempt should have attempt_number=3, got $ATTEMPT"
fi

if [[ "$CHAIN_LENGTH" -eq 2 ]]; then
  log_pass "Third attempt has 2 previous sessions in chain"
else
  log_fail "Third attempt should have 2 previous sessions, got $CHAIN_LENGTH"
fi

# ============================================================
# Additional Tests: Result JSON Schema
# ============================================================
echo ""
echo "## Result JSON Schema Tests"

# Test: session section exists
if _jq -e '.session' "$RESULT_FILE" >/dev/null 2>&1; then
  log_pass "Result JSON has session section"
else
  log_fail "Result JSON missing session section"
fi

# Test: review_audit section exists
if _jq -e '.review_audit' "$RESULT_FILE" >/dev/null 2>&1; then
  log_pass "Result JSON has review_audit section"
else
  log_fail "Result JSON missing review_audit section"
fi

# Test: metrics section exists
if _jq -e '.metrics' "$RESULT_FILE" >/dev/null 2>&1; then
  log_pass "Result JSON has metrics section"
else
  log_fail "Result JSON missing metrics section"
fi

# Test: principal_session_id is initially empty
PRINCIPAL_SID=$(_jq -r '.session.principal_session_id' "$RESULT_FILE")
if [[ -z "$PRINCIPAL_SID" ]]; then
  log_pass "principal_session_id is initially empty"
else
  log_fail "principal_session_id should be empty initially, got $PRINCIPAL_SID"
fi

# Test: review_audit fields are initially empty
REVIEWER_SID=$(_jq -r '.review_audit.reviewer_session_id' "$RESULT_FILE")
CI_TIMEOUT=$(_jq '.review_audit.ci_timeout' "$RESULT_FILE")

if [[ -z "$REVIEWER_SID" ]]; then
  log_pass "reviewer_session_id is initially empty"
else
  log_fail "reviewer_session_id should be empty initially"
fi

if [[ "$CI_TIMEOUT" == "false" ]]; then
  log_pass "ci_timeout defaults to false"
else
  log_fail "ci_timeout should default to false"
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
