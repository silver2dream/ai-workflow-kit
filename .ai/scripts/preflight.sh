#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# Preflight Checks for Multi-Repo Support
# Requirements: 7.1-7.7, 19.1-19.4, 23.1-23.4
# ============================================================

REPO_TYPE="${1:-root}"  # root | directory | submodule
REPO_PATH="${2:-.}"     # Path relative to monorepo root

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

# Cache directory for remote accessibility results (Req 19.4)
CACHE_DIR="$ROOT/.ai/state/cache"
REMOTE_CACHE_FILE="$CACHE_DIR/remote_accessibility.json"
CACHE_TTL_SECONDS=300  # 5 minutes

mkdir -p "$CACHE_DIR"

# ============================================================
# Helper Functions
# ============================================================

# Check if cached result is still valid
is_cache_valid() {
  local cache_file="$1"
  local key="$2"
  
  if [[ ! -f "$cache_file" ]]; then
    return 1
  fi
  
  python3 -c "
import json
import time
import sys

cache_file = '$cache_file'
key = '$key'
ttl = $CACHE_TTL_SECONDS

try:
    with open(cache_file) as f:
        cache = json.load(f)
    
    if key not in cache:
        sys.exit(1)
    
    entry = cache[key]
    if time.time() - entry.get('timestamp', 0) > ttl:
        sys.exit(1)
    
    # Return cached result
    if entry.get('accessible', False):
        sys.exit(0)
    else:
        sys.exit(1)
except Exception:
    sys.exit(1)
" 2>/dev/null
}

# Update cache with result
update_cache() {
  local cache_file="$1"
  local key="$2"
  local accessible="$3"  # true or false
  
  python3 -c "
import json
import time
import sys

cache_file = '$cache_file'
key = '$key'
accessible = '$accessible' == 'true'

try:
    with open(cache_file) as f:
        cache = json.load(f)
except (FileNotFoundError, json.JSONDecodeError):
    cache = {}

cache[key] = {
    'accessible': accessible,
    'timestamp': time.time()
}

with open(cache_file, 'w') as f:
    json.dump(cache, f, indent=2)
" 2>/dev/null || true
}

# Check remote accessibility with caching (Req 19.1-19.4)
check_remote_accessible() {
  local remote_url="$1"
  local cache_key="remote:$remote_url"
  
  # Check cache first (Req 19.4)
  if is_cache_valid "$REMOTE_CACHE_FILE" "$cache_key"; then
    return 0
  fi
  
  # Try to reach remote (Req 19.1, 19.2)
  if git ls-remote --exit-code "$remote_url" HEAD >/dev/null 2>&1; then
    update_cache "$REMOTE_CACHE_FILE" "$cache_key" "true"
    return 0
  else
    update_cache "$REMOTE_CACHE_FILE" "$cache_key" "false"
    return 1
  fi
}

# ============================================================
# Common Checks (All Types)
# ============================================================

echo "[preflight] type=$REPO_TYPE path=$REPO_PATH"

echo "[preflight] git status (root)"
if [[ -n "$(git status --porcelain)" ]]; then
  echo "ERROR: root working tree not clean. Commit/stash first." >&2
  git status --porcelain >&2 || true
  exit 2
fi

# ============================================================
# Type-Specific Checks
# ============================================================

