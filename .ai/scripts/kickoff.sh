#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# kickoff.sh - AI Workflow Launcher
# ============================================================================
# Usage:
#   bash .ai/scripts/kickoff.sh              # Start full workflow
#   bash .ai/scripts/kickoff.sh --dry-run    # Pre-flight check only
#   bash .ai/scripts/kickoff.sh --background # Background execution (nohup)
# ============================================================================

MONO_ROOT="$(git rev-parse --show-toplevel)"
cd "$MONO_ROOT"

DRY_RUN=false
BACKGROUND=false
FORCE=false
CHECK_UPDATE="${AWKIT_CHECK_UPDATE:-false}"

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --background) BACKGROUND=true ;;
    --force) FORCE=true ;;
    --check-update) CHECK_UPDATE=true ;;
    --help|-h)
      echo "Usage: bash .ai/scripts/kickoff.sh [--dry-run] [--background] [--force] [--check-update]"
      echo ""
      echo "Options:"
      echo "  --dry-run     Pre-flight check only, don't start workflow"
      echo "  --background  Background execution (using nohup)"
      echo "  --force       Auto-delete STOP marker without asking (for autonomous mode)"
      echo "  --check-update  Check for awkit updates (best-effort)"
      exit 0
      ;;
  esac
done

# ----------------------------------------------------------------------------
# Output helpers (plain text for Windows compatibility)
# ----------------------------------------------------------------------------
info()  { echo "[INFO] $1"; }
ok()    { echo "[OK] $1"; }
warn()  { echo "[WARN] $1"; }
error() { echo "[ERROR] $1"; }

# ----------------------------------------------------------------------------
# run_script - Cross-platform script executor
# ----------------------------------------------------------------------------
# Auto-selects .py or .sh version, prefers Python (cross-platform)
# Usage: run_script <script_name> [args...]
# Example: run_script scan_repo --json
# ----------------------------------------------------------------------------
run_script() {
  local script_name="$1"
  shift
  local sh_path="$MONO_ROOT/.ai/scripts/${script_name}.sh"
  local py_path="$MONO_ROOT/.ai/scripts/${script_name}.py"
  
  # Prefer Python (cross-platform)
  if command -v python3 &>/dev/null && [[ -f "$py_path" ]]; then
    python3 "$py_path" "$@"
  elif command -v python &>/dev/null && [[ -f "$py_path" ]]; then
    python "$py_path" "$@"
  elif [[ -f "$sh_path" ]]; then
    bash "$sh_path" "$@"
  else
    error "Script not found: $script_name (.py or .sh)"
    return 1
  fi
}

# ----------------------------------------------------------------------------
# Pre-flight checks
# ----------------------------------------------------------------------------
info "Running pre-flight checks..."

# 1. Check gh CLI
if ! command -v gh &>/dev/null; then
  error "gh CLI not installed. Run: brew install gh (macOS) or see https://cli.github.com/"
  exit 1
fi
ok "gh CLI installed"

# 2. Check gh auth
if ! gh auth status &>/dev/null; then
  error "gh not authenticated. Use one of:"
  echo "  1. Interactive: gh auth login"
  echo "  2. Environment: export GH_TOKEN=ghp_xxxx"
  echo "  3. CI/CD: Set GITHUB_TOKEN secret"
  exit 1
fi
ok "gh authenticated"

# 3. Check claude CLI
if ! command -v claude &>/dev/null; then
  error "claude CLI not installed. Ensure Claude Code Pro is installed and in PATH."
  exit 1
fi
ok "claude CLI installed"

# 4. Check codex CLI
if ! command -v codex &>/dev/null; then
  warn "codex CLI not installed. Worker execution will fail."
else
  ok "codex CLI installed"
fi

# 4.1 Optional: check for awkit updates
if [[ "$CHECK_UPDATE" == "true" ]]; then
  if command -v awkit &>/dev/null; then
    info "Checking for awkit updates..."
    awkit check-update --quiet || true
  else
    warn "awkit not found; update check skipped"
  fi
fi

# 5. Check working directory is clean
if [[ -n "$(git status --porcelain)" ]]; then
  error "Working directory not clean. Please commit or stash changes."
  git status --short
  exit 1
fi
ok "Working directory clean"

# 6. Check stop marker
if [[ -f ".ai/state/STOP" ]]; then
  if [[ "$FORCE" == "true" ]]; then
    warn "Found stop marker .ai/state/STOP, auto-deleting (--force mode)"
    rm -f ".ai/state/STOP"
    ok "Deleted stop marker"
  else
    error "Found stop marker .ai/state/STOP"
    error "Use --force to auto-delete, or manually delete and retry"
    exit 1
  fi
