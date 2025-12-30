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
# Test 1: Config validation (awkit validate)
# ============================================================
echo "## Config Tests"

if awkit validate > /dev/null 2>&1; then
  log_pass "workflow.yaml is valid (awkit validate)"
else
  log_fail "workflow.yaml validation failed (awkit validate)"
fi

# ============================================================
# Test 2: Required files exist
# ============================================================
echo ""
echo "## File Structure Tests"

# Test awkit binary is available
if command -v awkit &>/dev/null; then
  log_pass "awkit binary available"
else
  log_fail "awkit binary not found in PATH"
fi

required_files=(
  "$AI_ROOT/config/workflow.yaml"
  "$AI_ROOT/config/workflow.schema.json"
  "$AI_ROOT/scripts/audit_project.sh"
  "$AI_ROOT/scripts/scan_repo.sh"
  "$AI_ROOT/skills/principal-workflow/SKILL.md"
)

for file in "${required_files[@]}"; do
  if [[ -f "$file" ]]; then
    log_pass "$(basename "$file") exists"
  else
    log_fail "$(basename "$file") missing: $file"
  fi
done

# ============================================================
# Test 3: awkit generate produces valid output
# ============================================================
echo ""
echo "## Generate Tests"

# Create temp dir for test output
TEST_TMP=$(mktemp -d)
trap "rm -rf $TEST_TMP" EXIT

# Test awkit generate --dry-run works
if awkit generate --dry-run > /dev/null 2>&1; then
  log_pass "awkit generate --dry-run works"
else
  log_fail "awkit generate --dry-run failed"
fi

# Check generated files exist (from previous generation)
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
# Test 4: Scripts are executable (Unix only) + awkit commands
# ============================================================
echo ""
echo "## Script Tests"

if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "win32" ]]; then
  log_skip "Executable check (Windows)"
else
  # Only check scripts that are still in use (not migrated to Go)
  scripts=(
    "$AI_ROOT/scripts/audit_project.sh"
    "$AI_ROOT/scripts/scan_repo.sh"
  )
  for script in "${scripts[@]}"; do
    if [[ -x "$script" ]]; then
      log_pass "$(basename "$script") is executable"
    else
      log_fail "$(basename "$script") not executable"
    fi
  done
fi

# Test awkit subcommands respond to --help
for cmd in validate generate kickoff status analyze-next dispatch-worker; do
  if awkit $cmd --help > /dev/null 2>&1; then
    log_pass "awkit $cmd --help works"
  else
    log_fail "awkit $cmd --help failed"
  fi
done

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
# Test 7: Generator Output Verification (replaced Jinja2 tests)
# ============================================================
echo ""
echo "## Generator Output Tests"

# Test that generated files have required content structure
if [[ -f "$MONO_ROOT/CLAUDE.md" ]]; then
  # Check CLAUDE.md has required sections
  if grep -q "## Role" "$MONO_ROOT/CLAUDE.md" && \
     grep -q "## Principal Workflow" "$MONO_ROOT/CLAUDE.md"; then
    log_pass "CLAUDE.md has required sections"
  else
    log_fail "CLAUDE.md missing required sections"
  fi
else
  log_skip "CLAUDE.md not found (run awkit generate first)"
fi

if [[ -f "$MONO_ROOT/AGENTS.md" ]]; then
  # Check AGENTS.md has required sections
  if grep -q "## Role" "$MONO_ROOT/AGENTS.md" && \
     grep -q "## MUST-READ" "$MONO_ROOT/AGENTS.md"; then
    log_pass "AGENTS.md has required sections"
  else
    log_fail "AGENTS.md missing required sections"
  fi
else
  log_skip "AGENTS.md not found (run awkit generate first)"
fi

# Check git-workflow.md in _kit rules
if [[ -f "$AI_ROOT/rules/_kit/git-workflow.md" ]]; then
  if grep -qi "Branching" "$AI_ROOT/rules/_kit/git-workflow.md" && \
     grep -qi "Commit" "$AI_ROOT/rules/_kit/git-workflow.md"; then
    log_pass "git-workflow.md has required sections"
  else
    log_fail "git-workflow.md missing required sections"
  fi
else
  log_skip "git-workflow.md not found"
fi

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
# Test 9: Status Command Tests (awkit status)
# ============================================================
echo ""
echo "## Status Command Tests"

# Test awkit status --help
if awkit status --help > /dev/null 2>&1; then
  log_pass "awkit status --help works"
