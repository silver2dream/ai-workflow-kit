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
else
  INTEGRATION_BRANCH="develop"
  RELEASE_BRANCH="main"
fi

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

export AI_STATE_ROOT="$MONO_ROOT"
bash "$MONO_ROOT/.ai/scripts/attempt_guard.sh" "$ISSUE_ID" "codex-auto"

TARGET_REPO_ROOT="$MONO_ROOT"
WT_DIR="$MONO_ROOT/.worktrees/issue-$ISSUE_ID"

if [[ "$REPO" == "backend" || "$REPO" == "frontend" ]]; then
  TARGET_REPO_ROOT="$MONO_ROOT/$REPO"
  WT_DIR="$MONO_ROOT/.worktrees/${REPO}-issue-$ISSUE_ID"
fi

echo "[runner] preflight repo=$REPO"
if [[ "$REPO" == "root" ]]; then
  bash "$MONO_ROOT/.ai/scripts/preflight.sh"
else
  if [[ -n "$(git -C "$MONO_ROOT" status --porcelain)" ]]; then
    echo "ERROR: monorepo root working tree not clean. Commit/stash first." >&2
    git -C "$MONO_ROOT" status --porcelain >&2 || true
    exit 2
  fi
  if [[ -n "$(git -C "$TARGET_REPO_ROOT" status --porcelain)" ]]; then
    echo "ERROR: $REPO working tree not clean. Commit/stash first." >&2
    git -C "$TARGET_REPO_ROOT" status --porcelain >&2 || true
    exit 2
  fi
fi

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
- Use commit format: [type] subject (lowercase).
- Create a PR (base: $PR_BASE) and include "Closes #$ISSUE_ID" in the PR body.

Ticket:
$(cat "$TASK_PATH")

After making changes:
- Print: git status --porcelain
- Print: git diff
- Run verification commands from the ticket.
PROMPT

CODEX_LOG="$LOG_DIR/issue-$ISSUE_ID.${REPO}.codex.log"
SUMMARY_FILE="$RUN_DIR/summary.txt"

set +e
if command -v codex >/dev/null 2>&1; then
  HELP="$(codex exec --help 2>/dev/null || true)"
  if echo "$HELP" | grep -q -- '--full-auto'; then
    codex exec --full-auto --log-file "$CODEX_LOG" --prompt-file "$PROMPT_FILE" | tee "$SUMMARY_FILE"
    RC=${PIPESTATUS[0]}
  elif echo "$HELP" | grep -q -- '--yolo'; then
    codex exec --yolo --log-file "$CODEX_LOG" --prompt-file "$PROMPT_FILE" | tee "$SUMMARY_FILE"
    RC=${PIPESTATUS[0]}
  else
    codex exec --log-file "$CODEX_LOG" --prompt-file "$PROMPT_FILE" | tee "$SUMMARY_FILE"
    RC=${PIPESTATUS[0]}
  fi
else
  echo "ERROR: codex CLI not found in PATH" | tee "$SUMMARY_FILE"
  RC=127
fi
set -e

# Calculate execution duration
EXEC_END_TIME=$(date +%s)
EXEC_DURATION=$((EXEC_END_TIME - EXEC_START_TIME))
export AI_EXEC_DURATION="$EXEC_DURATION"

if [[ "$RC" -ne 0 ]]; then
  echo "[runner] codex failed rc=$RC" | tee -a "$SUMMARY_FILE"
  export AI_RESULTS_ROOT="$MONO_ROOT"
  export AI_REPO_NAME="$REPO"
  export AI_BRANCH_NAME="$BRANCH"
  export AI_PR_BASE="$PR_BASE"
  export AI_STATE_ROOT="$WT_DIR"
  bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE"
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

git add -A
if [[ "$REPO" == "root" ]]; then
  git reset -q .ai .worktrees >/dev/null 2>&1 || true
fi

if git diff --cached --quiet; then
  echo "ERROR: no changes staged; nothing to commit." | tee -a "$SUMMARY_FILE"
  export AI_RESULTS_ROOT="$MONO_ROOT"
  export AI_REPO_NAME="$REPO"
  export AI_BRANCH_NAME="$BRANCH"
  export AI_PR_BASE="$PR_BASE"
  export AI_STATE_ROOT="$WT_DIR"
  bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE"
  exit 2
fi

git commit -m "$COMMIT_MSG"
git push -u origin "$BRANCH"

PR_URL=""
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
  echo "ERROR: PR not created. Ensure 'gh auth login' and required repo permissions." | tee -a "$SUMMARY_FILE"
  export AI_RESULTS_ROOT="$MONO_ROOT"
  export AI_REPO_NAME="$REPO"
  export AI_BRANCH_NAME="$BRANCH"
  export AI_PR_BASE="$PR_BASE"
  export AI_STATE_ROOT="$WT_DIR"
  bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "failed" "" "$SUMMARY_FILE"
  exit 2
fi

export AI_RESULTS_ROOT="$MONO_ROOT"
export AI_REPO_NAME="$REPO"
export AI_BRANCH_NAME="$BRANCH"
export AI_PR_BASE="$PR_BASE"
export AI_STATE_ROOT="$WT_DIR"
bash "$MONO_ROOT/.ai/scripts/write_result.sh" "$ISSUE_ID" "success" "$PR_URL" "$SUMMARY_FILE"

echo "DONE: repo=$REPO branch=$BRANCH"
echo "PR:   $PR_URL"
echo "LOG:  $CODEX_LOG"
echo "JSON: $MONO_ROOT/.ai/results/issue-$ISSUE_ID.json"
