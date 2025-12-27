#!/usr/bin/env bash
# Cross-platform hash helpers for AWK bash scripts.
#
# Usage:
#   source ".ai/scripts/lib/hash.sh"
#   sha256_16 < input
#
# Output:
#   Writes the first 16 hex chars of sha256(stdin) to stdout.

set -u

sha256_16() {
  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY'
import hashlib
import sys

data = sys.stdin.buffer.read()
sys.stdout.write(hashlib.sha256(data).hexdigest()[:16])
PY
    return 0
  fi

  if command -v python >/dev/null 2>&1; then
    python - <<'PY'
import hashlib
import sys

data = sys.stdin.read()
if isinstance(data, str):
    try:
        data = data.encode("utf-8")
    except Exception:
        pass
sys.stdout.write(hashlib.sha256(data).hexdigest()[:16])
PY
    return 0
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}' | cut -c1-16
    return 0
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}' | cut -c1-16
    return 0
  fi

  echo "[HASH] no sha256 tool found" >&2
  return 1
}

