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
# Test 13: Path Reference Tests (v3)
# ============================================================
echo ""
echo "## Path Reference Tests (v3)"

# Test no old path references exist
OLD_PATH_MATCHES=$(grep -r "scripts/ai/" "$AI_ROOT/commands" "$MONO_ROOT/docs" 2>/dev/null | grep -v ".ai/scripts" || true)
if [[ -z "$OLD_PATH_MATCHES" ]]; then
  log_pass "No old 'scripts/ai/' references found"
else
  log_fail "Found old 'scripts/ai/' references"
fi

# Test review-pr.md uses correct git-workflow path
if grep -q "_kit/git-workflow.md" "$AI_ROOT/commands/review-pr.md"; then
  log_pass "review-pr.md uses correct git-workflow path"
else
  log_fail "review-pr.md has wrong git-workflow path"
fi

# Test review-pr.md doesn't hardcode feat/aether
if ! grep -q "feat/aether" "$AI_ROOT/commands/review-pr.md"; then
  log_pass "review-pr.md doesn't hardcode branch name"
else
  log_fail "review-pr.md still hardcodes feat/aether"
fi

# ============================================================
# Test 14: cleanup.sh Branch Patterns (v3)
# ============================================================
echo ""
echo "## cleanup.sh Branch Pattern Tests"

# Test cleanup.sh uses feat/ai-issue-* pattern for remote branches
if grep -q "origin/feat/ai-issue-" "$AI_ROOT/scripts/cleanup.sh"; then
  log_pass "cleanup.sh uses feat/ai-issue-* for remote branches"
else
  log_fail "cleanup.sh should use feat/ai-issue-* pattern"
fi

# Test cleanup.sh uses feat/ai-issue-* pattern for local branches
if grep -q "'feat/ai-issue-\*'" "$AI_ROOT/scripts/cleanup.sh"; then
  log_pass "cleanup.sh uses feat/ai-issue-* for local branches"
else
  log_fail "cleanup.sh should use feat/ai-issue-* for local branches"
fi

# ============================================================
# Test 15: Script Executability (v3)
# ============================================================
echo ""
echo "## Script Executability Tests"

# Test scan_repo.py actually executes
if python3 "$AI_ROOT/scripts/scan_repo.py" --json > /dev/null 2>&1; then
  log_pass "scan_repo.py executes successfully"
else
  log_fail "scan_repo.py failed to execute"
fi

# Test audit_project.py actually executes
if python3 "$AI_ROOT/scripts/audit_project.py" --json > /dev/null 2>&1; then
  log_pass "audit_project.py executes successfully"
else
  log_fail "audit_project.py failed to execute"
fi

# Test kickoff.sh --help (safe to run)
if bash "$AI_ROOT/scripts/kickoff.sh" --help > /dev/null 2>&1; then
  log_pass "kickoff.sh --help executes"
else
  log_fail "kickoff.sh --help failed"
fi

