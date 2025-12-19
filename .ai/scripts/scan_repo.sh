#!/usr/bin/env bash
# scan_repo.sh - Repository scanner (delegates to scan_repo.py for unified schema)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Delegate to Python for unified schema output
python3 "$SCRIPT_DIR/scan_repo.py" "$@"
