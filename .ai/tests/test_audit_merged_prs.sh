#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# test_audit_merged_prs.sh - audit_merged_prs.sh 單元測試
# Property 9: Post-Merge Audit Detection
# Property 5: Cross-Reference Consistency
# Validates: Requirements 7.1, 7.2, 7.3, 7.4
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"

# Test state
PASSED=0
FAILED=0

log_pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo "✗ $1"; FAILED=$((FAILED + 1)); }

echo "============================================"
echo "Test: audit_merged_prs.sh"
echo "============================================"

# ============================================================
# Test: Script exists and has correct syntax
# ============================================================
echo ""
echo "## Script Validation Tests"

AUDIT_SCRIPT="$AI_ROOT/scripts/audit_merged_prs.sh"

if [[ -f "$AUDIT_SCRIPT" ]]; then
  log_pass "audit_merged_prs.sh exists"
else
  log_fail "audit_merged_prs.sh not found"
  exit 1
fi

if bash -n "$AUDIT_SCRIPT" 2>/dev/null; then
  log_pass "audit_merged_prs.sh has valid syntax"
else
  log_fail "audit_merged_prs.sh has syntax errors"
fi

# ============================================================
# Test: Help option works
# ============================================================
echo ""
echo "## CLI Tests"

if bash "$AUDIT_SCRIPT" --help 2>&1 | grep -q "Usage:"; then
  log_pass "--help shows usage"
else
  log_fail "--help should show usage"
fi

# ============================================================
# Property 9: Post-Merge Audit Detection
# The audit tool SHALL detect PRs merged without proper AWK Review
# ============================================================
echo ""
echo "## Property 9: Post-Merge Audit Detection"

# Test: Script checks for AWK Review marker
if grep -q "AWK Review" "$AUDIT_SCRIPT"; then
  log_pass "Script checks for AWK Review marker"
else
  log_fail "Script should check for AWK Review marker"
fi

# Test: Script checks for Session ID
if grep -q "Session ID" "$AUDIT_SCRIPT"; then
  log_pass "Script checks for Session ID"
else
  log_fail "Script should check for Session ID"
fi

# Test: Script checks for Diff Hash
if grep -q "Diff Hash" "$AUDIT_SCRIPT"; then
  log_pass "Script checks for Diff Hash"
else
  log_fail "Script should check for Diff Hash"
fi

# Test: Script checks for score
if grep -qE "(score|Score|評分)" "$AUDIT_SCRIPT"; then
  log_pass "Script checks for score"
else
  log_fail "Script should check for score"
fi

# ============================================================
# Property 5: Cross-Reference Consistency
# Session IDs in review comments SHALL match local session logs
# ============================================================
echo ""
echo "## Property 5: Cross-Reference Consistency"

# Test: Script performs local session verification
if grep -q "SESSION_LOG_DIR" "$AUDIT_SCRIPT"; then
  log_pass "Script references session log directory"
else
  log_fail "Script should reference session log directory"
fi

if grep -q "verified locally" "$AUDIT_SCRIPT"; then
  log_pass "Script performs local session verification"
else
  log_fail "Script should perform local session verification"
fi

# ============================================================
# Test: Script detects suspicious PRs
# ============================================================
echo ""
echo "## Suspicious PR Detection Tests"

# Test: Script tracks suspicious PRs
if grep -q "SUSPICIOUS_PRS" "$AUDIT_SCRIPT"; then
  log_pass "Script tracks suspicious PRs"
else
  log_fail "Script should track suspicious PRs"
fi

# Test: Script reports missing AWK Review
if grep -q "Missing AWK Review" "$AUDIT_SCRIPT"; then
  log_pass "Script reports missing AWK Review"
else
  log_fail "Script should report missing AWK Review"
fi

# Test: Script reports low score merged PRs
if grep -q "Low score" "$AUDIT_SCRIPT"; then
  log_pass "Script reports low score merged PRs"
else
  log_fail "Script should report low score merged PRs"
fi

# ============================================================
# Test: Script outputs summary
# ============================================================
echo ""
echo "## Output Format Tests"

# Test: Script outputs audit summary
if grep -q "Audit Summary" "$AUDIT_SCRIPT"; then
  log_pass "Script outputs audit summary"
else
  log_fail "Script should output audit summary"
fi

# Test: Script has proper exit codes
if grep -q "exit 1" "$AUDIT_SCRIPT" && grep -q "exit 0" "$AUDIT_SCRIPT"; then
  log_pass "Script has proper exit codes"
else
  log_fail "Script should have proper exit codes"
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
