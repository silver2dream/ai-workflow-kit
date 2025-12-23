#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_worker_prompt_isolation.sh - Worker Prompt 隔離測試
# Property 8: Worker Prompt Isolation
# Validates: Requirements 3.4, 3.5
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"

# Test state
PASSED=0
FAILED=0

log_pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo "✗ $1"; FAILED=$((FAILED + 1)); }

echo "============================================"
echo "Test: Worker Prompt Isolation"
echo "============================================"

# ============================================================
# Property 8: Worker Prompt Isolation
# Worker prompts SHALL NOT contain paths to Principal session data
# ============================================================
echo ""
echo "## Property 8: Worker Prompt Isolation"

RUN_ISSUE_CODEX="$AI_ROOT/scripts/run_issue_codex.sh"

if [[ ! -f "$RUN_ISSUE_CODEX" ]]; then
  log_fail "run_issue_codex.sh not found"
  exit 1
fi

# Test: Script contains FORBIDDEN OPERATIONS section
if grep -q "FORBIDDEN OPERATIONS" "$RUN_ISSUE_CODEX"; then
  log_pass "Script contains FORBIDDEN OPERATIONS section"
else
  log_fail "Script should contain FORBIDDEN OPERATIONS section"
fi

# Test: Forbidden paths are listed
if grep -q ".ai/state/principal" "$RUN_ISSUE_CODEX"; then
  log_pass "Script forbids access to .ai/state/principal/"
else
  log_fail "Script should forbid access to .ai/state/principal/"
fi

if grep -q "session.json" "$RUN_ISSUE_CODEX"; then
  log_pass "Script forbids access to session.json"
else
  log_fail "Script should forbid access to session.json"
fi

if grep -q ".ai/scripts/" "$RUN_ISSUE_CODEX"; then
  log_pass "Script forbids modification of .ai/scripts/"
else
  log_fail "Script should forbid modification of .ai/scripts/"
fi

if grep -q ".ai/commands/" "$RUN_ISSUE_CODEX"; then
  log_pass "Script forbids modification of .ai/commands/"
else
  log_fail "Script should forbid modification of .ai/commands/"
fi

# Test: Prompt does not expose Principal session paths in WORK_DIR_INSTRUCTION
# The prompt template should not reference .ai/state/principal anywhere
PROMPT_SECTION=$(sed -n '/cat > "\$PROMPT_FILE"/,/^PROMPT$/p' "$RUN_ISSUE_CODEX")

if echo "$PROMPT_SECTION" | grep -q "FORBIDDEN OPERATIONS"; then
  log_pass "Prompt includes FORBIDDEN OPERATIONS warning"
else
  log_fail "Prompt should include FORBIDDEN OPERATIONS warning"
fi

# Test: Prompt warns about session ID forgery
if grep -q "session IDs" "$RUN_ISSUE_CODEX"; then
  log_pass "Prompt warns about session ID manipulation"
else
  log_fail "Prompt should warn about session ID manipulation"
fi

# Test: Prompt warns about audit log access
if grep -q "audit logs" "$RUN_ISSUE_CODEX"; then
  log_pass "Prompt warns about audit log access"
else
  log_fail "Prompt should warn about audit log access"
fi

# ============================================================
# Test: Verify prompt does not contain actual Principal paths
# ============================================================
echo ""
echo "## Path Exposure Tests"

# The prompt should not contain hardcoded paths to session files
# Check that the prompt template doesn't accidentally expose paths
if ! echo "$PROMPT_SECTION" | grep -qE '\$\{?SESSION_FILE\}?|\$\{?SESSION_LOG_DIR\}?'; then
  log_pass "Prompt does not expose SESSION_FILE or SESSION_LOG_DIR variables"
else
  log_fail "Prompt should not expose session file variables"
fi

# Check that PRINCIPAL_SESSION_ID is not exposed in prompt
if ! echo "$PROMPT_SECTION" | grep -qE '\$\{?PRINCIPAL_SESSION_ID\}?'; then
  log_pass "Prompt does not expose PRINCIPAL_SESSION_ID"
else
  log_fail "Prompt should not expose PRINCIPAL_SESSION_ID"
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
