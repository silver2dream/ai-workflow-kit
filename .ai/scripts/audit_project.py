#!/usr/bin/env python3
"""
audit_project.py - Cross-platform project auditor

Usage:
    python3 .ai/scripts/audit_project.py [--json]
"""

import os
import sys
import json
import subprocess
import re
from pathlib import Path
from typing import List, Dict, Any

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

def check_file_exists(root: Path, path: str) -> bool:
    """Check if file exists."""
    return (root / path).exists()

def check_dir_exists(root: Path, path: str) -> bool:
    """Check if directory exists."""
    return (root / path).is_dir()

def audit_project(root: Path) -> Dict[str, Any]:
    """Audit project for issues."""
    findings = []
    
    # P0: Critical files
    critical_files = [
        ('.ai/config/workflow.yaml', 'Workflow configuration missing'),
        ('CLAUDE.md', 'CLAUDE.md missing'),
        ('AGENTS.md', 'AGENTS.md missing'),
    ]
    
    for path, msg in critical_files:
        if not check_file_exists(root, path):
            findings.append({
                "severity": "P0",
                "type": "missing_file",
                "path": path,
                "message": msg
            })
    
    # P1: Important directories
    important_dirs = [
        ('.ai/scripts', 'Scripts directory missing'),
        ('.ai/rules/_kit', 'Kit rules directory missing'),
    ]
    
    for path, msg in important_dirs:
        if not check_dir_exists(root, path):
            findings.append({
                "severity": "P1",
                "type": "missing_dir",
                "path": path,
                "message": msg
            })
    
    # P1: Check git status
    try:
        result = subprocess.run(
            ['git', 'status', '--porcelain'],
            capture_output=True, text=True, cwd=root
        )
        if result.stdout.strip():
            findings.append({
                "severity": "P1",
                "type": "dirty_worktree",
                "path": str(root),
                "message": "Working tree has uncommitted changes"
            })
    except subprocess.CalledProcessError:
        pass
    
    # P2: Check for TODO/FIXME in code
    code_extensions = ['.go', '.py', '.ts', '.js', '.cs']
    todo_count = 0
    
    for ext in code_extensions:
        for file_path in root.rglob(f'*{ext}'):
            # Skip node_modules, vendor, etc.
            if any(skip in str(file_path) for skip in ['node_modules', 'vendor', '.git', '.worktrees']):
                continue
            try:
                with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                    content = f.read()
                    todo_count += len(re.findall(r'\b(TODO|FIXME|HACK)\b', content))
            except:
                pass
    
    if todo_count > 10:
        findings.append({
            "severity": "P2",
            "type": "code_quality",
            "path": "",
            "message": f"Found {todo_count} TODO/FIXME/HACK comments"
        })
    
    return {
        "root": str(root),
        "findings": findings,
        "summary": {
            "p0": len([f for f in findings if f["severity"] == "P0"]),
            "p1": len([f for f in findings if f["severity"] == "P1"]),
            "p2": len([f for f in findings if f["severity"] == "P2"]),
            "total": len(findings)
        }
    }

def main():
    output_json = '--json' in sys.argv
    root = get_repo_root()
    
    result = audit_project(root)
    
    if output_json:
        print(json.dumps(result, indent=2))
    else:
        print("=" * 60)
        print("Project Audit Report")
        print("=" * 60)
        print(f"Root: {result['root']}")
        print()
        
        if result['findings']:
            for finding in result['findings']:
                severity = finding['severity']
                icon = {'P0': '🔴', 'P1': '🟠', 'P2': '🟡'}.get(severity, '⚪')
                print(f"{icon} [{severity}] {finding['message']}")
                if finding['path']:
                    print(f"   Path: {finding['path']}")
        else:
            print("✅ No issues found")
        
        print()
        print("-" * 60)
        s = result['summary']
        print(f"Summary: {s['p0']} P0, {s['p1']} P1, {s['p2']} P2 ({s['total']} total)")
        
        if s['p0'] > 0:
            print("\n⚠️  P0 issues must be fixed before proceeding!")
            sys.exit(1)

if __name__ == '__main__':
    main()
