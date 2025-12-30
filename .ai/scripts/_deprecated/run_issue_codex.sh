#!/usr/bin/env bash
set -euo pipefail

ISSUE_ID="${1:?usage: run_issue_codex.sh <issue_id> <task_file> [root|backend|frontend]}"
TASK_FILE="${2:?usage: run_issue_codex.sh <issue_id> <task_file> [root|backend|frontend]}"
REPO_ARG="${3:-}"  # optional override

MONO_ROOT="$(git rev-parse --show-toplevel)"
AI_ROOT="$MONO_ROOT/.ai"
CONFIG_FILE="$AI_ROOT/config/workflow.yaml"

# ============================================================
# Source Session and Comment Managers (Req 2.1, 4.2, 4.3)
# ============================================================
source "$AI_ROOT/scripts/session_manager.sh"
source "$AI_ROOT/scripts/github_comment.sh"
source "$AI_ROOT/scripts/lib/timeout.sh"

# ============================================================
# Generate Worker Session ID (Req 2.1)
# ============================================================
WORKER_SESSION_ID=$(generate_session_id "worker")
export WORKER_SESSION_ID

# ============================================================
# Worker Log Function (writes to log file, not stdout/stderr)
# ============================================================
WORKER_LOG_FILE="$AI_ROOT/exe-logs/issue-${ISSUE_ID}.worker.log"
export WORKER_LOG_FILE
mkdir -p "$(dirname "$WORKER_LOG_FILE")"

worker_log() {
  local msg="[WORKER] $(date +%H:%M:%S) | $*"
  printf '%s\n' "$msg" >> "$WORKER_LOG_FILE" 2>/dev/null || true
}

# Also log with tee to summary file
worker_log_tee() {
  local msg="[WORKER] $(date +%H:%M:%S) | $*"
  printf '%s\n' "$msg" >> "$WORKER_LOG_FILE" 2>/dev/null || true
  printf '%s\n' "$msg" >> "$SUMMARY_FILE" 2>/dev/null || true
}

worker_log "worker_session_id=$WORKER_SESSION_ID"

# ============================================================
# Repo Type Detection Functions
# ============================================================
# Get repo type from workflow.yaml
# Returns: root | directory | submodule (default: directory)
get_repo_type() {
  local repo_name="$1"
  local config_file="$2"
  
  if [[ ! -f "$config_file" ]]; then
    echo "directory"
    return
  fi
  
  python3 -c "
import yaml
import sys

repo_name = '$repo_name'
config_file = '$config_file'

try:
    with open(config_file) as f:
        config = yaml.safe_load(f)
    
    for repo in config.get('repos', []):
        if repo.get('name') == repo_name:
            print(repo.get('type', 'directory'))
            sys.exit(0)
    
    # Not found in config, default to directory
    print('directory')
except Exception as e:
    print('directory', file=sys.stderr)
    print('directory')
" 2>/dev/null || echo "directory"
}

# Get repo path from workflow.yaml
get_repo_path() {
  local repo_name="$1"
  local config_file="$2"
  
  if [[ ! -f "$config_file" ]]; then
    echo "./"
    return
  fi
  
  python3 -c "
import yaml
import sys

repo_name = '$repo_name'
config_file = '$config_file'

try:
    with open(config_file) as f:
        config = yaml.safe_load(f)
    
    for repo in config.get('repos', []):
        if repo.get('name') == repo_name:
            print(repo.get('path', './'))
            sys.exit(0)
    
    # Not found, default to ./
    print('./')
except Exception:
    print('./')
" 2>/dev/null || echo "./"
}

# ============================================================
# Submodule Git Operations Functions (Req 6.1-6.5, 20.1-20.4, 24.1-24.5)
# ============================================================

# Global variables for submodule tracking
SUBMODULE_SHA=""
PARENT_SHA=""
CONSISTENCY_STATUS="consistent"

# Check if all changes are within submodule boundary (Req 20.1-20.4)
check_submodule_boundary() {
  local wt_dir="$1"
  local submodule_path="$2"
  local allow_parent="${3:-false}"
  
  # Get list of changed files
  local changed_files
  changed_files="$(git -C "$wt_dir" diff --cached --name-only 2>/dev/null || true)"
  
  if [[ -z "$changed_files" ]]; then
    return 0
  fi
  
  local outside_files=""
  while IFS= read -r file; do
    # Check if file is within submodule path
    if [[ ! "$file" =~ ^"$submodule_path"/ && "$file" != "$submodule_path" ]]; then
      # File is outside submodule
      outside_files="$outside_files$file"$'\n'
    fi
  done <<< "$changed_files"
  
  if [[ -n "$outside_files" ]]; then
    if [[ "$allow_parent" == "true" ]]; then
      worker_log "WARNING: Changes outside submodule boundary (allowed by config):"
      echo "$outside_files" >&2
      return 0
    else
      echo "ERROR: Changes detected outside submodule '$submodule_path':" >&2
      echo "$outside_files" >&2
      echo "HINT: Set allow_parent_changes=true in ticket to allow parent changes." >&2
      return 1
    fi
  fi
  
  return 0
}

# Commit changes in submodule first, then update parent reference (Req 6.1, 6.2)
git_commit_submodule() {
  local wt_dir="$1"
  local submodule_path="$2"
  local commit_msg="$3"
  local submodule_dir="$wt_dir/$submodule_path"
  
  worker_log "submodule commit: $submodule_path"
  
  # Stage and commit in submodule first (Req 6.1)
  git -C "$submodule_dir" add -A
  
  if git -C "$submodule_dir" diff --cached --quiet; then
    worker_log "no changes in submodule"
    return 1
  fi
  
  if ! git -C "$submodule_dir" commit -m "$commit_msg"; then
    echo "ERROR: submodule commit failed" >&2
    return 2
  fi
  
  # Record submodule SHA (Req 6.2)
  SUBMODULE_SHA="$(git -C "$submodule_dir" rev-parse HEAD)"
  worker_log "submodule SHA: $SUBMODULE_SHA"
  
  # Update parent's submodule reference (Req 6.2)
  git -C "$wt_dir" add "$submodule_path"
  
  if ! git -C "$wt_dir" commit -m "$commit_msg"; then
    echo "ERROR: parent commit (submodule ref update) failed" >&2
    CONSISTENCY_STATUS="submodule_committed_parent_failed"
    return 2
  fi
  
  # Record parent SHA
  PARENT_SHA="$(git -C "$wt_dir" rev-parse HEAD)"
  worker_log "parent SHA: $PARENT_SHA"
  
  return 0
}

