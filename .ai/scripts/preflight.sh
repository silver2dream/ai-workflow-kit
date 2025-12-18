#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

echo "[preflight] git status (root)"
if [[ -n "$(git status --porcelain)" ]]; then
  echo "ERROR: root working tree not clean. Commit/stash first." >&2
  git status --porcelain >&2 || true
  exit 2
fi

echo "[preflight] submodule status"
git submodule status --recursive || true

echo "[preflight] init submodules"
git submodule sync --recursive
git submodule update --init --recursive

echo "[preflight] verify submodule working trees clean + pinned SHAs exist on origin"
paths="$(git config -f .gitmodules --get-regexp path | awk '{print $2}' || true)"
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

echo "[preflight] ok"