# Test no CRLF in shell scripts (would break on Unix)
CRLF_FILES=""
for script in "$AI_ROOT/scripts"/*.sh; do
  if [[ -f "$script" ]] && file "$script" 2>/dev/null | grep -q "CRLF"; then
    CRLF_FILES="$CRLF_FILES $(basename "$script")"
  fi
done
if [[ -z "$CRLF_FILES" ]]; then
  log_pass "No CRLF line endings in shell scripts"
else
  log_fail "CRLF found in:$CRLF_FILES"
fi

# ============================================================
# Test 16: validate_config.py Dependency Handling (v3)
# ============================================================
echo ""
echo "## validate_config.py Dependency Tests"

# Test validate_config.py doesn't auto-install
if ! grep -q "pip3 install.*--quiet" "$AI_ROOT/scripts/validate_config.py" && \
   ! grep -q "os.system.*pip" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py doesn't auto-install dependencies"
else
  log_fail "validate_config.py still auto-installs dependencies"
fi

# Test validate_config.py shows helpful error message
if grep -q "Please install" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py shows install instructions"
else
  log_fail "validate_config.py missing install instructions"
fi

# ============================================================
# Test 17: workflow.yaml Consistency (v3)
# ============================================================
echo ""
echo "## workflow.yaml Consistency Tests"

# Test repos use 'directory' type (not submodule)
if python3 -c "
import yaml
config = yaml.safe_load(open('$AI_ROOT/config/workflow.yaml'))
for repo in config.get('repos', []):
    if repo.get('type') == 'submodule':
        exit(1)
exit(0)
" 2>/dev/null; then
  log_pass "workflow.yaml repos use directory type"
else
  log_fail "workflow.yaml still has submodule type"
fi

# Test validate-submodules.yml doesn't exist (no submodules)
if [[ ! -f "$MONO_ROOT/.github/workflows/validate-submodules.yml" ]]; then
  log_pass "No validate-submodules.yml (no submodules)"
else
  log_fail "validate-submodules.yml exists but no submodules"
fi

# ============================================================
# Test 18: run_script() Cross-Platform Tests (v3)
# ============================================================
echo ""
echo "## run_script() Cross-Platform Tests"

# Test run_script function exists in kickoff.sh
if grep -q "run_script()" "$AI_ROOT/scripts/kickoff.sh"; then
  log_pass "kickoff.sh has run_script() function"
else
  log_fail "kickoff.sh missing run_script() function"
fi

# Test run_script prefers Python
if grep -q "python3.*py_path" "$AI_ROOT/scripts/kickoff.sh" && grep -q "python.*py_path" "$AI_ROOT/scripts/kickoff.sh"; then
  log_pass "run_script() prefers Python over bash"
else
  log_fail "run_script() should prefer Python"
fi

# Test kickoff.sh uses run_script for audit
if grep -q "run_script scan_repo" "$AI_ROOT/scripts/kickoff.sh" && grep -q "run_script audit_project" "$AI_ROOT/scripts/kickoff.sh"; then
  log_pass "kickoff.sh uses run_script for audit"
else
  log_fail "kickoff.sh should use run_script for audit"
fi

# ============================================================
# Test 19: Python Scripts Write State Files (v3.1)
# ============================================================
echo ""
echo "## Python State File Tests (v3.1)"

# Clean up old state files first
rm -f "$AI_ROOT/state/repo_scan.json" "$AI_ROOT/state/audit.json" 2>/dev/null || true

# Test scan_repo.py writes state file
if python3 "$AI_ROOT/scripts/scan_repo.py" > /dev/null 2>&1 && [[ -f "$AI_ROOT/state/repo_scan.json" ]]; then
  log_pass "scan_repo.py writes repo_scan.json"
else
  log_fail "scan_repo.py should write repo_scan.json"
fi

# Test audit_project.py writes state file
if python3 "$AI_ROOT/scripts/audit_project.py" > /dev/null 2>&1 && [[ -f "$AI_ROOT/state/audit.json" ]]; then
  log_pass "audit_project.py writes audit.json"
else
  log_fail "audit_project.py should write audit.json"
fi

# Test state files are valid JSON
if python3 -m json.tool "$AI_ROOT/state/repo_scan.json" > /dev/null 2>&1; then
  log_pass "repo_scan.json is valid JSON"
else
  log_fail "repo_scan.json is not valid JSON"
fi

if python3 -m json.tool "$AI_ROOT/state/audit.json" > /dev/null 2>&1; then
  log_pass "audit.json is valid JSON"
else
  log_fail "audit.json is not valid JSON"
fi

# ============================================================
# Test 20: evaluate.sh Exists (v3.1)
# ============================================================
echo ""
echo "## evaluate.sh Tests (v3.1)"

if [[ -f "$AI_ROOT/scripts/evaluate.sh" ]]; then
  log_pass "evaluate.sh exists"
else
  log_fail "evaluate.sh missing"
fi

# Test evaluate.sh has offline gate
if grep -q "Offline Gate" "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh has Offline Gate"
else
  log_fail "evaluate.sh missing Offline Gate"
fi

# Test evaluate.sh has online gate
if grep -q "Online Gate" "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh has Online Gate"
else
  log_fail "evaluate.sh missing Online Gate"
fi

# Test evaluate.sh checks rollback
if grep -q "rollback" "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh checks rollback"
else
  log_fail "evaluate.sh missing rollback check"
fi

# ============================================================
# Test 21: evaluate.sh v4.0 Features
# ============================================================
echo ""
echo "## evaluate.sh v4.0 Tests"

# Test O0: git check-ignore
if grep -q "git check-ignore" "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh uses git check-ignore for O0"
else
  log_fail "evaluate.sh missing git check-ignore"
fi

# Test O6: Python YAML parsing
if grep -q "import yaml, glob, fnmatch" "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh uses Python for O6 CI check"
else
  log_fail "evaluate.sh missing Python CI check"
fi

# Test O7: version sync check
if grep -q "version mismatch" "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh checks version sync (O7)"
else
  log_fail "evaluate.sh missing version sync check"
fi

# Test N2: pipefail avoidance
if grep -q 'ROLLBACK_OUTPUT=.*|| true' "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh avoids pipefail for N2"
else
  log_fail "evaluate.sh may have pipefail issue in N2"
fi

# Test prerequisites check
if grep -q "import yaml" "$AI_ROOT/scripts/evaluate.sh" && grep -q "import jsonschema" "$AI_ROOT/scripts/evaluate.sh"; then
  log_pass "evaluate.sh checks Python dependencies"
else
  log_fail "evaluate.sh missing dependency checks"
fi

# ============================================================
# Test 22: validate_config.py Type-Specific Validation (v3.1)
# ============================================================
echo ""
echo "## Type-Specific Validation Tests (v3.1)"

# Test validate_config.py has type-specific validation
if grep -q "repo_type == 'submodule'" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py checks submodule type"
else
  log_fail "validate_config.py missing submodule type check"
fi

if grep -q "repo_type == 'directory'" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py checks directory type"
else
  log_fail "validate_config.py missing directory type check"
fi

if grep -q "repo_type == 'root'" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py checks root type"
else
  log_fail "validate_config.py missing root type check"
fi

# Test validate_config.py checks .gitmodules for submodule type
if grep -q "gitmodules" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py checks .gitmodules for submodules"
else
  log_fail "validate_config.py missing .gitmodules check"
fi

# Test validate_config.py warns about directory with .git
if grep -q "has .git" "$AI_ROOT/scripts/validate_config.py" || grep -q "consider type=submodule" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py warns about directory with .git"
else
  log_fail "validate_config.py missing directory .git warning"
fi

# Test validate_config.py validates root path
if grep -q "should be './'" "$AI_ROOT/scripts/validate_config.py" || grep -q "should be './' or empty" "$AI_ROOT/scripts/validate_config.py"; then
  log_pass "validate_config.py validates root path"
else
  log_fail "validate_config.py missing root path validation"
fi

# ============================================================
# Test 23: Schema Validation (v5.1)
# ============================================================
echo ""
echo "## Schema Validation Tests (v5.1)"

# Test repo_scan.schema.json exists
if [[ -f "$AI_ROOT/config/repo_scan.schema.json" ]]; then
  log_pass "repo_scan.schema.json exists"
else
  log_fail "repo_scan.schema.json missing"
fi

# Test audit.schema.json exists
if [[ -f "$AI_ROOT/config/audit.schema.json" ]]; then
  log_pass "audit.schema.json exists"
else
  log_fail "audit.schema.json missing"
fi

# Test repo_scan.json conforms to schema
if [[ -f "$AI_ROOT/state/repo_scan.json" ]] && [[ -f "$AI_ROOT/config/repo_scan.schema.json" ]]; then
  if python3 -c "
import json, jsonschema
with open('$AI_ROOT/config/repo_scan.schema.json') as f:
    schema = json.load(f)
with open('$AI_ROOT/state/repo_scan.json') as f:
    data = json.load(f)
jsonschema.validate(data, schema)
" 2>/dev/null; then
    log_pass "repo_scan.json conforms to schema"
  else
    log_fail "repo_scan.json does not conform to schema"
  fi
else
  log_skip "repo_scan.json schema validation (files missing)"
fi

# Test audit.json conforms to schema
if [[ -f "$AI_ROOT/state/audit.json" ]] && [[ -f "$AI_ROOT/config/audit.schema.json" ]]; then
  if python3 -c "
import json, jsonschema
with open('$AI_ROOT/config/audit.schema.json') as f:
    schema = json.load(f)
with open('$AI_ROOT/state/audit.json') as f:
    data = json.load(f)
jsonschema.validate(data, schema)
" 2>/dev/null; then
    log_pass "audit.json conforms to schema"
  else
    log_fail "audit.json does not conform to schema"
  fi
else
  log_skip "audit.json schema validation (files missing)"
fi

# Test dirty_worktree is P1 (not P0)
if [[ -f "$AI_ROOT/state/audit.json" ]]; then
  DIRTY_SEVERITY=$(python3 -c "
import json
with open('$AI_ROOT/state/audit.json') as f:
    audit = json.load(f)
found = False
for finding in audit.get('findings', []):
    finding_type = finding.get('type') or finding.get('id')
    if finding_type in ('dirty_worktree', 'dirty-worktree'):
        print(finding.get('severity', 'unknown'))
        found = True
        break
if not found:
    print('none')
" 2>/dev/null || echo "none")
  if [[ "$DIRTY_SEVERITY" == "P1" ]] || [[ "$DIRTY_SEVERITY" == "none" ]]; then
    log_pass "dirty_worktree severity is P1 (or not present)"
  else
    log_fail "dirty_worktree severity is $DIRTY_SEVERITY (should be P1)"
  fi
else
  log_skip "dirty_worktree severity check (audit.json missing)"
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
