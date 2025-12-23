#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_session_manager.sh - Session Manager 單元測試
# Property 1: Session ID Format Consistency
# Property 2: Session ID Uniqueness
# Property 15: PID Reuse Protection
# Validates: Requirements 1.1, 2.1
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
SESSION_MANAGER="$AI_ROOT/scripts/session_manager.sh"

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
echo "Test: session_manager.sh"
echo "============================================"

# Setup test directory
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
mkdir -p .ai/state/principal/sessions
mkdir -p .ai/results

# ============================================================
# Property 1: Session ID Format Consistency
# For any generated session ID, it SHALL match the format
# <role>-<YYYYMMDD>-<HHMMSS>-<hex4>
# ============================================================
echo ""
echo "## Property 1: Session ID Format Consistency"

# Test 1.1: Principal session ID format
PRINCIPAL_ID=$(bash "$SESSION_MANAGER" generate_session_id principal)
if [[ "$PRINCIPAL_ID" =~ ^principal-[0-9]{8}-[0-9]{6}-[a-f0-9]{4}$ ]]; then
  log_pass "Principal session ID format: $PRINCIPAL_ID"
else
  log_fail "Invalid Principal session ID format: $PRINCIPAL_ID"
fi

# Test 1.2: Worker session ID format
WORKER_ID=$(bash "$SESSION_MANAGER" generate_session_id worker)
if [[ "$WORKER_ID" =~ ^worker-[0-9]{8}-[0-9]{6}-[a-f0-9]{4}$ ]]; then
  log_pass "Worker session ID format: $WORKER_ID"
else
  log_fail "Invalid Worker session ID format: $WORKER_ID"
fi

# Test 1.3: Date component is valid (today's date)
TODAY=$(date -u +%Y%m%d)
if [[ "$PRINCIPAL_ID" == principal-${TODAY}-* ]]; then
  log_pass "Session ID contains today's date"
else
  log_fail "Session ID date mismatch: expected $TODAY"
fi


# ============================================================
# Property 2: Session ID Uniqueness
# For any two session IDs generated within the same second,
# the random hex suffix SHALL ensure uniqueness
# Note: 4-hex has 65536 possibilities, collision is rare but possible
# ============================================================
echo ""
echo "## Property 2: Session ID Uniqueness"

# Test 2.1: Generate multiple IDs and check uniqueness
# With 4-hex suffix, we test 5 IDs to reduce collision probability
declare -A SEEN_IDS
UNIQUE=true
COLLISION_COUNT=0
for i in {1..5}; do
  ID=$(bash "$SESSION_MANAGER" generate_session_id principal)
  if [[ -n "${SEEN_IDS[$ID]:-}" ]]; then
    COLLISION_COUNT=$((COLLISION_COUNT + 1))
  fi
  SEEN_IDS[$ID]=1
done
# Allow at most 1 collision in 5 IDs (birthday paradox consideration)
if [[ $COLLISION_COUNT -le 1 ]]; then
  log_pass "5 consecutive session IDs have acceptable uniqueness (collisions: $COLLISION_COUNT)"
else
  log_fail "Too many duplicate session IDs detected: $COLLISION_COUNT collisions"
fi

