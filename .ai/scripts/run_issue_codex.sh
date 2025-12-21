#!/usr/bin/env bash
set -euo pipefail

ISSUE_ID="${1:?usage: run_issue_codex.sh <issue_id> <task_file> [root|backend|frontend]}"
TASK_FILE="${2:?usage: run_issue_codex.sh <issue_id> <task_file> [root|backend|frontend]}"
REPO_ARG="${3:-}"  # optional override

MONO_ROOT="$(git rev-parse --show-toplevel)"
AI_ROOT="$MONO_ROOT/.ai"
CONFIG_FILE="$AI_ROOT/config/workflow.yaml"

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

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
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

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
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

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
PY
}

trace_init
trap 'trace_finalize $?' EXIT

export AI_STATE_ROOT="$MONO_ROOT"
trace_step_start "attempt_guard"
if ! bash "$MONO_ROOT/.ai/scripts/attempt_guard.sh" "$ISSUE_ID" "codex-auto"; then
  record_error "attempt_guard failed"
  trace_step_end "failed" "attempt_guard failed"
  exit 2
fi
trace_step_end "success"

TARGET_REPO_ROOT="$MONO_ROOT"
WT_DIR="$MONO_ROOT/.worktrees/issue-$ISSUE_ID"

if [[ "$REPO" == "backend" || "$REPO" == "frontend" ]]; then
  TARGET_REPO_ROOT="$MONO_ROOT/$REPO"
  WT_DIR="$MONO_ROOT/.worktrees/${REPO}-issue-$ISSUE_ID"
fi

echo "[runner] preflight repo=$REPO"
trace_step_start "preflight"
if [[ "$REPO" == "root" ]]; then
  if ! bash "$MONO_ROOT/.ai/scripts/preflight.sh"; then
    record_error "preflight failed"
    trace_step_end "failed" "preflight failed"
    exit 2
  fi
else
  if [[ -n "$(git -C "$MONO_ROOT" status --porcelain)" ]]; then
    record_error "monorepo root working tree not clean"
    echo "ERROR: monorepo root working tree not clean. Commit/stash first." >&2
    git -C "$MONO_ROOT" status --porcelain >&2 || true
    trace_step_end "failed" "monorepo root working tree not clean"
    exit 2
  fi
  if [[ -n "$(git -C "$TARGET_REPO_ROOT" status --porcelain)" ]]; then
    record_error "$REPO working tree not clean"
    echo "ERROR: $REPO working tree not clean. Commit/stash first." >&2
    git -C "$TARGET_REPO_ROOT" status --porcelain >&2 || true
    trace_step_end "failed" "$REPO working tree not clean"
    exit 2
  fi
fi
trace_step_end "success"

trace_step_start "worktree"
if [[ ! -d "$WT_DIR" ]]; then
  if [[ "$REPO" == "root" ]]; then
    WT_DIR="$(bash "$MONO_ROOT/.ai/scripts/new_worktree.sh" "$ISSUE_ID" "$BRANCH")"
  else
    echo "[runner] create worktree for $REPO at $WT_DIR"
    git -C "$TARGET_REPO_ROOT" fetch origin --prune >/dev/null 2>&1 || true

    if ! git -C "$TARGET_REPO_ROOT" show-ref --verify --quiet "refs/heads/$INTEGRATION_BRANCH"; then
      if git -C "$TARGET_REPO_ROOT" show-ref --verify --quiet "refs/remotes/origin/$INTEGRATION_BRANCH"; then
        git -C "$TARGET_REPO_ROOT" branch "$INTEGRATION_BRANCH" "origin/$INTEGRATION_BRANCH" >/dev/null 2>&1 || true
      fi
    fi

    if git -C "$TARGET_REPO_ROOT" show-ref --verify --quiet "refs/heads/$BRANCH"; then
      :
    elif git -C "$TARGET_REPO_ROOT" show-ref --verify --quiet "refs/remotes/origin/$BRANCH"; then
      git -C "$TARGET_REPO_ROOT" branch "$BRANCH" "origin/$BRANCH" >/dev/null 2>&1 || true
    else
      git -C "$TARGET_REPO_ROOT" branch "$BRANCH" "$INTEGRATION_BRANCH" >/dev/null 2>&1 || git -C "$TARGET_REPO_ROOT" branch "$BRANCH" "origin/$INTEGRATION_BRANCH" >/dev/null 2>&1
    fi

    git -C "$TARGET_REPO_ROOT" worktree add "$WT_DIR" "$BRANCH" >/dev/null
  fi
fi
echo "[runner] worktree=$WT_DIR"
trace_step_end "success"

cd "$WT_DIR"
git fetch origin --prune >/dev/null 2>&1 || true
git checkout -q "$BRANCH" || true

MODE="${AI_BRANCH_MODE:-reuse}" # reuse|reset
if [[ "$MODE" == "reset" ]]; then
  BASE_REF="${AI_RESET_BASE:-origin/$INTEGRATION_BRANCH}"
  echo "[runner] reset branch to $BASE_REF"
  git fetch origin --prune >/dev/null 2>&1 || true
  git reset --hard "$BASE_REF"
fi