else
  log_fail "awkit status --help failed"
fi

# Test awkit status --json produces valid JSON (needs gh auth)
if command -v gh &>/dev/null && gh auth status &>/dev/null 2>&1; then
  STATUS_OUTPUT=$(awkit status --json 2>/dev/null || echo "{}")
  if echo "$STATUS_OUTPUT" | python3 -m json.tool > /dev/null 2>&1; then
    log_pass "awkit status --json produces valid JSON"
  else
    log_fail "awkit status --json output is not valid JSON"
  fi
else
  log_skip "awkit status --json (gh not authenticated)"
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
# Test 11: Multi-Repo Coordination (awkit dispatch-worker)
# ============================================================
echo ""
echo "## Multi-Repo Tests"

# Test awkit dispatch-worker is available
if awkit dispatch-worker --help > /dev/null 2>&1; then
  log_pass "awkit dispatch-worker available"
else
  log_fail "awkit dispatch-worker not available"
fi

# Test skills structure exists
if [[ -f "$AI_ROOT/skills/principal-workflow/SKILL.md" ]]; then
  log_pass "principal-workflow skill exists"
else
  log_fail "principal-workflow skill missing"
fi

if [[ -f "$AI_ROOT/skills/principal-workflow/phases/main-loop.md" ]]; then
  log_pass "main-loop.md exists"
else
  log_fail "main-loop.md missing"
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
OLD_PATH_MATCHES=$(grep -r "scripts/ai/" "$MONO_ROOT/docs" 2>/dev/null | grep -v ".ai/scripts" || true)
if [[ -z "$OLD_PATH_MATCHES" ]]; then
  log_pass "No old 'scripts/ai/' references found"
else
  log_fail "Found old 'scripts/ai/' references"
fi

# Test review-pr.md uses awkit review commands (new architecture)
if grep -q "awkit prepare-review" "$AI_ROOT/skills/principal-workflow/tasks/review-pr.md" && \
   grep -q "awkit submit-review" "$AI_ROOT/skills/principal-workflow/tasks/review-pr.md"; then
  log_pass "review-pr.md uses awkit review commands"
else
  log_fail "review-pr.md missing awkit review commands"
fi

# Test review-pr.md doesn't hardcode feat/aether
if ! grep -q "feat/aether" "$AI_ROOT/skills/principal-workflow/tasks/review-pr.md"; then
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

# Test awkit kickoff --help (safe to run)
if awkit kickoff --help > /dev/null 2>&1; then
  log_pass "awkit kickoff --help executes"
else
  log_fail "awkit kickoff --help failed"
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
# Test 16: Config Validation (awkit validate)
# ============================================================
echo ""
echo "## Config Validation Tests"

# Test awkit validate succeeds on current config
if awkit validate > /dev/null 2>&1; then
  log_pass "awkit validate succeeds on current config"
else
  log_fail "awkit validate failed on current config"
fi

# Test awkit validate --help works
if awkit validate --help > /dev/null 2>&1; then
  log_pass "awkit validate --help works"
else
  log_fail "awkit validate --help failed"
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
# Test 18: Go Cross-Platform Tests (replaced run_script tests)
# ============================================================
echo ""
echo "## Go Cross-Platform Tests"

# Go handles cross-platform natively - verify Go tests pass
if command -v go &>/dev/null; then
  # Run Go tests (they include cross-platform compatibility)
  if go test ./... > /dev/null 2>&1; then
    log_pass "Go tests pass (includes cross-platform support)"
  else
    log_fail "Go tests failed"
  fi
else
  log_skip "go not installed"
fi

# Test awkit works on current platform
if awkit --help > /dev/null 2>&1; then
  log_pass "awkit runs on current platform"
else
  log_fail "awkit failed to run"
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
# Test 22: awkit validate Type-Specific Validation
# ============================================================
echo ""
echo "## Type-Specific Validation Tests (awkit)"

# Test awkit validate works on current config
if awkit validate > /dev/null 2>&1; then
  log_pass "awkit validate works on current config"
else
  log_fail "awkit validate failed on current config"
fi

# Test awkit validate provides helpful output
VALIDATE_OUTPUT=$(awkit validate 2>&1)
if echo "$VALIDATE_OUTPUT" | grep -qi "valid\|ok\|success"; then
  log_pass "awkit validate provides helpful output"
else
  log_fail "awkit validate output not helpful"
fi

