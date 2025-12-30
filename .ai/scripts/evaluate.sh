#!/usr/bin/env bash
# AI Workflow Kit - Evaluation v5.2
# 注意：不使用 set -o pipefail，避免 N2 誤判
set -eu

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"
cd "$MONO_ROOT"

# Timeout helpers
source "$SCRIPT_DIR/lib/timeout.sh"

# === 參數解析 ===
MODE="--offline"
STRICT=false
CHECK_ORIGIN=false
while [[ $# -gt 0 ]]; do
  case $1 in
    --strict) STRICT=true; shift ;;
    --online) MODE="--online"; shift ;;
    --offline) MODE="--offline"; shift ;;
    --check-origin) CHECK_ORIGIN=true; shift ;;
    *) shift ;;
  esac
done

echo "=========================================="
echo "AI Workflow Kit - Evaluation v5.2"
echo "Mode: $MODE$([ "$STRICT" = true ] && echo " --strict" || echo "")$([ "$CHECK_ORIGIN" = true ] && echo " --check-origin" || echo "")"
echo "=========================================="
echo ""

OFFLINE_PASS=true
OFFLINE_SKIP_COUNT=0

# === 輔助函數 ===
check_pass() { echo "[PASS] $1: $2"; }
check_fail() { echo "[FAIL] $1: $2"; OFFLINE_PASS=false; }
check_skip() { echo "[SKIP] $1: $2"; OFFLINE_SKIP_COUNT=$((OFFLINE_SKIP_COUNT + 1)); }

# === 前置條件檢查 ===
echo "## Prerequisites"
if ! command -v python3 > /dev/null 2>&1; then
  echo "ERROR: python3 not found"
  exit 1
fi
if ! python3 -c "import yaml" 2>/dev/null; then
  echo "ERROR: pyyaml not installed (pip3 install pyyaml)"
  exit 1
fi
if ! python3 -c "import jsonschema" 2>/dev/null; then
  echo "ERROR: jsonschema not installed (pip3 install jsonschema)"
  exit 1
fi
echo "[OK] Prerequisites satisfied"
echo ""

# === 記錄初始 git status ===
GIT_STATUS_BEFORE=$(git status --porcelain 2>/dev/null || echo "")

# === Offline Gate ===
echo "## Offline Gate"

# O0: 無副作用檢查 (git check-ignore)
O0_PASS=true
for DIR in ".ai/state" ".ai/results" ".ai/runs" ".ai/exe-logs" ".worktrees"; do
  TEST_FILE="$DIR/.gitignore-test-$$"
  mkdir -p "$DIR" 2>/dev/null || true
  touch "$TEST_FILE" 2>/dev/null || true
  if ! git check-ignore -q "$TEST_FILE" 2>/dev/null; then
    echo "[FAIL] O0: $DIR not ignored by git"
    O0_PASS=false
    OFFLINE_PASS=false
  fi
  rm -f "$TEST_FILE" 2>/dev/null || true
done
if [ "$O0_PASS" = true ]; then
  check_pass "O0" "state dirs ignored by git"
fi

# O1+O2: scan_repo
if bash "$AI_ROOT/scripts/scan_repo.sh" > /dev/null 2>&1; then
  if python3 -m json.tool "$AI_ROOT/state/repo_scan.json" > /dev/null 2>&1; then
    check_pass "O1+O2" "scan_repo (.sh mode)"
  else
    check_fail "O1+O2" "repo_scan.json invalid"
  fi
elif python3 "$AI_ROOT/scripts/scan_repo.py" --json 2>/dev/null | python3 -m json.tool > /dev/null 2>&1; then
  check_pass "O1+O2" "scan_repo (.py mode)"
else
  check_fail "O1+O2" "scan_repo failed"
fi

# O3+O4: audit_project
if bash "$AI_ROOT/scripts/audit_project.sh" > /dev/null 2>&1; then
  if python3 -m json.tool "$AI_ROOT/state/audit.json" > /dev/null 2>&1; then
    check_pass "O3+O4" "audit_project (.sh mode)"
  else
    check_fail "O3+O4" "audit.json invalid"
  fi