# Push submodule first, then parent (Req 6.3, 6.4, 6.5, 24.1-24.3)
git_push_submodule() {
  local wt_dir="$1"
  local submodule_path="$2"
  local branch="$3"
  local submodule_dir="$wt_dir/$submodule_path"
  
  worker_log "submodule push: $submodule_path"
  
  # Get submodule's default branch for push
  local submodule_branch
  submodule_branch="$(git -C "$submodule_dir" symbolic-ref --short HEAD 2>/dev/null || echo "$branch")"
  
  # Create branch in submodule if needed
  if ! git -C "$submodule_dir" show-ref --verify --quiet "refs/heads/$submodule_branch"; then
    git -C "$submodule_dir" checkout -b "$submodule_branch" >&2
  fi
  
  # Push submodule first (Req 6.3)
  if ! git_with_timeout -C "$submodule_dir" push -u origin "$submodule_branch"; then
    echo "ERROR: submodule push failed" >&2
    CONSISTENCY_STATUS="submodule_push_failed"
    return 2
  fi
  
  worker_log "submodule pushed to origin/$submodule_branch"
  
  # Push parent (Req 6.4)
  if ! git_with_timeout -C "$wt_dir" push -u origin "$branch"; then
    echo "ERROR: parent push failed (submodule already pushed)" >&2
    CONSISTENCY_STATUS="parent_push_failed_submodule_pushed"
    echo "RECOVERY: git -C '$submodule_dir' reset --hard HEAD~1 && git push -f origin $submodule_branch" >&2
    return 2
  fi
  
  worker_log "parent pushed to origin/$branch"
  CONSISTENCY_STATUS="consistent"
  
  return 0
}

# ============================================================
# Security Functions (Req 25.1-25.5, 29.1-29.5)
# ============================================================

# Check for script modifications (Req 25.1-25.5)
check_script_modifications() {
  local wt_dir="$1"
  local allow_script_changes="${2:-false}"
  local whitelist="${3:-}"
  
  # Get list of changed files
  local changed_files
  changed_files="$(git -C "$wt_dir" diff --cached --name-only 2>/dev/null || true)"
  
  if [[ -z "$changed_files" ]]; then
    return 0
  fi
  
  local protected_paths=".ai/scripts/ .ai/commands/"
  local violations=""
  
  while IFS= read -r file; do
    for protected in $protected_paths; do
      if [[ "$file" == "$protected"* ]]; then
        # Check if file is in whitelist
        if [[ -n "$whitelist" && "$whitelist" == *"$file"* ]]; then
          continue
        fi
        violations="$violations$file"$'\n'
      fi
    done
  done <<< "$changed_files"
  
  if [[ -n "$violations" ]]; then
    if [[ "$allow_script_changes" == "true" ]]; then
      worker_log "WARNING: Script modifications detected (allowed by approval flag):"
      echo "$violations" >&2
      return 0
    else
      echo "ERROR: Modifications to protected scripts detected:" >&2
      echo "$violations" >&2
      echo "HINT: Set allow_script_changes=true in ticket to allow script modifications." >&2
      return 1
    fi
  fi
  
  return 0
}

# Check for sensitive information in changes (Req 29.1-29.5)
check_sensitive_info() {
  local wt_dir="$1"
  local allow_secrets="${2:-false}"
  local custom_patterns="${3:-}"
  
  # Default secret patterns
  local patterns=(
    'password\s*[:=]\s*["\x27][^"\x27]+'
    'api[_-]?key\s*[:=]\s*["\x27][^"\x27]+'
    'secret[_-]?key\s*[:=]\s*["\x27][^"\x27]+'
    'access[_-]?token\s*[:=]\s*["\x27][^"\x27]+'
    'private[_-]?key\s*[:=]'
    'AWS_SECRET_ACCESS_KEY'
    'GITHUB_TOKEN'
    'BEGIN\s+(RSA|DSA|EC|OPENSSH)\s+PRIVATE\s+KEY'
  )
  
  # Get diff content
  local diff_content
  diff_content="$(git -C "$wt_dir" diff --cached 2>/dev/null || true)"
  
  if [[ -z "$diff_content" ]]; then
    return 0
  fi
  
  local found_secrets=""
  for pattern in "${patterns[@]}"; do
    if echo "$diff_content" | grep -iE "$pattern" >/dev/null 2>&1; then
      found_secrets="$found_secrets- Pattern matched: $pattern"$'\n'
    fi
  done
  
  # Check custom patterns if provided
  if [[ -n "$custom_patterns" ]]; then
    for pattern in $custom_patterns; do
      if echo "$diff_content" | grep -E "$pattern" >/dev/null 2>&1; then
        found_secrets="$found_secrets- Custom pattern matched: $pattern"$'\n'
      fi
    done
  fi
  
  if [[ -n "$found_secrets" ]]; then
    if [[ "$allow_secrets" == "true" ]]; then
      worker_log "WARNING: Potential sensitive information detected (allowed by flag):"
      echo "$found_secrets" >&2
      return 0
    else
      echo "ERROR: Potential sensitive information detected in changes:" >&2
      echo "$found_secrets" >&2
      echo "HINT: Set allow_secrets=true in ticket if this is intentional." >&2
      return 1
    fi
  fi
  
  return 0
}

# ============================================================
# Error Handling Functions (Req 13.1-13.5)
# ============================================================

# Format detailed error message with context
format_error() {
  local operation="$1"
  local error_msg="$2"
  local suggestion="${3:-}"
  
  echo "============================================================" >&2
  echo "ERROR: $error_msg" >&2
  echo "============================================================" >&2
  echo "Operation: $operation" >&2
  echo "Repo Type: ${REPO_TYPE:-unknown}" >&2
  echo "Repo Path: ${REPO_PATH:-unknown}" >&2
  echo "Worktree: ${WT_DIR:-unknown}" >&2
  echo "Work Dir: ${WORK_DIR:-unknown}" >&2
  echo "Branch: ${BRANCH:-unknown}" >&2
  if [[ -n "$suggestion" ]]; then
    echo "" >&2
    echo "SUGGESTION: $suggestion" >&2
  fi
  echo "============================================================" >&2
}

# ============================================================
# Cross-Platform Functions (Req 27.1-27.4, 28.1-28.4)
# ============================================================

# Normalize path for cross-platform comparison (Req 27.1-27.4)
normalize_path() {
  local path="$1"
  # Remove trailing slashes, convert backslashes to forward slashes
  path="${path//\\//}"
  path="${path%/}"
  # Convert to lowercase for case-insensitive comparison on Windows
  if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
    path="$(echo "$path" | tr '[:upper:]' '[:lower:]')"
  fi
  echo "$path"
}

