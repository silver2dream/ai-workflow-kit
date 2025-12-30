#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_session_manager.sh - Session Manager 單元測試 (awkit session)
# Property 1: Session ID Format Consistency
# Property 2: Session ID Uniqueness
# Property 15: PID Reuse Protection
# Validates: Requirements 1.1, 2.1
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
echo "Test: awkit session (Go implementation)"
echo "============================================"

# Setup test directory
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
git init --quiet
mkdir -p .ai/state/principal/sessions
mkdir -p .ai/results

# ============================================================
# Property 1: Session ID Format Consistency
# For any generated session ID, it SHALL match the format
# <role>-<YYYYMMDD>-<HHMMSS>-<hex4>
# ============================================================
echo ""
echo "## Property 1: Session ID Format Consistency"

# Test 1.1: Principal session ID format (via init)
PRINCIPAL_ID=$("$AWKIT" session init 2>/dev/null | tr -d '\r')
if [[ "$PRINCIPAL_ID" =~ ^principal-[0-9]{8}-[0-9]{6}-[a-f0-9]{4}$ ]]; then
  log_pass "Principal session ID format: $PRINCIPAL_ID"
else
  log_fail "Invalid Principal session ID format: $PRINCIPAL_ID"
fi

# Test 1.2: Date component is valid (today's date)
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

# Test 2.1: Random hex suffix is 4 characters
HEX_SUFFIX="${PRINCIPAL_ID##*-}"
if [[ ${#HEX_SUFFIX} -eq 4 ]]; then
  log_pass "Random hex suffix is 4 characters: $HEX_SUFFIX"
else
  log_fail "Random hex suffix length incorrect: $HEX_SUFFIX (${#HEX_SUFFIX} chars)"
fi

# ============================================================
# Session Lifecycle Tests
# ============================================================
echo ""
echo "## Session Lifecycle Tests"

# Test: init creates session.json
if [[ -f ".ai/state/principal/session.json" ]]; then
  log_pass "session init creates session.json"
else
  log_fail "session.json not created"
fi

# Test: init creates session log file
if [[ -f ".ai/state/principal/sessions/${PRINCIPAL_ID}.json" ]]; then
  log_pass "session init creates session log file"
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

# Test: get-id returns correct ID
RETRIEVED_ID=$("$AWKIT" session get-id 2>/dev/null | tr -d '\r')
if [[ "$RETRIEVED_ID" == "$PRINCIPAL_ID" ]]; then
  log_pass "session get-id returns correct ID"
else
  log_fail "session get-id mismatch: expected $PRINCIPAL_ID, got $RETRIEVED_ID"
fi

# Test: append adds action
"$AWKIT" session append "$PRINCIPAL_ID" "test_action" '{"test": "data"}' 2>/dev/null
ACTION_COUNT=$(_jq '.actions | length' ".ai/state/principal/sessions/${PRINCIPAL_ID}.json")
if [[ "$ACTION_COUNT" -eq 1 ]]; then
  log_pass "session append adds action to log"
else
  log_fail "Action not added to log (count: $ACTION_COUNT)"
fi

# Test: action has correct structure
ACTION_TYPE=$(_jq -r '.actions[0].type' ".ai/state/principal/sessions/${PRINCIPAL_ID}.json")
if [[ "$ACTION_TYPE" == "test_action" ]]; then
  log_pass "Action has correct type"
else
  log_fail "Action type mismatch: $ACTION_TYPE"
fi

# Test: action has timestamp in ISO 8601 format
ACTION_TS=$(_jq -r '.actions[0].timestamp' ".ai/state/principal/sessions/${PRINCIPAL_ID}.json")
if [[ "$ACTION_TS" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2} ]]; then
  log_pass "Action timestamp is ISO 8601 format"
else
  log_fail "Action timestamp format incorrect: $ACTION_TS"
fi

# Test: end adds ended_at and exit_reason
"$AWKIT" session end "$PRINCIPAL_ID" "test_complete" 2>/dev/null
if _jq -e '.ended_at' ".ai/state/principal/sessions/${PRINCIPAL_ID}.json" >/dev/null 2>&1; then
  log_pass "session end adds ended_at"
else
  log_fail "ended_at not added"
fi

EXIT_REASON=$(_jq -r '.exit_reason' ".ai/state/principal/sessions/${PRINCIPAL_ID}.json")
if [[ "$EXIT_REASON" == "test_complete" ]]; then
  log_pass "session end sets exit_reason"
else
  log_fail "exit_reason mismatch: $EXIT_REASON"
fi

# ============================================================
# Regression: init MUST NOT overwrite ended sessions
# ============================================================
echo ""
echo "## Regression: preserve ended session exit_reason"

# Simulate a previous session that ended normally, but whose PID is now stale.
PREV_SESSION_ID="principal-20250101-000000-dead"
mkdir -p ".ai/state/principal/sessions"
cat > ".ai/state/principal/sessions/${PREV_SESSION_ID}.json" <<'EOF'
{
  "session_id": "principal-20250101-000000-dead",
  "started_at": "2025-01-01T00:00:00Z",
  "ended_at": "2025-01-01T00:10:00Z",
  "exit_reason": "all_tasks_complete",
  "actions": []
}
EOF

cat > ".ai/state/principal/session.json" <<EOF
{
  "session_id": "$PREV_SESSION_ID",
  "started_at": "2025-01-01T00:00:00Z",
  "pid": 99999,
  "pid_start_time": 0
}
EOF

NEW_SESSION_ID=$("$AWKIT" session init 2>/dev/null | tr -d '\r')
if [[ -n "$NEW_SESSION_ID" ]]; then
  log_pass "session init succeeds with stale previous PID"
else
  log_fail "session init did not return a new session id"
fi

PREV_EXIT_REASON=$(_jq -r '.exit_reason // ""' ".ai/state/principal/sessions/${PREV_SESSION_ID}.json")
if [[ "$PREV_EXIT_REASON" == "all_tasks_complete" ]]; then
  log_pass "previous ended session exit_reason preserved"
else
  log_fail "previous ended session exit_reason changed: $PREV_EXIT_REASON"
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
