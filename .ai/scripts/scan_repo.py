#!/usr/bin/env python3
"""
scan_repo.py - Cross-platform repository scanner

Usage:
    python3 .ai/scripts/scan_repo.py [--json]
"""

import os
import sys
import json
import subprocess
from pathlib import Path

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

def scan_repo(root: Path) -> dict:
    """Scan repository structure."""
    result = {
        "root": str(root),
        "submodules": [],
        "repos": [],
        "ai_config": {
            "exists": False,
            "workflow_yaml": False,
            "scripts_dir": False
        }
    }
    
    # Check .gitmodules
    gitmodules = root / '.gitmodules'
    if gitmodules.exists():
        with open(gitmodules, 'r', encoding='utf-8') as f:
            content = f.read()
            for line in content.split('\n'):
                if line.strip().startswith('path = '):
                    path = line.split('=')[1].strip()
                    result["submodules"].append(path)
    
    # Check for repo directories
    for item in ['backend', 'frontend']:
        item_path = root / item
        if item_path.is_dir() and (item_path / '.git').exists():
            result["repos"].append(item)
    
    # Check AI config
    ai_dir = root / '.ai'
    if ai_dir.is_dir():
        result["ai_config"]["exists"] = True
        result["ai_config"]["workflow_yaml"] = (ai_dir / 'config' / 'workflow.yaml').exists()
        result["ai_config"]["scripts_dir"] = (ai_dir / 'scripts').is_dir()
    
    return result

def main():
    output_json = '--json' in sys.argv
    root = get_repo_root()
    
    result = scan_repo(root)
    
    # Always write to state file for consistency with .sh version
    state_dir = root / '.ai' / 'state'
    state_dir.mkdir(parents=True, exist_ok=True)
    state_file = state_dir / 'repo_scan.json'
    with open(state_file, 'w', encoding='utf-8') as f:
        json.dump(result, f, indent=2)
    
    if output_json:
        print(json.dumps(result, indent=2))
    else:
        print(f"Repository: {result['root']}")
        print(f"Submodules: {', '.join(result['submodules']) or 'none'}")
        print(f"Repos: {', '.join(result['repos']) or 'none'}")
        print(f"AI Config: {'yes' if result['ai_config']['exists'] else 'no'}")
        if result['ai_config']['exists']:
            print(f"  - workflow.yaml: {'yes' if result['ai_config']['workflow_yaml'] else 'no'}")
            print(f"  - scripts dir: {'yes' if result['ai_config']['scripts_dir'] else 'no'}")
        print(f"\nState saved to: {state_file}")

if __name__ == '__main__':
    main()