# Compare paths case-insensitively on Windows (Req 27.2)
paths_equal() {
  local path1="$1"
  local path2="$2"
  local norm1 norm2
  norm1="$(normalize_path "$path1")"
  norm2="$(normalize_path "$path2")"
  [[ "$norm1" == "$norm2" ]]
}

# Cache directory for push permission results (Req 28.4)
PUSH_PERMISSION_CACHE_FILE="$MONO_ROOT/.ai/state/cache/push_permissions.json"

# Check push permission with caching (Req 28.1-28.4)
check_push_permission() {
  local remote_url="$1"
  local cache_key="push:$remote_url"
  local cache_ttl=300  # 5 minutes
  
  mkdir -p "$(dirname "$PUSH_PERMISSION_CACHE_FILE")"
  
  # Check cache first (Req 28.4)
  if [[ -f "$PUSH_PERMISSION_CACHE_FILE" ]]; then
    local cached_result
    cached_result=$(python3 -c "
import json
import time
import sys

cache_file = '$PUSH_PERMISSION_CACHE_FILE'
key = '$cache_key'
ttl = $cache_ttl

try:
    with open(cache_file) as f:
        cache = json.load(f)
    
    if key in cache:
        entry = cache[key]
        if time.time() - entry.get('timestamp', 0) <= ttl:
            print('allowed' if entry.get('allowed', False) else 'denied')
            sys.exit(0)
    print('unknown')
except Exception:
    print('unknown')
" 2>/dev/null || echo "unknown")
    
    if [[ "$cached_result" == "allowed" ]]; then
      return 0
    elif [[ "$cached_result" == "denied" ]]; then
      return 1
    fi
  fi
  
  # Try a dry-run push to check permission (Req 28.1, 28.2)
  local allowed="false"
  if git_with_timeout push --dry-run "$remote_url" HEAD:refs/heads/__permission_check__ 2>/dev/null; then
    allowed="true"
  fi
  
  # Update cache (Req 28.3)
  python3 -c "
import json
import time

cache_file = '$PUSH_PERMISSION_CACHE_FILE'
key = '$cache_key'
allowed = '$allowed' == 'true'

try:
    with open(cache_file) as f:
        cache = json.load(f)
except (FileNotFoundError, json.JSONDecodeError):
    cache = {}

cache[key] = {
    'allowed': allowed,
    'timestamp': time.time()
}

with open(cache_file, 'w') as f:
    json.dump(cache, f, indent=2)
" 2>/dev/null || true
  
  [[ "$allowed" == "true" ]]
}

# ============================================================
# Submodule Branch Management Functions (Req 16.1, 16.2, 16.4)
# ============================================================

# Create or reuse branch in submodule (Req 16.1, 16.2, 16.4)
setup_submodule_branch() {
  local submodule_dir="$1"
  local branch_name="$2"
  
  worker_log "setup submodule branch: $branch_name"
  
  # Get submodule's default branch (Req 16.2)
  local default_branch
  default_branch="$(git -C "$submodule_dir" symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@' || echo "main")"
  
  # Check if branch already exists (Req 16.4)
  if git -C "$submodule_dir" show-ref --verify --quiet "refs/heads/$branch_name"; then
    worker_log "reusing existing submodule branch: $branch_name"
    git -C "$submodule_dir" checkout "$branch_name" >&2
    return 0
  fi
  
  # Check if branch exists on remote
  if git -C "$submodule_dir" show-ref --verify --quiet "refs/remotes/origin/$branch_name"; then
    worker_log "checking out remote submodule branch: $branch_name"
    git -C "$submodule_dir" checkout -b "$branch_name" "origin/$branch_name" >&2
    return 0
  fi
  
  # Create new branch from default branch (Req 16.1)
  worker_log "creating new submodule branch from $default_branch"
  git -C "$submodule_dir" checkout -b "$branch_name" "origin/$default_branch" >&2 || \
    git -C "$submodule_dir" checkout -b "$branch_name" >&2
  
  return 0
}

# Read config
if [[ -f "$CONFIG_FILE" ]]; then
  INTEGRATION_BRANCH=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c['git']['integration_branch'])")
  RELEASE_BRANCH=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c['git']['release_branch'])")
  RETRY_COUNT=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')) or {}; print(c.get('escalation', {}).get('retry_count', 2))")
  RETRY_DELAY=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')) or {}; print(c.get('escalation', {}).get('retry_delay_seconds', 5))")
else
  INTEGRATION_BRANCH="develop"
  RELEASE_BRANCH="main"
  RETRY_COUNT="2"
  RETRY_DELAY="5"
fi

RETRY_COUNT="${RETRY_COUNT:-2}"
RETRY_DELAY="${RETRY_DELAY:-5}"

TASK_PATH="$MONO_ROOT/$TASK_FILE"
if [[ ! -f "$TASK_PATH" ]]; then
  echo "ERROR: task file not found: $TASK_PATH" >&2
  exit 2
fi

TICKET_REPO="$(grep -iE '^- +Repo:' "$TASK_PATH" | head -n 1 | sed -E 's/^- +Repo:\s*//I' | tr -d '\r' || true)"
TICKET_REPO="$(echo "$TICKET_REPO" | awk '{print tolower($0)}')"
REPO="${REPO_ARG:-${TICKET_REPO:-root}}"

# Validate repo against config
VALID_REPOS=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(' '.join([r['name'] for r in c['repos']] + ['root']))" 2>/dev/null || echo "root backend frontend")
if ! echo "$VALID_REPOS" | grep -qw "$REPO"; then
  echo "ERROR: Repo must be one of: $VALID_REPOS (got '$REPO')" >&2
  exit 2
fi

# ============================================================
# Detect Repo Type and Path from Config
# ============================================================
REPO_TYPE="$(get_repo_type "$REPO" "$CONFIG_FILE")"
REPO_PATH="$(get_repo_path "$REPO" "$CONFIG_FILE")"

# Special handling for root type
if [[ "$REPO" == "root" ]]; then
  REPO_TYPE="root"
  REPO_PATH="./"
fi

# Export for use by other scripts
export REPO_TYPE
export REPO_PATH

worker_log "repo=$REPO type=$REPO_TYPE path=$REPO_PATH"

RELEASE_FLAG="$(grep -iE '^- +Release:' "$TASK_PATH" | head -n 1 | sed -E 's/^- +Release:\s*//I' | tr -d '\r' || echo "false")"
RELEASE_FLAG="$(echo "$RELEASE_FLAG" | awk '{print tolower($0)}')"
if [[ "$RELEASE_FLAG" != "true" ]]; then RELEASE_FLAG="false"; fi

