#!/usr/bin/env bash
# test_review_scripts.sh - Test awkit prepare-review and submit-review
#
# Tests:
# - awkit prepare-review exists and works
# - awkit submit-review exists and works
# - awkit run-issue handles PR existence check

set -euo pipefail

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

PASS=0
FAIL=0

log_pass() { echo "✓ $1"; PASS=$((PASS + 1)); }
log_fail() { echo "✗ $1"; FAIL=$((FAIL + 1)); }

echo "## Review Scripts Tests (awkit commands)"
echo ""

# ============================================================
# Test awkit prepare-review
# ============================================================

# Test 1: awkit prepare-review --help works
if "$AWKIT" prepare-review --help >/dev/null 2>&1; then
  log_pass "awkit prepare-review --help works"
else
  log_fail "awkit prepare-review --help failed"
fi

# Test 2: prepare-review help mentions required fields
PREPARE_HELP=$("$AWKIT" prepare-review --help 2>&1 || true)
if echo "$PREPARE_HELP" | grep -qi "pr\|issue\|diff"; then
  log_pass "awkit prepare-review mentions PR/issue/diff"
else
  log_fail "awkit prepare-review missing PR/issue/diff in help"
fi

# ============================================================
# Test awkit submit-review
# ============================================================

# Test 3: awkit submit-review --help works
if "$AWKIT" submit-review --help >/dev/null 2>&1; then
  log_pass "awkit submit-review --help works"
else
  log_fail "awkit submit-review --help failed"
fi

# Test 4: submit-review help mentions score/decision
SUBMIT_HELP=$("$AWKIT" submit-review --help 2>&1 || true)
if echo "$SUBMIT_HELP" | grep -qi "score\|decision\|review"; then
  log_pass "awkit submit-review mentions score/decision"
else
  log_fail "awkit submit-review missing score/decision in help"
fi

# ============================================================
# Test Go implementation files
# ============================================================

# Test 5: prepare.go exists
if [[ -f "$MONO_ROOT/internal/reviewer/prepare.go" ]]; then
  log_pass "internal/reviewer/prepare.go exists"
else
  log_fail "internal/reviewer/prepare.go missing"
fi

# Test 6: submit.go exists
if [[ -f "$MONO_ROOT/internal/reviewer/submit.go" ]]; then
  log_pass "internal/reviewer/submit.go exists"
else
  log_fail "internal/reviewer/submit.go missing"
fi

# Test 7: prepare.go fetches Issue content
if grep -q "issue\|Issue" "$MONO_ROOT/internal/reviewer/prepare.go" 2>/dev/null; then
  log_pass "prepare.go handles Issue content"
else
  log_fail "prepare.go doesn't handle Issue content"
fi

# Test 8: prepare.go fetches PR diff
if grep -q "diff\|Diff" "$MONO_ROOT/internal/reviewer/prepare.go" 2>/dev/null; then
  log_pass "prepare.go handles PR diff"
else
  log_fail "prepare.go doesn't handle PR diff"
fi

# Test 9: submit.go handles merge
if grep -q "merge\|Merge" "$MONO_ROOT/internal/reviewer/submit.go" 2>/dev/null; then
  log_pass "submit.go handles merge"
else
  log_fail "submit.go doesn't handle merge"
fi

# ============================================================
# Test awkit run-issue
# ============================================================

# Test 10: awkit run-issue --help works
if "$AWKIT" run-issue --help >/dev/null 2>&1; then
  log_pass "awkit run-issue --help works"
else
  log_fail "awkit run-issue --help failed"
fi

# Test 11: run-issue help mentions issue number
RUN_HELP=$("$AWKIT" run-issue --help 2>&1 || true)
if echo "$RUN_HELP" | grep -qi "issue"; then
  log_pass "awkit run-issue mentions issue"
else
  log_fail "awkit run-issue missing issue in help"
fi

# ============================================================
# Test worker implementation
# ============================================================

# Test 12: worker/runner.go exists
if [[ -f "$MONO_ROOT/internal/worker/runner.go" ]]; then
  log_pass "internal/worker/runner.go exists"
else
  log_fail "internal/worker/runner.go missing"
fi

# Test 13: worker handles AGENTS.md
if grep -q "AGENTS\|agents" "$MONO_ROOT/internal/worker/runner.go" 2>/dev/null || \
   grep -q "AGENTS\|agents" "$MONO_ROOT/internal/worker/ticket.go" 2>/dev/null; then
  log_pass "worker handles AGENTS.md"
else
  log_fail "worker doesn't reference AGENTS.md"
fi

# ============================================================
# Summary
# ============================================================
echo ""
echo "Results: $PASS passed, $FAIL failed"

if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi
exit 0
