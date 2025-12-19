#!/usr/bin/env bash
# audit_project.sh - Project auditor (delegates to audit_project.py for unified schema)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Delegate to Python for unified schema output
python3 "$SCRIPT_DIR/audit_project.py" "$@"
