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

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log_pass() { echo -e "${GREEN}✓${NC} $1"; PASSED=$((PASSED + 1)); }
log_fail() { echo -e "${RED}✗${NC} $1"; FAILED=$((FAILED + 1)); }
log_skip() { echo -e "${YELLOW}○${NC} $1 (skipped)"; SKIPPED=$((SKIPPED + 1)); }
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
# Summary
# ============================================================
echo ""
echo "============================================"
echo "Results: ${GREEN}$PASSED passed${NC}, ${RED}$FAILED failed${NC}, ${YELLOW}$SKIPPED skipped${NC}"
echo "============================================"

if [[ $FAILED -gt 0 ]]; then
  exit 1
fi
exit 0
