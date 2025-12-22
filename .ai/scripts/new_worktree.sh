#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# Worktree Creation with Multi-Repo Support
# Requirements: 2.3, 3.3, 4.1-4.5, 14.1-14.5
# ============================================================

ISSUE_ID="${1:?usage: new_worktree.sh <issue_id> <branch_name> [repo_type] [repo_path]}"
BRANCH="${2:?usage: new_worktree.sh <issue_id> <branch_name> [repo_type] [repo_path]}"
REPO_TYPE="${3:-root}"      # root | directory | submodule
REPO_PATH="${4:-.}"         # Path relative to monorepo root

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

# If worktree already exists, reuse it (idempotent) (Req 14.3)
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

# Ensure target branch exists (local or from remote) (Req 14.1, 14.2)
if git -C "$ROOT" show-ref --verify --quiet "refs/heads/$BRANCH"; then
  :
elif git -C "$ROOT" show-ref --verify --quiet "refs/remotes/origin/$BRANCH"; then
  git -C "$ROOT" branch "$BRANCH" "origin/$BRANCH" >&2
else
  # Create from integration_branch (Req 14.2)
  git -C "$ROOT" branch "$BRANCH" "$BASE" >&2
fi

# Create worktree attached to that branch
git -C "$ROOT" worktree add "$WT_DIR" "$BRANCH" >&2

# ============================================================
# Type-Specific Post-Creation Steps
# ============================================================

case "$REPO_TYPE" in
  root)
    # Root type: init all submodules (Req 2.3)
    echo "[new_worktree] init submodules (root type)" >&2
    git -C "$WT_DIR" submodule sync --recursive >&2 || true
    git -C "$WT_DIR" submodule update --init --recursive >&2 || true
    ;;

  directory)
    # Directory type: verify WORK_DIR exists (Req 3.3, 14.4)
    WORK_DIR="$WT_DIR/$REPO_PATH"
    if [[ ! -d "$WORK_DIR" ]]; then
      echo "ERROR: directory path '$REPO_PATH' does not exist in worktree." >&2
      echo "  Worktree: $WT_DIR" >&2
      echo "  Expected: $WORK_DIR" >&2
      # Cleanup failed worktree
      git -C "$ROOT" worktree remove --force "$WT_DIR" >&2 || true
      exit 2
    fi
    echo "[new_worktree] verified directory exists: $WORK_DIR" >&2
    ;;

  submodule)
    # Submodule type: init specific submodule (Req 4.1-4.5, 14.4)
    echo "[new_worktree] init submodule: $REPO_PATH" >&2
    
    # Sync and init the specific submodule
    git -C "$WT_DIR" submodule sync "$REPO_PATH" >&2 || true
    git -C "$WT_DIR" submodule update --init --recursive "$REPO_PATH" >&2
    
    # Verify submodule directory exists (Req 4.4)
    SUBMODULE_DIR="$WT_DIR/$REPO_PATH"
    if [[ ! -d "$SUBMODULE_DIR" ]]; then
      echo "ERROR: submodule '$REPO_PATH' directory does not exist after init." >&2
      echo "  Worktree: $WT_DIR" >&2
      echo "  Expected: $SUBMODULE_DIR" >&2
      # Cleanup failed worktree
      git -C "$ROOT" worktree remove --force "$WT_DIR" >&2 || true
      exit 2
    fi
    
    # Verify submodule has .git (Req 4.3)
    if [[ ! -e "$SUBMODULE_DIR/.git" ]]; then
      echo "ERROR: submodule '$REPO_PATH' not properly initialized (no .git)." >&2
      # Cleanup failed worktree
      git -C "$ROOT" worktree remove --force "$WT_DIR" >&2 || true
      exit 2
    fi
    
    echo "[new_worktree] submodule initialized: $SUBMODULE_DIR" >&2
    ;;

  *)
    echo "ERROR: unknown repo type '$REPO_TYPE'. Expected: root, directory, submodule." >&2
    exit 2
    ;;
esac

echo "$WT_DIR"