TITLE_LINE="$(sed -n 's/^#\s\+//p' "$TASK_PATH" | head -n 1 | tr -d '\r')"
[[ -z "$TITLE_LINE" ]] && TITLE_LINE="issue-$ISSUE_ID"

PROMPT_FILE="$RUN_DIR/prompt.txt"
cat > "$PROMPT_FILE" <<PROMPT
You are an automated coding agent running inside a git worktree.

Repo rules:
- Read and follow CLAUDE.md and AGENTS.md.
- Keep changes minimal and strictly within ticket scope.

IMPORTANT: Do NOT run any git commands (commit, push, etc.) or create PRs.
The runner script will handle git operations after you complete the code changes.
Your job is ONLY to:
1. Write/modify code files
2. Run verification commands
3. Report the results

Ticket:
$(cat "$TASK_PATH")

After making changes:
- Print: git status --porcelain
- Print: git diff
- Run verification commands from the ticket.
- Do NOT commit or push - the runner will handle that.
PROMPT

CODEX_LOG_BASE="$LOG_DIR/issue-$ISSUE_ID.${REPO}.codex"
SUMMARY_FILE="$RUN_DIR/summary.txt"
: > "$SUMMARY_FILE"

MAX_ATTEMPTS=$((RETRY_COUNT + 1))
ATTEMPT=0
RC=0

set +e
while [[ "$ATTEMPT" -lt "$MAX_ATTEMPTS" ]]; do
  ATTEMPT=$((ATTEMPT + 1))
  CODEX_LOG="${CODEX_LOG_BASE}.attempt-${ATTEMPT}.log"
  echo "[runner] codex attempt $ATTEMPT/$MAX_ATTEMPTS" | tee -a "$SUMMARY_FILE"
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
    $CODEX_CMD < "$PROMPT_FILE" 2>&1 | tee -a "$SUMMARY_FILE" "$CODEX_LOG"
    RC=${PIPESTATUS[0]}
  else
    record_error "codex CLI not found in PATH"
    echo "ERROR: codex CLI not found in PATH" | tee -a "$SUMMARY_FILE"
    RC=127
    NO_RETRY="true"
  fi

  echo "[runner] codex attempt $ATTEMPT rc=$RC" | tee -a "$SUMMARY_FILE"

  if [[ "$RC" -eq 0 ]]; then
    trace_step_end "success" "" "{\"attempt\": $ATTEMPT}"
    break
  fi
  trace_step_end "failed" "codex rc=$RC" "{\"attempt\": $ATTEMPT}"

  if [[ -n "$NO_RETRY" ]]; then
    break
  fi

  if [[ "$ATTEMPT" -lt "$MAX_ATTEMPTS" ]]; then
    echo "[runner] retry in ${RETRY_DELAY}s" | tee -a "$SUMMARY_FILE"
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
  record_error "codex failed rc=$RC"
  echo "[runner] codex failed rc=$RC" | tee -a "$SUMMARY_FILE"
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

trace_step_start "git_commit"

# Clean up any stale index.lock files (may be left by sandbox restrictions)
if [[ -f "$WT_DIR/.git/index.lock" ]]; then
  echo "[runner] WARNING: Removing stale index.lock" | tee -a "$SUMMARY_FILE"
  rm -f "$WT_DIR/.git/index.lock" 2>/dev/null || true
fi

# For worktrees, .git is a file pointing to the actual git dir
# Check if there's a lock in the actual git dir
if [[ -f "$WT_DIR/.git" ]]; then
  ACTUAL_GIT_DIR="$(cat "$WT_DIR/.git" | sed 's/gitdir: //')"
  if [[ -f "$ACTUAL_GIT_DIR/index.lock" ]]; then
    echo "[runner] WARNING: Removing stale index.lock from gitdir" | tee -a "$SUMMARY_FILE"
    rm -f "$ACTUAL_GIT_DIR/index.lock" 2>/dev/null || true
  fi
fi

git add -A
if [[ "$REPO" == "root" ]]; then
  git reset -q .ai .worktrees >/dev/null 2>&1 || true
fi

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
trace_step_end "success"

trace_step_start "git_push"
if ! git push -u origin "$BRANCH"; then
  record_error "git push failed"
  trace_step_end "failed" "git push failed"
  exit 2
fi
trace_step_end "success"

PR_URL=""
trace_step_start "create_pr"
if command -v gh >/dev/null 2>&1; then
  PR_URL="$(gh pr create \
    --base "$PR_BASE" \
    --head "$BRANCH" \
    --title "$COMMIT_MSG" \
    --body "Closes #$ISSUE_ID

$COMMIT_MSG" \
    --json url -q .url 2>/dev/null || true)"
fi

if [[ -z "$PR_URL" ]]; then
  record_error "PR not created"
  echo "ERROR: PR not created. Ensure 'gh auth login' and required repo permissions." | tee -a "$SUMMARY_FILE"
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
trace_step_start "write_result"
if ! bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "success" "$PR_URL" "$SUMMARY_FILE"; then
  trace_step_end "failed" "write_result failed"
  exit 2
fi
trace_step_end "success"

echo "DONE: repo=$REPO branch=$BRANCH"
echo "PR:   $PR_URL"
echo "LOG:  $CODEX_LOG"
echo "JSON: $MONO_ROOT/.ai/results/issue-$ISSUE_ID.json"