PR_BASE="$INTEGRATION_BRANCH"
if [[ "$RELEASE_FLAG" == "true" ]]; then
  if [[ "$REPO" != "root" ]]; then
    echo "ERROR: Release tickets are allowed only for root repo." >&2
    exit 2
  fi
  PR_BASE="$RELEASE_BRANCH"
fi

LOG_DIR="$MONO_ROOT/.ai/exe-logs"
RUN_DIR="$MONO_ROOT/.ai/runs/issue-$ISSUE_ID"
mkdir -p "$LOG_DIR" "$RUN_DIR" "$MONO_ROOT/.ai/results" "$MONO_ROOT/.ai/state" "$MONO_ROOT/.worktrees"

# Define log file base early so early failures can be logged
# This ensures log files are created even if script exits before codex execution
CODEX_LOG_BASE="$LOG_DIR/issue-$ISSUE_ID.${REPO}.codex"
EARLY_FAILURE_LOG="${CODEX_LOG_BASE}.early-failure.log"

# Helper function to log early failures before codex execution
log_early_failure() {
  local stage="$1"
  local message="$2"
  {
    echo "============================================================"
    echo "EARLY FAILURE LOG - issue-$ISSUE_ID"
    echo "============================================================"
    echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "Stage: $stage"
    echo "Repo: $REPO"
    echo "Repo Type: ${REPO_TYPE:-unknown}"
    echo "Repo Path: ${REPO_PATH:-unknown}"
    echo "Error: $message"
    echo "============================================================"
  } >> "$EARLY_FAILURE_LOG"
}

# Track execution start time
EXEC_START_TIME=$(date +%s)

BRANCH="feat/ai-issue-$ISSUE_ID"

TRACE_DIR="$MONO_ROOT/.ai/state/traces"
TRACE_FILE="$TRACE_DIR/issue-$ISSUE_ID.json"
TRACE_ID="issue-$ISSUE_ID-$(date -u +%Y%m%dT%H%M%SZ)"
TRACE_START_TIME=$(date +%s)
TRACE_ERROR_MESSAGE=""
TRACE_STEP_NAME=""
TRACE_STEP_START=""
TRACE_STEP_START_ISO=""

record_error() {
  TRACE_ERROR_MESSAGE="$1"
}

trace_init() {
  mkdir -p "$TRACE_DIR"
  python3 - "$TRACE_FILE" "$TRACE_ID" "$ISSUE_ID" "$REPO" "$BRANCH" "$PR_BASE" <<'PY' || true
import json
import os
import sys
from datetime import datetime, timezone

path, trace_id, issue_id, repo, branch, base_branch = sys.argv[1:7]
now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

data = {
    "trace_id": trace_id,
    "issue_id": issue_id,
    "repo": repo,
    "branch": branch,
    "base_branch": base_branch,
    "status": "running",
    "started_at": now,
    "ended_at": "",
    "duration_seconds": 0,
    "error": "",
    "steps": [],
}

tmp_path = path + ".tmp"
with open(tmp_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
os.replace(tmp_path, path)
PY
}

trace_step_start() {
  TRACE_STEP_NAME="$1"
  TRACE_STEP_START=$(date +%s)
  TRACE_STEP_START_ISO=$(date -u +%Y-%m-%dT%H:%M:%SZ)
}

trace_step_end() {
  local status="$1"
  local error_message="${2:-}"
  local context_json="${3:-}"
  local end_time
  local end_iso
  local duration
  end_time=$(date +%s)
  end_iso=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  duration=$((end_time - TRACE_STEP_START))

  TRACE_STEP_ERROR="$error_message" TRACE_STEP_CONTEXT="$context_json" \
    python3 - "$TRACE_FILE" "$TRACE_STEP_NAME" "$status" "$TRACE_STEP_START_ISO" "$end_iso" "$duration" <<'PY' || true
import json
import os
import sys

path, name, status, start_iso, end_iso, duration = sys.argv[1:7]
error_message = os.environ.get("TRACE_STEP_ERROR", "")
context_json = os.environ.get("TRACE_STEP_CONTEXT", "")

context = {}
if context_json:
    try:
        context = json.loads(context_json)
    except json.JSONDecodeError:
        context = {"raw": context_json}

try:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
except FileNotFoundError:
    data = {"steps": []}

steps = data.get("steps", [])
step_entry = {
    "name": name,
    "status": status,
    "started_at": start_iso,
    "ended_at": end_iso,
    "duration_seconds": int(duration),
    "error": error_message,
    "context": context,
}
steps.append(step_entry)
data["steps"] = steps
if error_message:
    data["error"] = error_message

tmp_path = path + ".tmp"
with open(tmp_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
os.replace(tmp_path, path)
PY
}

trace_finalize() {
  local rc="${1:-0}"
  local end_time
  local duration
  end_time=$(date +%s)
  duration=$((end_time - TRACE_START_TIME))
  local status="success"
  if [[ "$rc" -ne 0 ]]; then
    status="failed"
  fi
  TRACE_FINAL_STATUS="$status" TRACE_FINAL_ERROR="$TRACE_ERROR_MESSAGE" TRACE_FINAL_DURATION="$duration" \
    python3 - "$TRACE_FILE" <<'PY' || true
import json
import os
import sys
from datetime import datetime, timezone

path = sys.argv[1]
status = os.environ.get("TRACE_FINAL_STATUS", "success")
error_message = os.environ.get("TRACE_FINAL_ERROR", "")
duration = int(os.environ.get("TRACE_FINAL_DURATION", "0"))
end_iso = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

try:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
except FileNotFoundError:
    data = {}

data["status"] = status
data["ended_at"] = end_iso
data["duration_seconds"] = duration
if error_message:
    data["error"] = error_message

tmp_path = path + ".tmp"
with open(tmp_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
os.replace(tmp_path, path)
PY
}

trace_init
trap 'trace_finalize $?' EXIT

export AI_STATE_ROOT="$MONO_ROOT"
trace_step_start "attempt_guard"
if ! bash "$MONO_ROOT/.ai/scripts/attempt_guard.sh" "$ISSUE_ID" "codex-auto"; then
  record_error "attempt_guard failed"
  log_early_failure "attempt_guard" "attempt_guard.sh returned non-zero"
  trace_step_end "failed" "attempt_guard failed"
  exit 2
fi
trace_step_end "success"

# Monorepo support: always create worktree from MONO_ROOT
# For backend/frontend repos, we cd into the subdirectory after worktree creation
WT_DIR="$MONO_ROOT/.worktrees/issue-$ISSUE_ID"
WORK_DIR="$WT_DIR"  # Where codex will actually work

if [[ "$REPO" != "root" ]]; then
  # For monorepo subdirectories, work inside the subdirectory
  # Use REPO_PATH (from config) which may have trailing slash, normalize it
  REPO_PATH_NORMALIZED="${REPO_PATH%/}"  # Remove trailing slash if present
  WORK_DIR="$WT_DIR/$REPO_PATH_NORMALIZED"
fi

worker_log "preflight repo=$REPO type=$REPO_TYPE"
trace_step_start "preflight"
# Always check monorepo root is clean
if [[ -n "$(git -C "$MONO_ROOT" status --porcelain)" ]]; then
  record_error "working tree not clean"
  echo "ERROR: working tree not clean. Commit/stash first." >&2
  git -C "$MONO_ROOT" status --porcelain >&2 || true
  log_early_failure "preflight" "working tree not clean - uncommitted changes detected"
  trace_step_end "failed" "working tree not clean"
  exit 2
fi
# Run preflight for ALL repo types (Req 7.6, 7.7)
if ! bash "$MONO_ROOT/.ai/scripts/preflight.sh" "$REPO_TYPE" "$REPO_PATH"; then
  record_error "preflight failed"
  log_early_failure "preflight" "preflight.sh returned non-zero for repo_type=$REPO_TYPE repo_path=$REPO_PATH"
  trace_step_end "failed" "preflight failed"
  exit 2
fi
trace_step_end "success"

trace_step_start "worktree"
if [[ ! -d "$WT_DIR" ]]; then
  # Pass repo_type and repo_path to new_worktree.sh (Req 14.5)
  WT_DIR="$(bash "$MONO_ROOT/.ai/scripts/new_worktree.sh" "$ISSUE_ID" "$BRANCH" "$REPO_TYPE" "$REPO_PATH")"
  if [[ "$REPO" != "root" ]]; then
    # Use REPO_PATH (from config) which may have trailing slash, normalize it
    REPO_PATH_NORMALIZED="${REPO_PATH%/}"
    WORK_DIR="$WT_DIR/$REPO_PATH_NORMALIZED"
  else
    WORK_DIR="$WT_DIR"
  fi
fi

# Verify work directory exists
if [[ ! -d "$WORK_DIR" ]]; then
  format_error "worktree_setup" "work directory not found: $WORK_DIR" \
    "Check that the repo path '$REPO_PATH' exists in the worktree"
  record_error "work directory not found: $WORK_DIR"
  log_early_failure "worktree" "work directory not found: $WORK_DIR (expected at $WT_DIR/$REPO_PATH)"
  trace_step_end "failed" "work directory not found"
  exit 2
fi

worker_log "worktree=$WT_DIR work_dir=$WORK_DIR"
trace_step_end "success"

cd "$WT_DIR"
git_with_timeout fetch origin --prune >/dev/null 2>&1 || true
git checkout -q "$BRANCH" || true

MODE="${AI_BRANCH_MODE:-reuse}" # reuse|reset
if [[ "$MODE" == "reset" ]]; then
  BASE_REF="${AI_RESET_BASE:-origin/$INTEGRATION_BRANCH}"
  worker_log "reset branch to $BASE_REF"
  git_with_timeout fetch origin --prune >/dev/null 2>&1 || true
  git reset --hard "$BASE_REF"
fi

TITLE_LINE="$(sed -n 's/^#\s\+//p' "$TASK_PATH" | head -n 1 | tr -d '\r')"
[[ -z "$TITLE_LINE" ]] && TITLE_LINE="issue-$ISSUE_ID"

# Build work directory instruction for prompt (Req 10.1-10.5)
WORK_DIR_INSTRUCTION=""
case "$REPO_TYPE" in
  root)
    # Root type: no special path instructions (Req 10.1)
    ;;
  directory)
    # Directory type: explain paths relative to monorepo root (Req 10.2, 10.4)
    WORK_DIR_INSTRUCTION="
IMPORTANT: You are working in a MONOREPO (directory type).
- Working directory: $WORK_DIR
- All file paths should be relative to the worktree root
- Example: $REPO/internal/foo.go (not internal/foo.go)
"
    ;;
  submodule)
    # Submodule type: warn about file boundary (Req 10.3, 10.5)
    WORK_DIR_INSTRUCTION="