elif python3 "$AI_ROOT/scripts/audit_project.py" --json 2>/dev/null | python3 -m json.tool > /dev/null 2>&1; then
  check_pass "O3+O4" "audit_project (.py mode)"
else
  check_fail "O3+O4" "audit_project failed"
fi

# O4.1: --strict 模式檢查 audit P0
if [ "$STRICT" = true ]; then
  P0_COUNT=$(python3 -c "
import json, sys
try:
    with open('.ai/state/audit.json') as f:
        audit = json.load(f)
    p0 = [f for f in audit.get('findings', []) if f.get('severity') == 'P0']
    print(len(p0))
except:
    print('0')
" 2>/dev/null || echo "0")
  
  if [ "$P0_COUNT" -gt 0 ]; then
    check_fail "O4.1" "audit has $P0_COUNT P0 findings (--strict mode)"
  else
    check_pass "O4.1" "no P0 findings in audit (--strict mode)"
  fi
fi

# O5: validate_config (awkit validate)
if awkit validate > /dev/null 2>&1; then
  check_pass "O5" "validate_config (awkit validate)"
else
  check_fail "O5" "validate_config failed (awkit validate)"
fi

# O7: 版本同步 (P0 強制)
DOC_VER=$(grep -oE 'v[0-9]+\.[0-9]+' "$AI_ROOT/docs/evaluate.md" 2>/dev/null | head -1 || echo "")
SCRIPT_VER=$(grep -oE 'v[0-9]+\.[0-9]+' "$AI_ROOT/scripts/evaluate.sh" 2>/dev/null | head -1 || echo "")
if [ -z "$SCRIPT_VER" ]; then
  check_fail "O7" "evaluate.sh not found or no version"
elif [ "$DOC_VER" != "$SCRIPT_VER" ]; then
  check_fail "O7" "version mismatch: doc=$DOC_VER script=$SCRIPT_VER"
else
  check_pass "O7" "version sync ($DOC_VER)"
fi

# O8: CRLF/UTF-16 in scripts
if ! command -v file > /dev/null 2>&1; then
  check_skip "O8" "file command not found (optional dependency)"
elif file "$AI_ROOT/scripts"/*.sh 2>/dev/null | grep -qE 'CRLF|UTF-16'; then
  check_fail "O8" "scripts have CRLF or UTF-16"
else
  check_pass "O8" "scripts encoding OK"
fi

# O9: UTF-16 in docs
if ! command -v file > /dev/null 2>&1; then
  check_skip "O9" "file command not found (optional dependency)"
elif file README.md CLAUDE.md AGENTS.md 2>/dev/null | grep -qE 'UTF-16'; then
  check_fail "O9" "docs have UTF-16"
else
  check_pass "O9" "docs encoding OK"
fi

# O10: tests
TEST_LOG="$(mktemp -t awk-tests.XXXXXX)"
if bash "$AI_ROOT/tests/run_all_tests.sh" >"$TEST_LOG" 2>&1; then
  check_pass "O10" "tests passed"
else
  echo ""
  echo "---- .ai/tests/run_all_tests.sh output (last 200 lines) ----"
  tail -n 200 "$TEST_LOG" || true
  echo "---- end ----"
  check_fail "O10" "tests failed"
fi
rm -f "$TEST_LOG" 2>/dev/null || true

# === 驗證 git status 未變 ===
GIT_STATUS_AFTER=$(git status --porcelain 2>/dev/null || echo "")
if [ "$GIT_STATUS_BEFORE" != "$GIT_STATUS_AFTER" ]; then
  echo ""
  echo "Warning: git status changed during evaluation"
fi

echo ""
if [ "$OFFLINE_PASS" = false ]; then
  echo "----------------------------------------"
  echo "RESULT: Offline Gate FAILED"
  echo "Score cap: 4.0 (F)"
  echo "----------------------------------------"
  exit 1
fi
echo "----------------------------------------"
echo "RESULT: Offline Gate PASSED (SKIP: $OFFLINE_SKIP_COUNT)"
echo "----------------------------------------"

# === Origin Checks (需要網路，可選) ===
if [ "$CHECK_ORIGIN" = true ]; then
  echo ""
  echo "## Origin Checks (--check-origin)"
  
  # 檢查 submodule pinned sha 是否存在於 origin
  # SKIP 僅允許：無 submodules
  # FAIL：網路錯誤、sha 不存在、Python 錯誤
  SUBMODULE_CHECK=$(python3 -c "
import json, subprocess, os, sys
try:
    with open('.ai/state/repo_scan.json') as f:
        scan = json.load(f)
    root_path = scan.get('root', {}).get('path', '.')
    if isinstance(scan.get('root'), str):
        root_path = scan.get('root')
    submodules = scan.get('submodules', [])
    if not submodules:
        print('SKIP:no submodules')
        sys.exit(0)
    
    # 先測試網路連線
    rc = subprocess.call(
        ['git', 'ls-remote', '--exit-code', 'origin', 'HEAD'],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        timeout=10
    )
    if rc != 0:
        print('FAIL:network error - cannot reach origin')
        sys.exit(0)
    
    failed = []
    for sm in submodules:
        if not sm.get('exists'):
            continue
        path = sm.get('path')
        head = sm.get('head')
        if not head:
            continue
        full_path = os.path.join(root_path, path)
        rc = subprocess.call(
            ['git', '-C', full_path, 'fetch', '-q', 'origin', head, '--depth=1'],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            timeout=30
        )
        if rc != 0:
            failed.append(path)
    if failed:
        print('FAIL:sha not found - ' + ','.join(failed))
    else:
        print('PASS:all submodule shas found on origin')
except subprocess.TimeoutExpired:
    print('FAIL:network timeout')
except Exception as e:
    print('FAIL:' + str(e))
" 2>&1)

  # Python 執行失敗也是 FAIL（不允許 SKIP）
  if [ -z "$SUBMODULE_CHECK" ]; then
    SUBMODULE_CHECK="FAIL:python execution error"
  fi

  case "$SUBMODULE_CHECK" in
    PASS:*)
      echo "[PASS] ORIGIN1: ${SUBMODULE_CHECK#PASS:}"
      ;;
    FAIL:*)
      echo "[FAIL] ORIGIN1: ${SUBMODULE_CHECK#FAIL:}"
      ;;
    SKIP:*)
      echo "[SKIP] ORIGIN1: ${SUBMODULE_CHECK#SKIP:}"
      ;;
    *)
      echo "[FAIL] ORIGIN1: unexpected - $SUBMODULE_CHECK"
      ;;
  esac
fi

# === Extensibility Checks (不影響 Offline Gate) ===
echo ""
echo "## Extensibility Checks"

# EXT1 (原 O6): CI/分支對齊 (P1, 不影響 Gate)
# SKIP 白名單：僅「無 .github/workflows 目錄」允許 SKIP
if [ ! -d .github/workflows ]; then
  echo "[SKIP] EXT1: CI/branch alignment (no .github/workflows - not applicable)"
else
  WORKFLOW_COUNT=$(find .github/workflows -name "*.yml" -o -name "*.yaml" 2>/dev/null | wc -l)
  if [ "$WORKFLOW_COUNT" -eq 0 ]; then
    echo "[FAIL] EXT1: .github/workflows exists but empty (should have workflows or remove directory)"
  else
    EXT1_RESULT=$(python3 -c "
import yaml, glob, fnmatch, sys
try:
    with open('.ai/config/workflow.yaml') as f:
        int_branch = yaml.safe_load(f)['git']['integration_branch']
except Exception as e:
    # 配置錯誤 = FAIL，不是 SKIP
    print('FAIL:cannot read integration_branch - config error')
    sys.exit(0)

def check_triggers(int_branch):
    wf_files = glob.glob('.github/workflows/*.yml') + glob.glob('.github/workflows/*.yaml')
    for wf_file in wf_files:
        try:
            with open(wf_file) as f:
                wf = yaml.safe_load(f)
            if not wf:
                continue
            # PyYAML 1.1 會把 'on' 解析成 True (布林值)，需要處理兩種情況
            on_config = wf.get('on') or wf.get(True)
            if not on_config:
                continue
            if isinstance(on_config, list):
                return True
            if isinstance(on_config, dict):
                for event in ['push', 'pull_request']:
                    if event not in on_config:
                        continue
                    event_config = on_config[event]
                    if event_config is None:
                        return True
                    if isinstance(event_config, dict):
                        branches = event_config.get('branches', [])
                        if not branches:
                            return True
                        for pattern in branches:
                            if fnmatch.fnmatch(int_branch, pattern) or pattern in ['*', '**']:
                                return True
        except Exception as e:
            # Python 執行錯誤 = FAIL，不是 SKIP
            print('FAIL:error parsing ' + wf_file)
            sys.exit(0)
    return False

if check_triggers(int_branch):
    print('PASS:' + int_branch)
else:
    print('FAIL:CI does not trigger on ' + int_branch)
" 2>&1)

    # 檢查 Python 是否執行失敗
    if [ $? -ne 0 ] && [ -z "$EXT1_RESULT" ]; then
      echo "[FAIL] EXT1: Python execution error"
    else
      case "$EXT1_RESULT" in
        PASS:*)
          echo "[PASS] EXT1: CI triggers on ${EXT1_RESULT#PASS:}"
          ;;
        FAIL:*)
          echo "[FAIL] EXT1: ${EXT1_RESULT#FAIL:}"
          ;;
        *)
          echo "[FAIL] EXT1: unexpected result - $EXT1_RESULT"
          ;;
      esac
    fi
  fi
fi

# === Online Gate ===
SCORE_CAP=8.5
if [ "$MODE" = "--online" ]; then
  echo ""
  echo "## Online Gate"

  # 前置條件檢查
  if ! command -v gh > /dev/null 2>&1; then
    echo "[SKIP] gh CLI not installed"
    echo "Score cap: 8.5 (B)"
  elif ! gh_with_timeout auth status > /dev/null 2>&1; then
    echo "[SKIP] gh not authenticated"
    echo "Score cap: 8.5 (B)"
  elif ! command -v curl > /dev/null 2>&1; then
    echo "[SKIP] curl not installed"
    echo "Score cap: 8.5 (B)"
  elif ! curl -s --max-time 5 https://api.github.com > /dev/null 2>&1; then
    echo "[SKIP] cannot reach GitHub API"
    echo "Score cap: 8.5 (B)"
  else
    ONLINE_PASS=true

    # N1: kickoff (awkit kickoff)
    if awkit kickoff --dry-run > /dev/null 2>&1; then
      check_pass "N1" "kickoff --dry-run (awkit)"
    else
      echo "[FAIL] N1: kickoff --dry-run (awkit)"
      ONLINE_PASS=false
    fi

    # N2: rollback (特殊處理：捕獲輸出，不依賴 exit code)
    ROLLBACK_OUTPUT=$(bash "$AI_ROOT/scripts/rollback.sh" 99999 --dry-run 2>&1 || true)
    if echo "$ROLLBACK_OUTPUT" | grep -qiE 'not found|usage|dry|PR.*not|ERROR'; then
      check_pass "N2" "rollback --dry-run"
    else
      echo "[FAIL] N2: rollback --dry-run"
      ONLINE_PASS=false
    fi

    # N3: status (awkit status)
    if awkit status --json 2>/dev/null | python3 -m json.tool > /dev/null 2>&1; then
      check_pass "N3" "status --json (awkit)"
    else
      echo "[FAIL] N3: status --json (awkit)"
      ONLINE_PASS=false
    fi

    echo ""
    if [ "$ONLINE_PASS" = true ]; then
      echo "----------------------------------------"
      echo "RESULT: Online Gate PASSED"
      echo "Score cap: 10.0 (A)"
      echo "----------------------------------------"
      SCORE_CAP=10.0
    else
      echo "----------------------------------------"
      echo "RESULT: Online Gate FAILED"
      echo "Score cap: 8.5 (B)"
      echo "----------------------------------------"
    fi
  fi
else
  echo ""
  echo "## Online Gate"
  echo "[SKIP] Use --online to run Online Gate"
  echo "Score cap: 8.5 (B)"
fi

echo ""
echo "=========================================="
echo "Final Score Cap: $SCORE_CAP"
echo "=========================================="
