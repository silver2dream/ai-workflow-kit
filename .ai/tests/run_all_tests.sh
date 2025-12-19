#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# run_all_tests.sh - AI Workflow Kit 測試套件
# ============================================================================
# 用法:
#   bash .ai/tests/run_all_tests.sh [--verbose]
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

VERBOSE="${1:-}"
PASSED=0
FAILED=0
SKIPPED=0

# Simple output (no ANSI colors for Windows compatibility)
log_pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo "✗ $1"; FAILED=$((FAILED + 1)); }
log_skip() { echo "○ $1 (skipped)"; SKIPPED=$((SKIPPED + 1)); }
log_info() { [[ "$VERBOSE" == "--verbose" ]] && echo "  $1" || true; }

echo "============================================"
echo "AI Workflow Kit - Test Suite"
echo "============================================"
echo ""

# ============================================================
# Test 1: Config validation
# ============================================================
echo "## Config Tests"

if python3 "$AI_ROOT/scripts/validate_config.py" > /dev/null 2>&1; then
  log_pass "workflow.yaml is valid"
else
  log_fail "workflow.yaml validation failed"
fi

# ============================================================
# Test 2: Required files exist
# ============================================================
echo ""
echo "## File Structure Tests"

required_files=(
  "$AI_ROOT/config/workflow.yaml"
  "$AI_ROOT/config/workflow.schema.json"
  "$AI_ROOT/scripts/generate.sh"
  "$AI_ROOT/scripts/kickoff.sh"
  "$AI_ROOT/scripts/audit_project.sh"
  "$AI_ROOT/scripts/scan_repo.sh"
  "$AI_ROOT/templates/CLAUDE.md.j2"
  "$AI_ROOT/templates/AGENTS.md.j2"
  "$AI_ROOT/commands/start-work.md"
)

for file in "${required_files[@]}"; do
  if [[ -f "$file" ]]; then
    log_pass "$(basename "$file") exists"
  else
    log_fail "$(basename "$file") missing: $file"
  fi
done

# ============================================================
# Test 3: generate.sh produces valid output
# ============================================================
echo ""
echo "## Generate Tests"

# Create temp dir for test output
TEST_TMP=$(mktemp -d)
trap "rm -rf $TEST_TMP" EXIT

# Copy config to temp
cp "$AI_ROOT/config/workflow.yaml" "$TEST_TMP/"

# Test generate.sh (dry run by checking templates exist)
if [[ -f "$AI_ROOT/templates/CLAUDE.md.j2" ]] && [[ -f "$AI_ROOT/templates/AGENTS.md.j2" ]]; then
  log_pass "Templates exist for generation"
else
  log_fail "Templates missing"
fi

# Check generated files exist
if [[ -f "$MONO_ROOT/CLAUDE.md" ]] && [[ -f "$MONO_ROOT/AGENTS.md" ]]; then
  log_pass "Generated files exist (CLAUDE.md, AGENTS.md)"
else
  log_fail "Generated files missing"
fi

# Check _kit rules directory
if [[ -d "$AI_ROOT/rules/_kit" ]] && [[ -f "$AI_ROOT/rules/_kit/git-workflow.md" ]]; then
  log_pass "Kit rules generated (_kit/git-workflow.md)"
else
  log_fail "Kit rules missing"
fi

# ============================================================
# Test 4: Scripts are executable (Unix only)
# ============================================================
echo ""
echo "## Script Tests"

if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "win32" ]]; then
  log_skip "Executable check (Windows)"
else
  scripts=(
    "$AI_ROOT/scripts/generate.sh"
    "$AI_ROOT/scripts/kickoff.sh"
    "$AI_ROOT/scripts/audit_project.sh"
  )
  for script in "${scripts[@]}"; do
    if [[ -x "$script" ]]; then
      log_pass "$(basename "$script") is executable"
    else
      log_fail "$(basename "$script") not executable"
    fi
  done
fi

# ============================================================
# Test 5: YAML syntax check
# ============================================================
echo ""
echo "## YAML Syntax Tests"

yaml_files=(
  "$AI_ROOT/config/workflow.yaml"
)

for file in "${yaml_files[@]}"; do
  if python3 -c "import yaml; yaml.safe_load(open('$file'))" 2>/dev/null; then
    log_pass "$(basename "$file") valid YAML"
  else
    log_fail "$(basename "$file") invalid YAML"
  fi
done