IMPORTANT: You are working in a SUBMODULE within a monorepo.
- Submodule path: $REPO_PATH
- Working directory: $WORK_DIR
- WARNING: Do NOT modify files outside the submodule directory!
- All changes must be within: $REPO_PATH/
- Files outside this boundary will cause the commit to fail.
"
    ;;
esac

# ============================================================
# Fetch AWK Review comments from Issue (for retry scenarios)
# ============================================================
AWK_REVIEW_COMMENTS=""
if command -v gh >/dev/null 2>&1; then
  # Get comments containing "AWK Review" (Principal's review feedback)
  AWK_REVIEW_COMMENTS=$(gh_with_timeout issue view "$ISSUE_ID" --json comments --jq '.comments[] | select(.body | contains("AWK Review")) | "---\n\(.createdAt):\n\(.body)"' 2>/dev/null | tail -c 4000 || echo "")
fi

PROMPT_FILE="$RUN_DIR/prompt.txt"
cat > "$PROMPT_FILE" <<PROMPT
You are an automated coding agent running inside a git worktree.

Repo rules:
- Read and follow AGENTS.md.
- Keep changes minimal and strictly within ticket scope.
$WORK_DIR_INSTRUCTION
IMPORTANT: Do NOT run any git commands (commit, push, etc.) or create PRs.
The runner script will handle git operations after you complete the code changes.
Your job is ONLY to:
1. Write/modify code files
2. Run verification commands
3. Report the results

============================================================
FORBIDDEN OPERATIONS (Req 3.4, 3.5)
============================================================
You MUST NOT:
- Read, write, or access any files in .ai/state/principal/
- Read, write, or access session.json or any session log files
- Modify any files in .ai/scripts/ or .ai/commands/
- Access or modify Principal session data
- Forge or manipulate session IDs
- Access audit logs or review data

These paths are reserved for the Principal agent and are protected.
Attempting to access them will result in task failure.
============================================================

Ticket:
$(cat "$TASK_PATH")
PROMPT

# Append AWK Review comments if any (for retry scenarios)
if [[ -n "$AWK_REVIEW_COMMENTS" ]]; then
  cat >> "$PROMPT_FILE" <<REVIEW

============================================================
PREVIOUS REVIEW FEEDBACK (IMPORTANT - Address these issues!)
============================================================
$AWK_REVIEW_COMMENTS
============================================================
REVIEW
fi

# Append final instructions
cat >> "$PROMPT_FILE" <<FINAL

