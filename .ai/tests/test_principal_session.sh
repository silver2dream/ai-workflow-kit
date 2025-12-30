#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_principal_session.sh - Principal Session 整合測試 (awkit session)
# Property 7: Strict Sequential Execution
# Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 5.7
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
echo "Test: Principal Session Integration"
echo "============================================"

# Setup test directory
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
git init --quiet
mkdir -p .ai/state/principal/sessions .ai/results

# ============================================================
# Property 7: Strict Sequential Execution
# Only one Principal session SHALL be active at any time
# ============================================================
echo ""
echo "## Property 7: Strict Sequential Execution"

# Test 7.1: First Principal session initializes successfully
SESSION_ID=$("$AWKIT" session init 2>/dev/null | tr -d '\r')

if [[ -n "$SESSION_ID" ]] && [[ "$SESSION_ID" =~ ^principal-[0-9]{8}-[0-9]{6}-[a-f0-9]{4}$ ]]; then
  log_pass "First Principal session initialized: $SESSION_ID"
else
  log_fail "First Principal session failed to initialize"
fi

# Test 7.2: Session file created with correct structure
SESSION_FILE="$TEST_DIR/.ai/state/principal/session.json"
if [[ -f "$SESSION_FILE" ]]; then
  log_pass "Session file created"
  
  # Verify session file has required fields
  HAS_SID=$(_jq -r '.session_id // ""' "$SESSION_FILE")
  HAS_PID=$(_jq '.pid // 0' "$SESSION_FILE")
  HAS_START=$(_jq '.pid_start_time // 0' "$SESSION_FILE")
  
  if [[ "$HAS_SID" == "$SESSION_ID" ]]; then
    log_pass "Session file contains correct session_id"
  else
    log_fail "Session file session_id mismatch"
  fi
  
  if [[ "$HAS_PID" -gt 0 ]]; then
    log_pass "Session file contains PID"
  else
    log_fail "Session file missing PID"
  fi
else
  log_fail "Session file not created"
fi

# Test 7.3: Session log file created
SESSION_LOG="$TEST_DIR/.ai/state/principal/sessions/${SESSION_ID}.json"
if [[ -f "$SESSION_LOG" ]]; then
  log_pass "Session log file created"
  
  # Verify log has actions array
  ACTIONS_COUNT=$(_jq '.actions | length' "$SESSION_LOG")
  if [[ "$ACTIONS_COUNT" -eq 0 ]]; then
    log_pass "Session log has empty actions array"
  else
    log_fail "Session log should start with empty actions"
  fi
else
  log_fail "Session log file not created"
fi

# ============================================================
# Test Action Recording (Req 1.4)
# ============================================================
echo ""
echo "## Action Recording Tests"

# Test: Append issue_created action
"$AWKIT" session append "$SESSION_ID" "issue_created" '{"issue_id":"42","title":"Test Issue"}' 2>/dev/null

ACTIONS_COUNT=$(_jq '.actions | length' "$SESSION_LOG")
if [[ "$ACTIONS_COUNT" -eq 1 ]]; then
  log_pass "issue_created action appended"
else
  log_fail "issue_created action not appended, count: $ACTIONS_COUNT"
fi

# Test: Append worker_dispatched action
"$AWKIT" session append "$SESSION_ID" "worker_dispatched" '{"issue_id":"42"}' 2>/dev/null

ACTIONS_COUNT=$(_jq '.actions | length' "$SESSION_LOG")
if [[ "$ACTIONS_COUNT" -eq 2 ]]; then
  log_pass "worker_dispatched action appended"
else
  log_fail "worker_dispatched action not appended"
fi

# Test: Append worker_completed action
"$AWKIT" session append "$SESSION_ID" "worker_completed" '{"issue_id":"42","worker_session_id":"worker-20251223-100000-aaaa","status":"success","pr_url":"https://github.com/test/pr/1"}' 2>/dev/null

ACTIONS_COUNT=$(_jq '.actions | length' "$SESSION_LOG")
LAST_ACTION=$(_jq -r '.actions[-1].type' "$SESSION_LOG")
if [[ "$ACTIONS_COUNT" -eq 3 ]] && [[ "$LAST_ACTION" == "worker_completed" ]]; then
  log_pass "worker_completed action recorded"
else
  log_fail "worker_completed action not recorded correctly"
fi

# ============================================================
# Test Session End (Req 1.6)
# ============================================================
echo ""
echo "## Session End Tests"

# Test: End session with reason
"$AWKIT" session end "$SESSION_ID" "all_tasks_complete" 2>/dev/null

ENDED_AT=$(_jq -r '.ended_at // ""' "$SESSION_LOG")
EXIT_REASON=$(_jq -r '.exit_reason // ""' "$SESSION_LOG")

if [[ -n "$ENDED_AT" ]]; then
  log_pass "Session ended_at recorded"
else
  log_fail "Session ended_at not recorded"
fi

if [[ "$EXIT_REASON" == "all_tasks_complete" ]]; then
  log_pass "Session exit_reason recorded correctly"
else
  log_fail "Session exit_reason mismatch: $EXIT_REASON"
fi

# ============================================================
# Test Result JSON Updates (Req 6.3, 6.4)
# ============================================================
echo ""
echo "## Result JSON Update Tests"

# Create a mock result.json
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

# Test: update-result
"$AWKIT" session update-result "42" "$SESSION_ID" 2>/dev/null

RESULT_PSID=$(_jq -r '.session.principal_session_id' "$TEST_DIR/.ai/results/issue-42.json")
if [[ "$RESULT_PSID" == "$SESSION_ID" ]]; then
  log_pass "Result JSON principal_session_id updated"
else
  log_fail "Result JSON principal_session_id not updated: $RESULT_PSID"
fi

# Test: update-review
"$AWKIT" session update-review "42" "$SESSION_ID" "approved" "passed" "false" "2025-12-23T10:00:00Z" 2>/dev/null

REVIEW_DEC=$(_jq -r '.review_audit.decision' "$TEST_DIR/.ai/results/issue-42.json")
REVIEW_CI=$(_jq -r '.review_audit.ci_status' "$TEST_DIR/.ai/results/issue-42.json")
REVIEW_MERGE=$(_jq -r '.review_audit.merge_timestamp' "$TEST_DIR/.ai/results/issue-42.json")

if [[ "$REVIEW_DEC" == "approved" ]]; then
  log_pass "Review audit decision updated"
else
  log_fail "Review audit decision not updated: $REVIEW_DEC"
fi

if [[ "$REVIEW_CI" == "passed" ]]; then
  log_pass "Review audit ci_status updated"
else
  log_fail "Review audit ci_status not updated: $REVIEW_CI"
fi

if [[ "$REVIEW_MERGE" == "2025-12-23T10:00:00Z" ]]; then
  log_pass "Review audit merge_timestamp updated"
else
  log_fail "Review audit merge_timestamp not updated: $REVIEW_MERGE"
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