# ============================================================
# Test 6: JSON syntax check
# ============================================================
echo ""
echo "## JSON Syntax Tests"

json_files=(
  "$AI_ROOT/config/workflow.schema.json"
)

for file in "${json_files[@]}"; do
  if python3 -c "import json; json.load(open('$file'))" 2>/dev/null; then
    log_pass "$(basename "$file") valid JSON"
  else
    log_fail "$(basename "$file") invalid JSON"
  fi
done

# ============================================================
# Test 7: Jinja2 template syntax
# ============================================================
echo ""
echo "## Template Syntax Tests"

templates=(
  "$AI_ROOT/templates/CLAUDE.md.j2"
  "$AI_ROOT/templates/AGENTS.md.j2"
  "$AI_ROOT/templates/git-workflow.md.j2"
)

for template in "${templates[@]}"; do
  if python3 -c "
from jinja2 import Environment, FileSystemLoader, TemplateSyntaxError
import os
env = Environment(loader=FileSystemLoader(os.path.dirname('$template')))
try:
    env.get_template(os.path.basename('$template'))
    exit(0)
except TemplateSyntaxError as e:
    print(e)
    exit(1)
" 2>/dev/null; then
    log_pass "$(basename "$template") valid Jinja2"
  else
    log_fail "$(basename "$template") invalid Jinja2"
  fi
done

# ============================================================
# Test 8: Failure analysis
# ============================================================
echo ""
echo "## Failure Analysis Tests"

if [[ -f "$AI_ROOT/config/failure_patterns.json" ]]; then
  log_pass "failure_patterns.json exists"
else
  log_fail "failure_patterns.json missing"
fi

if [[ -f "$AI_ROOT/scripts/analyze_failure.sh" ]]; then
  log_pass "analyze_failure.sh exists"
  
  # Test basic functionality
  RESULT=$(echo "cannot find package" | bash "$AI_ROOT/scripts/analyze_failure.sh" - 2>/dev/null || echo '{"matched":false}')
  if echo "$RESULT" | grep -q '"matched": true'; then
    log_pass "analyze_failure.sh detects errors"
  else
    log_fail "analyze_failure.sh failed to detect error"
  fi
else
  log_fail "analyze_failure.sh missing"
fi

if [[ -f "$AI_ROOT/scripts/rollback.sh" ]]; then
  log_pass "rollback.sh exists"
else
  log_fail "rollback.sh missing"
fi

if [[ -f "$AI_ROOT/scripts/cleanup.sh" ]]; then
  log_pass "cleanup.sh exists"
else
  log_fail "cleanup.sh missing"
fi

# ============================================================
# Test 9: Stats and History Tracking
# ============================================================
echo ""
echo "## Stats and History Tests"

if [[ -f "$AI_ROOT/scripts/stats.sh" ]]; then
  log_pass "stats.sh exists"
  
  # Test --no-save flag exists in script
  if grep -q "\-\-no-save" "$AI_ROOT/scripts/stats.sh"; then
    log_pass "stats.sh supports --no-save flag"
  else
    log_fail "stats.sh missing --no-save flag"
  fi
  
  # Test trends calculation function exists
  if grep -q "calculate_trends" "$AI_ROOT/scripts/stats.sh"; then
    log_pass "stats.sh has trends calculation"
  else
    log_fail "stats.sh missing trends calculation"
  fi
  
  # Test JSON output includes trends
  if grep -q '"trends"' "$AI_ROOT/scripts/stats.sh"; then
    log_pass "stats.sh JSON includes trends"
  else
    log_fail "stats.sh JSON missing trends"
  fi
  
  # Test metrics aggregation
  if grep -q 'total_duration_seconds' "$AI_ROOT/scripts/stats.sh"; then
    log_pass "stats.sh includes duration metrics"
  else
    log_fail "stats.sh missing duration metrics"
  fi
else
  log_fail "stats.sh missing"
fi

# Test write_result.sh includes metrics
if [[ -f "$AI_ROOT/scripts/write_result.sh" ]]; then
  if grep -q 'metrics' "$AI_ROOT/scripts/write_result.sh"; then
    log_pass "write_result.sh includes metrics"
  else
    log_fail "write_result.sh missing metrics"
  fi
  
  if grep -q 'duration_seconds' "$AI_ROOT/scripts/write_result.sh"; then
    log_pass "write_result.sh tracks duration"
  else
    log_fail "write_result.sh missing duration tracking"
  fi