After making changes:
- Print: git status --porcelain
- Print: git diff
- Run verification commands from the ticket.
- Do NOT commit or push - the runner will handle that.
FINAL

# CODEX_LOG_BASE already defined early in script for early failure logging
SUMMARY_FILE="$RUN_DIR/summary.txt"
: > "$SUMMARY_FILE"

# ============================================================
# Read previous session info for retry tracking (Req 8.1, 8.2, 8.3, 8.4)
# ============================================================
RESULT_FILE="$MONO_ROOT/.ai/results/issue-$ISSUE_ID.json"
AI_ATTEMPT_NUMBER=1
AI_PREV_SESSION_IDS="[]"
AI_PREV_FAILURE_REASON=""

if [[ -f "$RESULT_FILE" ]]; then
  # Read previous session info
  PREV_SESSION_ID=$(_jq -r '.session.worker_session_id // ""' "$RESULT_FILE" 2>/dev/null || echo "")
  PREV_ATTEMPT=$(_jq -r '.session.attempt_number // 0' "$RESULT_FILE" 2>/dev/null || echo "0")
  PREV_SESSIONS=$(_jq -c '.session.previous_session_ids // []' "$RESULT_FILE" 2>/dev/null || echo "[]")
  
  if [[ -n "$PREV_SESSION_ID" && "$PREV_SESSION_ID" != "null" ]]; then
    # This is a retry - increment attempt number and add previous session to chain
    AI_ATTEMPT_NUMBER=$((PREV_ATTEMPT + 1))
    # Append previous session ID to the chain
    AI_PREV_SESSION_IDS=$(echo "$PREV_SESSIONS" | _jq -c --arg sid "$PREV_SESSION_ID" '. + [$sid]')
    # Get previous failure reason if available
    AI_PREV_FAILURE_REASON=$(_jq -r '.session.previous_failure_reason // ""' "$RESULT_FILE" 2>/dev/null || echo "")
    worker_log "retry attempt $AI_ATTEMPT_NUMBER (previous: $PREV_SESSION_ID)"
  fi
fi

export AI_ATTEMPT_NUMBER
export AI_PREV_SESSION_IDS
export AI_PREV_FAILURE_REASON

# ============================================================
# Post worker_start comment to Issue (Req 4.2)
# ============================================================
trace_step_start "worker_start_comment"
if command -v gh >/dev/null 2>&1; then
  EXTRA_DATA=""
  if [[ "$AI_ATTEMPT_NUMBER" -gt 1 ]]; then
    EXTRA_DATA="| Attempt | $AI_ATTEMPT_NUMBER |"
  fi
  add_issue_comment "$ISSUE_ID" "$WORKER_SESSION_ID" "worker_start" "" "$EXTRA_DATA" 2>/dev/null || true
  worker_log "posted worker_start comment to Issue #$ISSUE_ID"
fi
trace_step_end "success"

MAX_ATTEMPTS=$((RETRY_COUNT + 1))
ATTEMPT=0
RC=0

set +e
while [[ "$ATTEMPT" -lt "$MAX_ATTEMPTS" ]]; do
  ATTEMPT=$((ATTEMPT + 1))
  CODEX_LOG="${CODEX_LOG_BASE}.attempt-${ATTEMPT}.log"
  worker_log_tee "codex attempt $ATTEMPT/$MAX_ATTEMPTS"
  trace_step_start "codex_exec_attempt_$ATTEMPT"
  NO_RETRY=""
  if command -v codex >/dev/null 2>&1; then
    HELP="$(codex exec --help 2>/dev/null || true)"
    # Build codex command based on available flags
    CODEX_CMD="codex exec"
    if echo "$HELP" | grep -q -- '--full-auto'; then
      CODEX_CMD="$CODEX_CMD --full-auto"
    elif echo "$HELP" | grep -q -- '--yolo'; then
      CODEX_CMD="$CODEX_CMD --yolo"
    fi
    # Use --json for structured output logging (if available)
    if echo "$HELP" | grep -q -- '--json'; then
      CODEX_CMD="$CODEX_CMD --json"
    fi
    # Read prompt from stdin (new codex CLI style)
    # Log output via shell redirection instead of --log-file
    CODEX_TIMEOUT="${AI_CODEX_TIMEOUT:-1800}"
    run_with_timeout "$CODEX_TIMEOUT" $CODEX_CMD < "$PROMPT_FILE" 2>&1 | tee -a "$SUMMARY_FILE" "$CODEX_LOG"
    RC=${PIPESTATUS[0]}
  else
    record_error "codex CLI not found in PATH"
    echo "ERROR: codex CLI not found in PATH" | tee -a "$SUMMARY_FILE"
    RC=127
    NO_RETRY="true"
  fi

  worker_log_tee "codex attempt $ATTEMPT rc=$RC"

  if [[ "$RC" -eq 0 ]]; then
    trace_step_end "success" "" "{\"attempt\": $ATTEMPT}"
    break
  fi
  if [[ "$RC" -eq 124 ]]; then
    worker_log_tee "ERROR: codex timeout after ${CODEX_TIMEOUT}s"
    trace_step_end "failed" "codex timeout after ${CODEX_TIMEOUT}s" "{\"attempt\": $ATTEMPT, \"timeout_seconds\": $CODEX_TIMEOUT}"
  else
    trace_step_end "failed" "codex rc=$RC" "{\"attempt\": $ATTEMPT}"
  fi

  if [[ -n "$NO_RETRY" ]]; then
    break
  fi

  if [[ "$ATTEMPT" -lt "$MAX_ATTEMPTS" ]]; then
    worker_log_tee "retry in ${RETRY_DELAY}s"
    sleep "$RETRY_DELAY"
  fi
done
set -e

RETRY_USED=$((ATTEMPT - 1))
export AI_RETRY_COUNT="$RETRY_USED"

# Calculate execution duration
EXEC_END_TIME=$(date +%s)
EXEC_DURATION=$((EXEC_END_TIME - EXEC_START_TIME))
export AI_EXEC_DURATION="$EXEC_DURATION"

