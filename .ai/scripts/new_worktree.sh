#!/usr/bin/env bash
set -euo pipefail

ISSUE_ID="${1:?usage: new_worktree.sh <issue_id> <branch_name>}"
BRANCH="${2:?usage: new_worktree.sh <issue_id> <branch_name>}"

ROOT="$(git rev-parse --show-toplevel)"
WT_DIR="$ROOT/.worktrees/issue-$ISSUE_ID"
CONFIG_FILE="$ROOT/.ai/config/workflow.yaml"

# Read integration branch from config
if [[ -f "$CONFIG_FILE" ]]; then
  DEFAULT_BASE=$(python3 -c "import yaml; c=yaml.safe_load(open('$CONFIG_FILE')); print(c['git']['integration_branch'])" 2>/dev/null || echo "develop")
else
  DEFAULT_BASE="develop"
fi

BASE="${AI_BASE_BRANCH:-$DEFAULT_BASE}"
REMOTE_BASE="${AI_REMOTE_BASE:-origin/$DEFAULT_BASE}"

mkdir -p "$ROOT/.worktrees" "$ROOT/.ai/runs" "$ROOT/.ai/results" "$ROOT/.ai/exe-logs"

# If worktree already exists, reuse it (idempotent)
if [[ -d "$WT_DIR" ]]; then
  echo "$WT_DIR"
  exit 0
fi

# Ensure base is up to date once (no loops)
echo "[new_worktree] fetch origin" >&2
git -C "$ROOT" fetch origin --prune >&2

# Ensure local base exists
if git -C "$ROOT" show-ref --verify --quiet "refs/heads/$BASE"; then
  :
else
  # create local base from remote
  git -C "$ROOT" checkout -q -b "$BASE" "$REMOTE_BASE" >&2
fi

# Fast-forward base
git -C "$ROOT" checkout -q "$BASE" >&2
git -C "$ROOT" pull --ff-only origin "$BASE" >&2 || true

# Ensure target branch exists (local or from remote)
if git -C "$ROOT" show-ref --verify --quiet "refs/heads/$BRANCH"; then
  :
elif git -C "$ROOT" show-ref --verify --quiet "refs/remotes/origin/$BRANCH"; then
  git -C "$ROOT" branch "$BRANCH" "origin/$BRANCH" >&2
else
  git -C "$ROOT" branch "$BRANCH" "$BASE" >&2
fi

# Create worktree attached to that branch
git -C "$ROOT" worktree add "$WT_DIR" "$BRANCH" >&2

echo "$WT_DIR"
