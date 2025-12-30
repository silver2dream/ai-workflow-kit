#!/usr/bin/env bash
# Cross-platform timeout helpers for AWK bash scripts.
#
# Usage:
#   source ".ai/scripts/lib/timeout.sh"
#   run_with_timeout <seconds> <command> [args...]
#
# Env:
#   AI_GH_TIMEOUT   Default timeout for gh calls (seconds, default: 60)
#   AI_GIT_TIMEOUT  Default timeout for git calls (seconds, default: 120)
#   AI_CODEX_TIMEOUT Default timeout for codex exec (seconds, default: 1800)

set -u

_timeout_lib_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

_timeout_py() {
  local py="${_timeout_lib_dir}/run_with_timeout.py"

  if command -v python3 >/dev/null 2>&1; then
    python3 "$py" "$@"
    return $?
  fi
  if command -v python >/dev/null 2>&1; then
    python "$py" "$@"
    return $?
  fi

  echo "[TIMEOUT] python not found; running without timeout: $*" >&2
  shift
  "$@"
}

run_with_timeout() {
  local timeout_sec="${1:-0}"
  shift || true

  if [[ -z "$timeout_sec" || "$timeout_sec" == "0" ]]; then
    "$@"
    return $?
  fi

  _timeout_py "$timeout_sec" "$@"
  return $?
}

gh_with_timeout() {
  local timeout_sec="${AI_GH_TIMEOUT:-60}"
  run_with_timeout "$timeout_sec" gh "$@"
}

git_with_timeout() {
  local timeout_sec="${AI_GIT_TIMEOUT:-120}"
  run_with_timeout "$timeout_sec" git "$@"
}

codex_with_timeout() {
  local timeout_sec="${AI_CODEX_TIMEOUT:-1800}"
  run_with_timeout "$timeout_sec" codex "$@"
}