else
  log_fail "write_result.sh missing"
fi

# Test run_issue_codex.sh tracks execution time
if [[ -f "$AI_ROOT/scripts/run_issue_codex.sh" ]]; then
  if grep -q 'EXEC_START_TIME' "$AI_ROOT/scripts/run_issue_codex.sh"; then
    log_pass "run_issue_codex.sh tracks execution time"
  else
    log_fail "run_issue_codex.sh missing execution time tracking"
  fi
else
  log_fail "run_issue_codex.sh missing"
fi

# ============================================================
# Test 10: Task DAG Parser
# ============================================================
echo ""
echo "## Task DAG Tests"

if [[ -f "$AI_ROOT/scripts/parse_tasks.py" ]]; then
  log_pass "parse_tasks.py exists"
  
  # Test basic parsing
  TEST_TASKS=$(cat <<'EOF'
# Test Tasks
- [ ] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_
- [x] 3. Third task
EOF
  )
  
  PARSE_RESULT=$(echo "$TEST_TASKS" | python3 "$AI_ROOT/scripts/parse_tasks.py" /dev/stdin --json 2>/dev/null || echo "[]")
  if echo "$PARSE_RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); exit(0 if len(d)>=2 else 1)" 2>/dev/null; then
    log_pass "parse_tasks.py parses tasks"
  else
    log_fail "parse_tasks.py failed to parse tasks"
  fi
  
  # Test dependency detection
  if echo "$PARSE_RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); exit(0 if any(t.get('depends_on') for t in d) else 1)" 2>/dev/null; then
    log_pass "parse_tasks.py detects dependencies"
  else
    log_fail "parse_tasks.py failed to detect dependencies"
  fi
  
  # Test --next flag
  NEXT_RESULT=$(echo "$TEST_TASKS" | python3 "$AI_ROOT/scripts/parse_tasks.py" /dev/stdin --next --json 2>/dev/null || echo "[]")
  if echo "$NEXT_RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); exit(0 if isinstance(d, list) else 1)" 2>/dev/null; then
    log_pass "parse_tasks.py --next works"
  else
    log_fail "parse_tasks.py --next failed"
  fi
else
  log_fail "parse_tasks.py missing"
fi

# ============================================================
# Test 11: Multi-Repo Coordination
# ============================================================
echo ""
echo "## Multi-Repo Tests"

# Test start-work.md has multi-repo support
if grep -q 'Coordination' "$AI_ROOT/commands/start-work.md"; then
  log_pass "start-work.md supports Coordination"
else
  log_fail "start-work.md missing Coordination support"
fi

if grep -q 'sequential' "$AI_ROOT/commands/start-work.md" && grep -q 'parallel' "$AI_ROOT/commands/start-work.md"; then
  log_pass "start-work.md supports sequential/parallel modes"
else
  log_fail "start-work.md missing execution modes"
fi

if grep -q 'Multi-Repo' "$AI_ROOT/commands/start-work.md"; then
  log_pass "start-work.md has multi-repo documentation"
else
  log_fail "start-work.md missing multi-repo documentation"
fi

# ============================================================
# Test 12: Cross-Platform Python Scripts
# ============================================================
echo ""
echo "## Cross-Platform Tests"

# Test scan_repo.py exists and runs
if [[ -f "$AI_ROOT/scripts/scan_repo.py" ]]; then
  log_pass "scan_repo.py exists"
  
  if python3 "$AI_ROOT/scripts/scan_repo.py" --json > /dev/null 2>&1; then
    log_pass "scan_repo.py runs successfully"
  else
    log_fail "scan_repo.py failed to run"
  fi
else
  log_fail "scan_repo.py missing"
fi

# Test audit_project.py exists and runs
if [[ -f "$AI_ROOT/scripts/audit_project.py" ]]; then
  log_pass "audit_project.py exists"
  
  if python3 "$AI_ROOT/scripts/audit_project.py" --json > /dev/null 2>&1; then
    log_pass "audit_project.py runs successfully"
  else
    log_fail "audit_project.py failed to run"
  fi
else
  log_fail "audit_project.py missing"
fi

# ============================================================
# Summary
# ============================================================
echo ""
echo "============================================"
echo "Results: $PASSED passed, $FAILED failed, $SKIPPED skipped"
echo "============================================"

if [[ $FAILED -gt 0 ]]; then
  exit 1
fi
exit 0