# Test awkit validate shows project info
if echo "$VALIDATE_OUTPUT" | grep -qi "project\|type\|repos"; then
  log_pass "awkit validate shows project info"
else
  log_fail "awkit validate missing project info"
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
# Test 24: Python Unit Tests (pytest)
# ============================================================
echo ""
echo "## Python Unit Tests (pytest)"

PYTEST_AVAILABLE=false
if command -v pytest &>/dev/null; then
  PYTEST_AVAILABLE=true
elif python3 -m pytest --version &>/dev/null 2>&1; then
  PYTEST_AVAILABLE=true
fi

if [[ "$PYTEST_AVAILABLE" == "true" ]]; then
  PYTEST_OUTPUT=$(python3 -m pytest "$AI_ROOT/tests/unit" -v --tb=short 2>&1)
  PYTEST_EXIT=$?

  if [[ $PYTEST_EXIT -eq 0 ]]; then
    log_pass "Python unit tests passed"
  elif [[ $PYTEST_EXIT -eq 5 ]]; then
    # Exit code 5 means no tests collected
    log_skip "No pytest tests found"
  else
    log_fail "Python unit tests failed"
    if [[ "$VERBOSE" == "--verbose" ]]; then
      echo "$PYTEST_OUTPUT"
    fi
  fi
else
  log_skip "pytest not installed (pip3 install pytest)"
fi

# ============================================================
# Test 25: Go Tests
# ============================================================
echo ""
echo "## Go Tests"

if command -v go &>/dev/null; then
  GO_OUTPUT=$(go test ./... 2>&1)
  GO_EXIT=$?

  if [[ $GO_EXIT -eq 0 ]]; then
    log_pass "Go tests passed"
  else
    log_fail "Go tests failed"
    if [[ "$VERBOSE" == "--verbose" ]]; then
      echo "$GO_OUTPUT"
    fi
  fi
else
  log_skip "go not installed"
fi

# ============================================================
# Test 26: Shell Workflow Integration Tests
# ============================================================
echo ""
echo "## Shell Workflow Integration Tests"

# Test analyze_failure.sh
if [[ -f "$SCRIPT_DIR/test_analyze_failure.sh" ]]; then
  if bash "$SCRIPT_DIR/test_analyze_failure.sh" > /dev/null 2>&1; then
    log_pass "test_analyze_failure.sh passed"
  else
    log_fail "test_analyze_failure.sh failed"
  fi
else
  log_skip "test_analyze_failure.sh not found"
fi

# Test root workflow
if [[ -f "$SCRIPT_DIR/test_root_workflow.sh" ]]; then
  if bash "$SCRIPT_DIR/test_root_workflow.sh" > /dev/null 2>&1; then
    log_pass "test_root_workflow.sh passed"
  else
    log_fail "test_root_workflow.sh failed"
  fi
else
  log_skip "test_root_workflow.sh not found"
fi

# Test directory workflow
if [[ -f "$SCRIPT_DIR/test_directory_workflow.sh" ]]; then
  if bash "$SCRIPT_DIR/test_directory_workflow.sh" > /dev/null 2>&1; then
    log_pass "test_directory_workflow.sh passed"
  else
    log_fail "test_directory_workflow.sh failed"
  fi
else
  log_skip "test_directory_workflow.sh not found"
fi

# Test submodule workflow
if [[ -f "$SCRIPT_DIR/test_submodule_workflow.sh" ]]; then
  if bash "$SCRIPT_DIR/test_submodule_workflow.sh" > /dev/null 2>&1; then
    log_pass "test_submodule_workflow.sh passed"
  else
    log_fail "test_submodule_workflow.sh failed"
  fi
else
  log_skip "test_submodule_workflow.sh not found"
fi

# Test review scripts (prepare_review.sh, submit_review.sh)
if [[ -f "$SCRIPT_DIR/test_review_scripts.sh" ]]; then
  if bash "$SCRIPT_DIR/test_review_scripts.sh" > /dev/null 2>&1; then
    log_pass "test_review_scripts.sh passed"
  else
    log_fail "test_review_scripts.sh failed"
  fi
else
  log_skip "test_review_scripts.sh not found"
fi

# ============================================================
# Test 27: Session Audit Tracking Tests
# ============================================================
echo ""
echo "## Session Audit Tracking Tests (Properties 1-15)"