case "$REPO_TYPE" in
  root)
    # Root type: full submodule checks (Req 7.1-7.5)
    echo "[preflight] submodule status"
    git submodule status --recursive || true

    echo "[preflight] init submodules"
    git submodule sync --recursive
    git submodule update --init --recursive

    echo "[preflight] verify submodule working trees clean + pinned SHAs exist on origin"
    paths="$(git config -f .gitmodules --get-regexp path 2>/dev/null | awk '{print $2}' || true)"
    for p in $paths; do
      echo "  - $p"
      if [[ -n "$(git -C "$p" status --porcelain 2>/dev/null || true)" ]]; then
        echo "ERROR: submodule '$p' working tree not clean." >&2
        git -C "$p" status --porcelain >&2 || true
        exit 2
      fi

      sha="$(git -C "$p" rev-parse HEAD)"
      if ! git -C "$p" fetch -q origin "$sha" --depth=1 2>/dev/null; then
        echo "ERROR: submodule '$p' pinned sha '$sha' not found on origin (not our ref / missing commit)." >&2
        echo "HINT: push the commit to origin or update root to a reachable commit (merged into integration branch)." >&2
        exit 2
      fi
    done
    ;;

  directory)
    # Directory type: verify path exists (Req 7.6)
    echo "[preflight] verify directory path exists"
    if [[ ! -d "$ROOT/$REPO_PATH" ]]; then
      echo "ERROR: directory path '$REPO_PATH' does not exist." >&2
      exit 2
    fi
    ;;

  submodule)
    # Submodule type: specific checks (Req 7.3, 7.4, 7.5, 19.1-19.4, 23.1-23.4)
    echo "[preflight] submodule-specific checks for path=$REPO_PATH"
    
    # Verify submodule path exists
    if [[ ! -d "$ROOT/$REPO_PATH" ]]; then
      echo "ERROR: submodule path '$REPO_PATH' does not exist." >&2
      exit 2
    fi
    
    # Verify it's actually a submodule (has .git file or directory)
    if [[ ! -e "$ROOT/$REPO_PATH/.git" ]]; then
      echo "ERROR: '$REPO_PATH' is not a valid submodule (no .git)." >&2
      exit 2
    fi
    
    # Check submodule working tree is clean (Req 7.3)
    echo "[preflight] check submodule working tree clean"
    if [[ -n "$(git -C "$ROOT/$REPO_PATH" status --porcelain 2>/dev/null || true)" ]]; then
      echo "ERROR: submodule '$REPO_PATH' working tree not clean." >&2
      git -C "$ROOT/$REPO_PATH" status --porcelain >&2 || true
      exit 2
    fi
    
    # Check for detached HEAD state (Req 23.1-23.4)
    echo "[preflight] check submodule HEAD state"
    SUBMODULE_HEAD="$(git -C "$ROOT/$REPO_PATH" symbolic-ref -q HEAD 2>/dev/null || echo "DETACHED")"
    if [[ "$SUBMODULE_HEAD" == "DETACHED" ]]; then
      echo "[preflight] WARNING: submodule '$REPO_PATH' is in detached HEAD state." >&2
      echo "[preflight] This is normal for submodules. Will create branch during worktree setup." >&2
      # Note: This is informational, not an error (Req 23.2, 23.3)
    fi
    
    # Check remote accessibility (Req 19.1-19.4)
    echo "[preflight] check submodule remote accessibility"
    SUBMODULE_REMOTE="$(git -C "$ROOT/$REPO_PATH" remote get-url origin 2>/dev/null || true)"
    if [[ -n "$SUBMODULE_REMOTE" ]]; then
      if ! check_remote_accessible "$SUBMODULE_REMOTE"; then
        echo "ERROR: submodule '$REPO_PATH' remote '$SUBMODULE_REMOTE' is not accessible." >&2
        echo "HINT: check network connectivity or remote permissions." >&2
        exit 2
      fi
      echo "[preflight] submodule remote accessible: $SUBMODULE_REMOTE"
    else
      echo "WARNING: submodule '$REPO_PATH' has no origin remote configured." >&2
    fi
    
    # Verify pinned SHA exists on origin (Req 7.4, 7.5)
    echo "[preflight] verify submodule pinned SHA on origin"
    sha="$(git -C "$ROOT/$REPO_PATH" rev-parse HEAD)"
    if ! git -C "$ROOT/$REPO_PATH" fetch -q origin "$sha" --depth=1 2>/dev/null; then
      echo "ERROR: submodule '$REPO_PATH' pinned sha '$sha' not found on origin." >&2
      echo "HINT: push the commit to origin or update root to a reachable commit." >&2
      exit 2
    fi
    ;;

  *)
    echo "ERROR: unknown repo type '$REPO_TYPE'. Expected: root, directory, submodule." >&2
    exit 2
    ;;
esac

echo "[preflight] ok"