else
  ok "No stop marker"
fi

# 7. Run project audit
info "Running project audit..."
run_script scan_repo >/dev/null
run_script audit_project >/dev/null

AUDIT_FILE="$MONO_ROOT/.ai/state/audit.json"
if [[ -f "$AUDIT_FILE" ]]; then
  P0_COUNT=$(python3 -c "import json; print(json.load(open('$AUDIT_FILE'))['summary']['p0'])" 2>/dev/null || echo "0")
  P1_COUNT=$(python3 -c "import json; print(json.load(open('$AUDIT_FILE'))['summary']['p1'])" 2>/dev/null || echo "0")
  
  if [[ "$P0_COUNT" -gt 0 ]]; then
    error "Found $P0_COUNT P0 issues, must fix first!"
    python3 -c "
import json
audit = json.load(open('$AUDIT_FILE'))
for f in audit['findings']:
    if f['severity'] == 'P0':
        print(f\"  - [{f['severity']}] {f['title']}\")
"
    exit 1
  fi
  
  if [[ "$P1_COUNT" -gt 0 ]]; then
    warn "Found $P1_COUNT P1 issues, recommend fixing"
  fi
fi
ok "Project audit complete"

# ----------------------------------------------------------------------------
# Dry Run mode
# ----------------------------------------------------------------------------
if [[ "$DRY_RUN" == "true" ]]; then
  echo ""
  info "=== Dry Run Complete ==="
  info "All pre-flight checks passed, ready to start workflow."
  info "Run 'bash .ai/scripts/kickoff.sh' to start."
  exit 0
fi

# ----------------------------------------------------------------------------
# Prepare to start
# ----------------------------------------------------------------------------
mkdir -p "$MONO_ROOT/.ai/state" "$MONO_ROOT/.ai/results" "$MONO_ROOT/.ai/runs" "$MONO_ROOT/.ai/exe-logs"

TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "$TIMESTAMP" > "$MONO_ROOT/.ai/state/kickoff_time.txt"

info "Preparing to start AI workflow..."
echo ""
echo "============================================================"
echo "                    AI Automation Workflow                   "
echo "============================================================"
echo "  Start time: $TIMESTAMP"
echo "  Working dir: $MONO_ROOT"
echo ""
echo "  Stop methods:"
echo "    1. touch .ai/state/STOP"
echo "    2. Say 'stop' in Claude Code"
echo "    3. Ctrl+C"
echo ""
echo "  Check progress:"
echo "    gh issue list --label ai-task"
echo "    gh pr list"
echo "============================================================"
echo ""

# ----------------------------------------------------------------------------
# Start Claude Code
# ----------------------------------------------------------------------------

BOOT_PROMPT="$MONO_ROOT/.ai/scripts/principal_boot.txt"

if [[ "$BACKGROUND" == "true" ]]; then
  info "Starting in background mode..."
  LOG_FILE="$MONO_ROOT/.ai/exe-logs/kickoff-$(date +%Y%m%d-%H%M%S).log"

  nohup bash -c "
    cd '$MONO_ROOT'
    if [[ -f '$BOOT_PROMPT' ]]; then
      claude --print < '$BOOT_PROMPT' >> '$LOG_FILE' 2>&1
    else
      echo '/start-work --autonomous' | claude --print >> '$LOG_FILE' 2>&1
    fi
  " > /dev/null 2>&1 &

  CLAUDE_PID=$!
  echo "$CLAUDE_PID" > "$MONO_ROOT/.ai/state/claude_pid.txt"

  ok "Claude Code started in background (PID: $CLAUDE_PID)"
  info "Log file: $LOG_FILE"
  info "Stop: kill $CLAUDE_PID or touch .ai/state/STOP"
else
  info "Starting in foreground mode (autonomous)..."
  info "Claude Code will run autonomously, executing /start-work --autonomous"
  echo ""

  # Use principal_boot.txt if exists, otherwise pass /start-work --autonomous directly
  # Note: --print flag avoids raw mode issues in WSL/non-TTY environments
  # Use stdbuf to disable output buffering for real-time progress
  if [[ -f "$BOOT_PROMPT" ]]; then
    info "Using principal_boot.txt as boot prompt"
    if command -v stdbuf &>/dev/null; then
      stdbuf -oL -eL claude --print < "$BOOT_PROMPT"
    else
      claude --print < "$BOOT_PROMPT"
    fi
  else
    # Auto-execute /start-work in autonomous mode
    if command -v stdbuf &>/dev/null; then
      echo "/start-work --autonomous" | stdbuf -oL -eL claude --print
    else
      echo "/start-work --autonomous" | claude --print
    fi
  fi
fi