# Test session_manager.sh (Property 1, 2, 15)
if [[ -f "$SCRIPT_DIR/test_session_manager.sh" ]]; then
  if bash "$SCRIPT_DIR/test_session_manager.sh" > /dev/null 2>&1; then
    log_pass "test_session_manager.sh passed (Property 1, 2, 15)"
  else
    log_fail "test_session_manager.sh failed"
  fi
else
  log_skip "test_session_manager.sh not found"
fi

# Test github_comment.sh (Property 3, 10, 11)
if [[ -f "$SCRIPT_DIR/test_github_comment.sh" ]]; then
  if bash "$SCRIPT_DIR/test_github_comment.sh" > /dev/null 2>&1; then
    log_pass "test_github_comment.sh passed (Property 3, 10, 11)"
  else
    log_fail "test_github_comment.sh failed"
  fi
else
  log_skip "test_github_comment.sh not found"
fi

# Test worker_session.sh (Property 4, 6)
if [[ -f "$SCRIPT_DIR/test_worker_session.sh" ]]; then
  if bash "$SCRIPT_DIR/test_worker_session.sh" > /dev/null 2>&1; then
    log_pass "test_worker_session.sh passed (Property 4, 6)"
  else
    log_fail "test_worker_session.sh failed"
  fi
else
  log_skip "test_worker_session.sh not found"
fi

# Test principal_session.sh (Property 7)
if [[ -f "$SCRIPT_DIR/test_principal_session.sh" ]]; then
  if bash "$SCRIPT_DIR/test_principal_session.sh" > /dev/null 2>&1; then
    log_pass "test_principal_session.sh passed (Property 7)"
  else
    log_fail "test_principal_session.sh failed"
  fi
else
  log_skip "test_principal_session.sh not found"
fi

# Test verify_review.sh (Property 12)
if [[ -f "$SCRIPT_DIR/test_verify_review.sh" ]]; then
  if bash "$SCRIPT_DIR/test_verify_review.sh" > /dev/null 2>&1; then
    log_pass "test_verify_review.sh passed (Property 12)"
  else
    log_fail "test_verify_review.sh failed"
  fi
else
  log_skip "test_verify_review.sh not found"
fi

# Test review_flow.sh (Property 13, 14)
if [[ -f "$SCRIPT_DIR/test_review_flow.sh" ]]; then
  if bash "$SCRIPT_DIR/test_review_flow.sh" > /dev/null 2>&1; then
    log_pass "test_review_flow.sh passed (Property 13, 14)"
  else
    log_fail "test_review_flow.sh failed"
  fi
else
  log_skip "test_review_flow.sh not found"
fi

# Test audit_merged_prs.sh (Property 5, 9)
if [[ -f "$SCRIPT_DIR/test_audit_merged_prs.sh" ]]; then
  if bash "$SCRIPT_DIR/test_audit_merged_prs.sh" > /dev/null 2>&1; then
    log_pass "test_audit_merged_prs.sh passed (Property 5, 9)"
  else
    log_fail "test_audit_merged_prs.sh failed"
  fi
else
  log_skip "test_audit_merged_prs.sh not found"
fi

# Test worker_prompt_isolation.sh (Property 8)
if [[ -f "$SCRIPT_DIR/test_worker_prompt_isolation.sh" ]]; then
  if bash "$SCRIPT_DIR/test_worker_prompt_isolation.sh" > /dev/null 2>&1; then
    log_pass "test_worker_prompt_isolation.sh passed (Property 8)"
  else
    log_fail "test_worker_prompt_isolation.sh failed"
  fi
else
  log_skip "test_worker_prompt_isolation.sh not found"
fi

# ============================================================
# Test 28: awkit Commands Integration Tests
# ============================================================
echo ""
echo "## awkit Commands Integration Tests"

# Test all awkit subcommands respond to --help
AWKIT_COMMANDS=(
  "validate"
  "generate"
  "kickoff"
  "status"
  "analyze-next"
  "dispatch-worker"
  "check-result"
  "prepare-review"
  "submit-review"
  "create-task"
)

for cmd in "${AWKIT_COMMANDS[@]}"; do
  if awkit "$cmd" --help > /dev/null 2>&1; then
    log_pass "awkit $cmd --help"
  else
    log_fail "awkit $cmd --help failed"
  fi
done

# Test awkit version (if implemented)
if awkit version > /dev/null 2>&1 || awkit --version > /dev/null 2>&1; then
  log_pass "awkit version available"
else
  log_skip "awkit version not implemented"
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
