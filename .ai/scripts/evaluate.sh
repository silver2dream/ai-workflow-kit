#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# evaluate.sh - AI Workflow Kit Evaluation Script
# ============================================================================
# Usage:
#   bash .ai/scripts/evaluate.sh              # Offline mode (default)
#   bash .ai/scripts/evaluate.sh --offline    # Offline mode
#   bash .ai/scripts/evaluate.sh --online     # Online mode (requires gh auth)
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

MODE="${1:---offline}"

echo "=========================================="
echo "AI Workflow Kit - Evaluation v3.1"
echo "Mode: $MODE"
echo "=========================================="
echo ""

# === Offline Gate ===
echo "## Offline Gate"
OFFLINE_PASS=true

check_offline() {
  local id="$1" cmd="$2" desc="$3"
  if eval "$cmd" > /dev/null 2>&1; then
    echo "[PASS] $id: $desc"
  else
    echo "[FAIL] $id: $desc"
    OFFLINE_PASS=false
  fi
}

# O1: scan_repo executable
check_offline "O1" "bash $AI_ROOT/scripts/scan_repo.sh 2>/dev/null || python3 $AI_ROOT/scripts/scan_repo.py" "scan_repo executable"

# O2: repo_scan.json valid
check_offline "O2" "python3 -m json.tool $AI_ROOT/state/repo_scan.json" "repo_scan.json valid"

# O3: audit_project executable
check_offline "O3" "bash $AI_ROOT/scripts/audit_project.sh 2>/dev/null || python3 $AI_ROOT/scripts/audit_project.py" "audit_project executable"

# O4: audit.json valid
check_offline "O4" "python3 -m json.tool $AI_ROOT/state/audit.json" "audit.json valid"

# O5: validate_config passes
check_offline "O5" "python3 $AI_ROOT/scripts/validate_config.py" "validate_config passes"

# O6: No CRLF/UTF-16 in shell scripts (skip if 'file' command not available)
if command -v file > /dev/null 2>&1; then
  check_offline "O6" "! file $AI_ROOT/scripts/*.sh 2>/dev/null | grep -qE 'CRLF|UTF-16'" "no CRLF/UTF-16 in scripts"
else
  echo "[SKIP] O6: no CRLF/UTF-16 in scripts (file command not available)"
fi

# O7: Main files not UTF-16 (skip if 'file' command not available)
if command -v file > /dev/null 2>&1; then
  check_offline "O7" "! file $MONO_ROOT/README.md $MONO_ROOT/CLAUDE.md $MONO_ROOT/AGENTS.md 2>/dev/null | grep -qE 'UTF-16'" "main files not UTF-16"
else
  echo "[SKIP] O7: main files not UTF-16 (file command not available)"
fi

# O8: Test suite passes
check_offline "O8" "bash $AI_ROOT/tests/run_all_tests.sh" "test suite passes"

echo ""
if [ "$OFFLINE_PASS" = false ]; then
  echo "----------------------------------------"
  echo "RESULT: Offline Gate FAILED"
  echo "Score cap: 4.0 (F)"
  echo "----------------------------------------"
  exit 1
fi
echo "----------------------------------------"
echo "RESULT: Offline Gate PASSED"
echo "----------------------------------------"


# === Online Gate (if requested) ===
SCORE_CAP=8.5
if [ "$MODE" = "--online" ]; then
  echo ""
  echo "## Online Gate"

  # Prerequisites check
  if ! command -v gh > /dev/null 2>&1; then
    echo "[SKIP] gh CLI not installed"
    echo "Score cap: 8.5 (B)"
  elif ! gh auth status > /dev/null 2>&1; then
    echo "[SKIP] gh not authenticated"
    echo "Score cap: 8.5 (B)"
  elif ! curl -s --max-time 5 https://api.github.com > /dev/null 2>&1; then
    echo "[SKIP] Cannot connect to GitHub"
    echo "Score cap: 8.5 (B)"
  else
    ONLINE_PASS=true

    check_online() {
      local id="$1" cmd="$2" desc="$3"
      if eval "$cmd" > /dev/null 2>&1; then
        echo "[PASS] $id: $desc"
      else
        echo "[FAIL] $id: $desc"
        ONLINE_PASS=false
      fi
    }

    # N1: kickoff --dry-run
    check_online "N1" "bash $AI_ROOT/scripts/kickoff.sh --dry-run" "kickoff --dry-run"

    # N2: rollback --dry-run
    check_online "N2" "bash $AI_ROOT/scripts/rollback.sh 99999 --dry-run 2>&1 | grep -qiE 'not found|usage|dry|error'" "rollback --dry-run"

    # N3: stats --json
    check_online "N3" "bash $AI_ROOT/scripts/stats.sh --json | python3 -m json.tool" "stats --json"

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