if [[ "$RC" -ne 0 ]]; then
  if [[ "$RC" -eq 124 ]]; then
    record_error "codex timeout after ${CODEX_TIMEOUT}s"
    worker_log_tee "codex timeout after ${CODEX_TIMEOUT}s"
    export AI_FAILURE_STAGE="codex_timeout"
  else
    record_error "codex failed rc=$RC"
    worker_log_tee "codex failed rc=$RC"
    export AI_FAILURE_STAGE="codex_exec"
  fi
  
  # ============================================================
  # Extract failure reason for retry tracking (Req 8.3)
  # ============================================================
  AI_FAILURE_REASON=""
  if [[ "$RC" -eq 124 ]]; then
    AI_FAILURE_REASON="codex timeout after ${CODEX_TIMEOUT}s"
  else
    if [[ -f "$CODEX_LOG" ]]; then
      AI_FAILURE_REASON=$(tail -n 50 "$CODEX_LOG" | grep -iE "ERROR|FAILED|Exception|error:" | head -n 5 | tr '\n' ' ' || echo "Unknown error")
    fi
    if [[ -z "$AI_FAILURE_REASON" ]]; then
      AI_FAILURE_REASON="codex exit code $RC"
    fi
  fi
  export AI_FAILURE_REASON
  
  export AI_RESULTS_ROOT="$MONO_ROOT"
  export AI_REPO_NAME="$REPO"
  export AI_BRANCH_NAME="$BRANCH"
  export AI_PR_BASE="$PR_BASE"
  export AI_STATE_ROOT="$WT_DIR"
  trace_step_start "write_result"
  if ! bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE"; then
    trace_step_end "failed" "write_result failed"
    exit "$RC"
  fi
  trace_step_end "success"
  
  # ============================================================
  # Post worker_complete (failed) comment to Issue (Req 4.3)
  # ============================================================
  trace_step_start "worker_complete_comment"
  if command -v gh >/dev/null 2>&1; then
    EXTRA_DATA="| Status | failed |
| Duration | $(format_duration "$AI_EXEC_DURATION") |
| Exit Code | $RC |"
    add_issue_comment "$ISSUE_ID" "$WORKER_SESSION_ID" "worker_complete" "" "$EXTRA_DATA" 2>/dev/null || true
    worker_log "posted worker_complete (failed) comment to Issue #$ISSUE_ID"
  fi
  trace_step_end "success"
  
  exit "$RC"
fi

{
  echo "=== git status ==="
  git status --porcelain
  echo
  echo "=== git diff ==="
  git diff
} >> "$SUMMARY_FILE" || true

TYPE="chore"
SUBJECT="$TITLE_LINE"
if echo "$TITLE_LINE" | grep -qE '^\[[a-z]+\]\s+'; then
  TYPE="$(echo "$TITLE_LINE" | sed -E 's/^\[([a-z]+)\].*$/\1/')"
  SUBJECT="$(echo "$TITLE_LINE" | sed -E 's/^\[[a-z]+\]\s+//')"
fi

SUBJECT="$(echo "$SUBJECT" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9 _-]/ /g' | tr -s ' ' | sed 's/^ *//;s/ *$//')"
COMMIT_MSG="[$TYPE] $SUBJECT"

# Security checks before commit (Req 25.1-25.5, 29.1-29.5)
trace_step_start "security_check"

# Read security flags from ticket
ALLOW_SCRIPT_CHANGES="$(grep -iE '^- +allow_script_changes:' "$TASK_PATH" | head -n 1 | sed -E 's/^- +allow_script_changes:\s*//I' | tr -d '\r' || echo "false")"
ALLOW_SCRIPT_CHANGES="$(echo "$ALLOW_SCRIPT_CHANGES" | awk '{print tolower($0)}')"
SCRIPT_WHITELIST="$(grep -iE '^- +script_whitelist:' "$TASK_PATH" | head -n 1 | sed -E 's/^- +script_whitelist:\s*//I' | tr -d '\r' || true)"

ALLOW_SECRETS="$(grep -iE '^- +allow_secrets:' "$TASK_PATH" | head -n 1 | sed -E 's/^- +allow_secrets:\s*//I' | tr -d '\r' || echo "false")"
ALLOW_SECRETS="$(echo "$ALLOW_SECRETS" | awk '{print tolower($0)}')"
CUSTOM_SECRET_PATTERNS="$(grep -iE '^- +secret_patterns:' "$TASK_PATH" | head -n 1 | sed -E 's/^- +secret_patterns:\s*//I' | tr -d '\r' || true)"

# Stage changes first to check them
git add -A
if [[ "$REPO" == "root" ]]; then
  git reset -q .ai .worktrees >/dev/null 2>&1 || true
fi

# Check for script modifications (Req 25.1-25.5)
if ! check_script_modifications "$WT_DIR" "$ALLOW_SCRIPT_CHANGES" "$SCRIPT_WHITELIST"; then
  record_error "script modification not allowed"
  trace_step_end "failed" "script modification not allowed"
  exit 2
fi

# Check for sensitive information (Req 29.1-29.5)
if ! check_sensitive_info "$WT_DIR" "$ALLOW_SECRETS" "$CUSTOM_SECRET_PATTERNS"; then
  record_error "sensitive information detected"
  trace_step_end "failed" "sensitive information detected"
  exit 2
fi

trace_step_end "success"

trace_step_start "git_commit"

# Clean up any stale index.lock files (may be left by sandbox restrictions)
if [[ -f "$WT_DIR/.git/index.lock" ]]; then
  worker_log_tee "WARNING: Removing stale index.lock"
  rm -f "$WT_DIR/.git/index.lock" 2>/dev/null || true
fi

# For worktrees, .git is a file pointing to the actual git dir
# Check if there's a lock in the actual git dir
if [[ -f "$WT_DIR/.git" ]]; then
  ACTUAL_GIT_DIR="$(cat "$WT_DIR/.git" | sed 's/gitdir: //')"
  if [[ -f "$ACTUAL_GIT_DIR/index.lock" ]]; then
    worker_log_tee "WARNING: Removing stale index.lock from gitdir"
    rm -f "$ACTUAL_GIT_DIR/index.lock" 2>/dev/null || true
  fi
fi

# Handle git operations based on repo type
if [[ "$REPO_TYPE" == "submodule" ]]; then
  # Submodule type: check boundary, commit submodule first, then parent (Req 6.1-6.5, 20.1-20.4)
  ALLOW_PARENT_CHANGES="$(grep -iE '^- +allow_parent_changes:' "$TASK_PATH" | head -n 1 | sed -E 's/^- +allow_parent_changes:\s*//I' | tr -d '\r' || echo "false")"
  ALLOW_PARENT_CHANGES="$(echo "$ALLOW_PARENT_CHANGES" | awk '{print tolower($0)}')"
  
  if ! check_submodule_boundary "$WT_DIR" "$REPO_PATH" "$ALLOW_PARENT_CHANGES"; then
    record_error "changes outside submodule boundary"
    trace_step_end "failed" "changes outside submodule boundary"
    exit 2
  fi
  
  if ! git_commit_submodule "$WT_DIR" "$REPO_PATH" "$COMMIT_MSG"; then
    RC=$?
    if [[ "$RC" -eq 1 ]]; then
      record_error "no changes in submodule"
      echo "ERROR: no changes staged in submodule." | tee -a "$SUMMARY_FILE"
      trace_step_end "failed" "no changes in submodule"
    else
      record_error "submodule commit failed"
      trace_step_end "failed" "submodule commit failed"
    fi
    export AI_RESULTS_ROOT="$MONO_ROOT"
    export AI_REPO_NAME="$REPO"
    export AI_BRANCH_NAME="$BRANCH"
    export AI_PR_BASE="$PR_BASE"
    export AI_STATE_ROOT="$WT_DIR"
    export AI_SUBMODULE_SHA="$SUBMODULE_SHA"
    export AI_CONSISTENCY_STATUS="$CONSISTENCY_STATUS"
    trace_step_start "write_result"
    bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE" || true
    trace_step_end "success"
    exit 2
  fi
