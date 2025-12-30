#!/usr/bin/env python3
"""
scan_repo.py - Cross-platform repository scanner

Usage:
    python3 .ai/scripts/scan_repo.py [--json]

Output: .ai/state/repo_scan.json (conforms to repo_scan.schema.json)
"""

import os
import sys
import json
import subprocess
import re
import time
from pathlib import Path

# Add scripts directory to Python path for lib imports
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if SCRIPT_DIR not in sys.path:
    sys.path.insert(0, SCRIPT_DIR)

from lib.errors import AWKError, ConfigError, handle_unexpected_error, print_error
from lib.logger import Logger, split_log_level

def get_repo_root() -> Path:
    """Get git repository root."""
    try:
        result = subprocess.run(
            ['git', 'rev-parse', '--show-toplevel'],
            capture_output=True, text=True, check=True
        )
        return Path(result.stdout.strip())
    except subprocess.CalledProcessError:
        return Path.cwd()

def sh(cmd, cwd=None, ok=True):
    """Run shell command and return output."""
    try:
        result = subprocess.run(
            cmd, cwd=cwd, capture_output=True, text=True, check=True
        )
        return result.stdout.strip()
    except subprocess.CalledProcessError as e:
        if ok:
            return e.stdout.strip() if e.stdout else ''
        raise

def is_clean(cwd) -> bool:
    """Check if working tree is clean."""
    return sh(['git', 'status', '--porcelain'], cwd=cwd) == ''

def get_submodule_paths(root: Path) -> list:
    """Parse .gitmodules for submodule paths."""
    gitmodules = root / '.gitmodules'
    if not gitmodules.exists():
        return []
    paths = []
    with open(gitmodules, 'r', encoding='utf-8', errors='ignore') as f:
        for line in f:
            m = re.match(r'\s*path\s*=\s*(.+)\s*$', line)
            if m:
                paths.append(m.group(1).strip())
    return paths

def scan_repo(root: Path) -> dict:
    """Scan repository structure. Output conforms to repo_scan.schema.json."""
    result = {
        "timestamp_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "root": {
            "path": str(root),
            "branch": sh(['git', 'rev-parse', '--abbrev-ref', 'HEAD'], cwd=root),
            "head": sh(['git', 'rev-parse', 'HEAD'], cwd=root),
            "clean": is_clean(root),
            "status": sh(['git', 'status', '-sb'], cwd=root),
            "workflows": (root / '.github' / 'workflows').is_dir()
        },
        "submodules": [],
        "presence": {
            "gitmodules": (root / '.gitmodules').exists(),
            "claude_md": (root / 'CLAUDE.md').exists(),
            "agents_md": (root / 'AGENTS.md').exists(),
            "tasks_md": (root / 'tasks.md').exists(),
            "design_md": (root / 'design.md').exists(),
            "validate_submodules_workflow": (root / '.github' / 'workflows' / 'validate-submodules.yml').exists()
        },
        "ai_config": {
            "exists": (root / '.ai').is_dir(),
            "workflow_yaml": (root / '.ai' / 'config' / 'workflow.yaml').exists(),
            "scripts_dir": (root / '.ai' / 'scripts').is_dir()
        }
    }
    
    # Scan submodules
    for p in get_submodule_paths(root):
        sp = root / p
        entry = {"path": p, "exists": sp.is_dir()}
        if entry["exists"]:
            entry["clean"] = is_clean(sp)
            entry["branch"] = sh(['git', 'rev-parse', '--abbrev-ref', 'HEAD'], cwd=sp)
            entry["head"] = sh(['git', 'rev-parse', 'HEAD'], cwd=sp)
            entry["has_workflows"] = (sp / '.github' / 'workflows').is_dir()
            
            # Detect kind
            if (sp / 'go.mod').exists():
                entry["kind"] = "go"
                # Count test files
                try:
                    count = len(list(sp.rglob('*_test.go')))
                    entry["test_files"] = count
                except:
                    entry["test_files"] = 0
            elif (sp / 'ProjectSettings').exists() or (sp / 'Assets').exists():
                entry["kind"] = "unity"
                entry["test_folders_present"] = any(
                    (sp / x).is_dir() for x in ['Assets/Tests', 'Assets/Editor/Tests']
                )
            else:
                entry["kind"] = "unknown"
        result["submodules"].append(entry)
    
    return result

def main():
    args = sys.argv[1:]
    log_level, args, log_error = split_log_level(args)
    if log_error:
        print_error(ConfigError(log_error))
        sys.exit(2)

    output_json = '--json' in args
    root = get_repo_root()
    logger = Logger("scan_repo", root / '.ai' / 'logs', level=log_level)

    try:
        logger.info("scan start", {"output_json": output_json})
        result = scan_repo(root)

        # Always write to state file
        state_dir = root / '.ai' / 'state'
        state_dir.mkdir(parents=True, exist_ok=True)
        state_file = state_dir / 'repo_scan.json'
        with open(state_file, 'w', encoding='utf-8') as f:
            json.dump(result, f, indent=2, ensure_ascii=True)
            f.write('\n')

        if output_json:
            print(json.dumps(result, indent=2, ensure_ascii=True))
        else:
            print(f"Repository: {result['root']['path']}")
            print(f"Branch: {result['root']['branch']}")
            print(f"Clean: {'yes' if result['root']['clean'] else 'no'}")
            print(f"Submodules: {len(result['submodules'])}")
            print(f"AI Config: {'yes' if result['ai_config']['exists'] else 'no'}")
            print(f"\nState saved to: {state_file}")
        logger.info("scan complete", {"state_file": str(state_file)})
    except AWKError as err:
        logger.error("scan failed", {"error": err.message})
        print_error(err)
        sys.exit(err.exit_code)
    except Exception as exc:
        err = handle_unexpected_error(exc)
        logger.error("scan failed", {"error": str(exc)})
        print_error(err)
        sys.exit(err.exit_code)

if __name__ == '__main__':
    main()