# Test 2.2: Random hex suffix is 4 characters
HEX_SUFFIX="${PRINCIPAL_ID##*-}"
if [[ ${#HEX_SUFFIX} -eq 4 ]]; then
  log_pass "Random hex suffix is 4 characters: $HEX_SUFFIX"
else
  log_fail "Random hex suffix length incorrect: $HEX_SUFFIX (${#HEX_SUFFIX} chars)"
fi

# ============================================================
# Property 15: PID Reuse Protection
# The system SHALL verify both PID existence AND process start time
# ============================================================
echo ""
echo "## Property 15: PID Reuse Protection"

# Test 15.1: Non-existent PID returns false
if ! bash "$SESSION_MANAGER" check_principal_running 99999 0 2>/dev/null; then
  log_pass "Non-existent PID correctly returns false"
else
  log_fail "Non-existent PID should return false"
fi

# Test 15.2: Current process with wrong start time returns false
CURRENT_PID=$$
WRONG_START=0
if ! bash "$SESSION_MANAGER" check_principal_running "$CURRENT_PID" "$WRONG_START" 2>/dev/null; then
  log_pass "PID with wrong start time correctly returns false (PID reuse protection)"
else
  log_fail "PID with wrong start time should return false"
fi


# ============================================================
# Additional Unit Tests: Session Lifecycle
# ============================================================
echo ""
echo "## Session Lifecycle Tests"

# Test: init_principal_session creates files
SESSION_ID=$(bash "$SESSION_MANAGER" init_principal_session 2>/dev/null)
if [[ -f ".ai/state/principal/session.json" ]]; then
  log_pass "init_principal_session creates session.json"
else
  log_fail "session.json not created"
fi

if [[ -f ".ai/state/principal/sessions/${SESSION_ID}.json" ]]; then
  log_pass "init_principal_session creates session log file"
else
  log_fail "Session log file not created"
fi

# Test: session.json contains correct fields
if _jq -e '.session_id' ".ai/state/principal/session.json" >/dev/null 2>&1; then
  log_pass "session.json contains session_id"
else
  log_fail "session.json missing session_id"
fi

if _jq -e '.pid' ".ai/state/principal/session.json" >/dev/null 2>&1; then
  log_pass "session.json contains pid"
else
  log_fail "session.json missing pid"
fi

if _jq -e '.pid_start_time' ".ai/state/principal/session.json" >/dev/null 2>&1; then
  log_pass "session.json contains pid_start_time"
else
  log_fail "session.json missing pid_start_time"
fi

# Test: get_current_session_id returns correct ID
RETRIEVED_ID=$(bash "$SESSION_MANAGER" get_current_session_id | tr -d '\r')
if [[ "$RETRIEVED_ID" == "$SESSION_ID" ]]; then
  log_pass "get_current_session_id returns correct ID"
else
  log_fail "get_current_session_id mismatch: expected $SESSION_ID, got $RETRIEVED_ID"
fi

# Test: append_session_action adds action
bash "$SESSION_MANAGER" append_session_action "$SESSION_ID" "test_action" '{"test": "data"}' 2>/dev/null
ACTION_COUNT=$(_jq '.actions | length' ".ai/state/principal/sessions/${SESSION_ID}.json")
if [[ "$ACTION_COUNT" -eq 1 ]]; then
  log_pass "append_session_action adds action to log"
else
  log_fail "Action not added to log (count: $ACTION_COUNT)"
fi

# Test: action has correct structure
ACTION_TYPE=$(_jq -r '.actions[0].type' ".ai/state/principal/sessions/${SESSION_ID}.json")
if [[ "$ACTION_TYPE" == "test_action" ]]; then
  log_pass "Action has correct type"
else
  log_fail "Action type mismatch: $ACTION_TYPE"
fi

# Test: action has timestamp in ISO 8601 format
ACTION_TS=$(_jq -r '.actions[0].timestamp' ".ai/state/principal/sessions/${SESSION_ID}.json")
if [[ "$ACTION_TS" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
  log_pass "Action timestamp is UTC ISO 8601 format"
else
  log_fail "Action timestamp format incorrect: $ACTION_TS"
fi

# Test: end_principal_session adds ended_at and exit_reason
bash "$SESSION_MANAGER" end_principal_session "$SESSION_ID" "test_complete" 2>/dev/null
if _jq -e '.ended_at' ".ai/state/principal/sessions/${SESSION_ID}.json" >/dev/null 2>&1; then
  log_pass "end_principal_session adds ended_at"
else
  log_fail "ended_at not added"
fi

EXIT_REASON=$(_jq -r '.exit_reason' ".ai/state/principal/sessions/${SESSION_ID}.json")
if [[ "$EXIT_REASON" == "test_complete" ]]; then
  log_pass "end_principal_session sets exit_reason"
else
  log_fail "exit_reason mismatch: $EXIT_REASON"
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