else
  # Root/Directory type: standard git operations
  # Files already staged in security_check step

  if git diff --cached --quiet; then
    record_error "no changes staged"
    echo "ERROR: no changes staged; nothing to commit." | tee -a "$SUMMARY_FILE"
    trace_step_end "failed" "no changes staged"
    export AI_RESULTS_ROOT="$MONO_ROOT"
    export AI_REPO_NAME="$REPO"
    export AI_BRANCH_NAME="$BRANCH"
    export AI_PR_BASE="$PR_BASE"
    export AI_STATE_ROOT="$WT_DIR"
    trace_step_start "write_result"
    if ! bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE"; then
      trace_step_end "failed" "write_result failed"
      exit 2
    fi
    trace_step_end "success"
    exit 2
  fi

  if ! git commit -m "$COMMIT_MSG"; then
    record_error "git commit failed"
    trace_step_end "failed" "git commit failed"
    exit 2
  fi
fi
trace_step_end "success"

trace_step_start "git_push"
if [[ "$REPO_TYPE" == "submodule" ]]; then
  # Submodule type: push submodule first, then parent (Req 6.3-6.5, 24.1-24.3)
  if ! git_push_submodule "$WT_DIR" "$REPO_PATH" "$BRANCH"; then
    record_error "submodule push failed: $CONSISTENCY_STATUS"
    trace_step_end "failed" "submodule push failed"
    export AI_RESULTS_ROOT="$MONO_ROOT"
    export AI_REPO_NAME="$REPO"
    export AI_BRANCH_NAME="$BRANCH"
    export AI_PR_BASE="$PR_BASE"
    export AI_STATE_ROOT="$WT_DIR"
    export AI_SUBMODULE_SHA="$SUBMODULE_SHA"
    export AI_CONSISTENCY_STATUS="$CONSISTENCY_STATUS"
    trace_step_start "write_result"
    bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE" || true
    trace_step_end "success"
    exit 2
  fi
else
  # Root/Directory type: standard push
  if ! git_with_timeout push -u origin "$BRANCH"; then
    record_error "git push failed"
    trace_step_end "failed" "git push failed"
    exit 2
  fi
fi
trace_step_end "success"

PR_URL=""
trace_step_start "create_pr"
if command -v gh >/dev/null 2>&1; then
  # Check if PR already exists for this branch
  # Use gh pr list which has better compatibility across gh versions
  EXISTING_PR_URL="$(gh_with_timeout pr list --head "$BRANCH" --json url -q '.[0].url' 2>/dev/null || true)"

  if [[ -n "$EXISTING_PR_URL" ]]; then
    # PR already exists (retry scenario), just use existing PR
    PR_URL="$EXISTING_PR_URL"
    worker_log_tee "PR already exists: $PR_URL"
  else
    # Create new PR
    # gh pr create outputs the URL directly on success
    PR_URL="$(gh_with_timeout pr create \
      --base "$PR_BASE" \
      --head "$BRANCH" \
      --title "$COMMIT_MSG" \
      --body "Closes #$ISSUE_ID

$COMMIT_MSG" 2>&1 | grep -oE 'https://github.com/[^[:space:]]+/pull/[0-9]+' | head -1 || true)"
  fi
fi

if [[ -z "$PR_URL" ]]; then
  record_error "PR not created"
  echo "ERROR: PR not created or found. Ensure 'gh auth login' and required repo permissions." | tee -a "$SUMMARY_FILE"
  trace_step_end "failed" "PR not created"
  export AI_RESULTS_ROOT="$MONO_ROOT"
  export AI_REPO_NAME="$REPO"
  export AI_BRANCH_NAME="$BRANCH"
  export AI_PR_BASE="$PR_BASE"
  export AI_STATE_ROOT="$WT_DIR"
  trace_step_start "write_result"
  if ! bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE"; then
    trace_step_end "failed" "write_result failed"
    exit 2
  fi
  trace_step_end "success"
  exit 2
fi
trace_step_end "success"

export AI_RESULTS_ROOT="$MONO_ROOT"
export AI_REPO_NAME="$REPO"
export AI_BRANCH_NAME="$BRANCH"
export AI_PR_BASE="$PR_BASE"
export AI_STATE_ROOT="$WT_DIR"
export AI_REPO_TYPE="$REPO_TYPE"
export AI_SUBMODULE_SHA="$SUBMODULE_SHA"
export AI_CONSISTENCY_STATUS="$CONSISTENCY_STATUS"
trace_step_start "write_result"
if ! bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "success" "$PR_URL" "$SUMMARY_FILE"; then
  trace_step_end "failed" "write_result failed"
  exit 2
fi
trace_step_end "success"

# ============================================================
# Post worker_complete comment to Issue (Req 4.3)
# ============================================================
trace_step_start "worker_complete_comment"
if command -v gh >/dev/null 2>&1; then
  EXTRA_DATA=$(build_worker_complete_extra "$PR_URL" "$AI_EXEC_DURATION")
  add_issue_comment "$ISSUE_ID" "$WORKER_SESSION_ID" "worker_complete" "" "$EXTRA_DATA" 2>/dev/null || true
  worker_log "posted worker_complete comment to Issue #$ISSUE_ID"
fi
trace_step_end "success"

# Reset fail_count on success so subsequent runs start fresh
if [[ -f "$RUN_DIR/fail_count.txt" ]]; then
  worker_log "resetting fail_count (success)"
  rm -f "$RUN_DIR/fail_count.txt"
fi

echo "DONE: repo=$REPO type=$REPO_TYPE branch=$BRANCH"
echo "PR:   $PR_URL"
echo "LOG:  $CODEX_LOG"
echo "JSON: $MONO_ROOT/.ai/results/issue-$ISSUE_ID.json"
