#!/usr/bin/env python3
import sys
import os
import json
import subprocess
import time
from pathlib import Path

# Add scripts directory to Python path for lib imports
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if SCRIPT_DIR not in sys.path:
    sys.path.insert(0, SCRIPT_DIR)

from lib.errors import AWKError, ConfigError, handle_unexpected_error, print_error
from lib.logger import Logger, split_log_level

def get_repo_root():
    try:
        result = subprocess.run(['git', 'rev-parse', '--show-toplevel'], capture_output=True, text=True, check=True)
        return Path(result.stdout.strip())
    except: return Path.cwd()

def is_clean(cwd):
    try:
        result = subprocess.run(['git', 'status', '--porcelain'], capture_output=True, text=True, cwd=cwd)
        return result.stdout.strip() == ''
    except: return True

def audit_project(root):
    findings = []
    fid = 0
    def add(sev, ftype, path, msg):
        nonlocal fid
        fid += 1
        findings.append({"id": f"F{fid:03d}", "severity": sev, "type": ftype, "path": path, "message": msg})
    
    for p, m in [('.ai/config/workflow.yaml', 'Workflow config missing'), ('CLAUDE.md', 'CLAUDE.md missing'), ('AGENTS.md', 'AGENTS.md missing')]:
        if not (root / p).exists(): add("P0", "missing_file", p, m)
    for p, m in [('.ai/scripts', 'Scripts dir missing'), ('.ai/rules/_kit', 'Kit rules dir missing')]:
        if not (root / p).is_dir(): add("P1", "missing_dir", p, m)
    if not is_clean(root): add("P1", "dirty_worktree", str(root), "Working tree has uncommitted changes")
    
    # Submodule checks (Req 8.1-8.7)
    gitmodules_path = root / '.gitmodules'
    if gitmodules_path.exists():
        try:
            # Get submodule status
            result = subprocess.run(
                ['git', 'submodule', 'status', '--recursive'],
                capture_output=True, text=True, cwd=root, timeout=30
            )
            if result.returncode == 0:
                for line in result.stdout.strip().split('\n'):
                    if not line.strip():
                        continue
                    # Parse submodule status line: [+-U ]<sha> <path> (<describe>)
                    # - = not initialized, + = different commit, U = merge conflict
                    status_char = line[0] if line else ' '
                    parts = line[1:].strip().split()
                    if len(parts) >= 2:
                        submodule_path = parts[1]
                        
                        # Check for uninitialized submodule (Req 8.1, 8.2)
                        if status_char == '-':
                            add("P1", "submodule_uninitialized", submodule_path, 
                                f"Submodule '{submodule_path}' is not initialized")
                        
                        # Check for different commit (potential unpushed changes) (Req 8.5)
                        elif status_char == '+':
                            add("P2", "submodule_modified", submodule_path,
                                f"Submodule '{submodule_path}' has different commit than recorded")
            
            # Check each submodule for dirty working tree (Req 8.3, 8.4)
            with open(gitmodules_path, 'r', encoding='utf-8') as f:
                import re
                submodule_paths = re.findall(r'path\s*=\s*(.+)', f.read())
                submodule_paths = [p.strip() for p in submodule_paths]
            
            for submodule_path in submodule_paths:
                full_path = root / submodule_path
                if full_path.exists() and (full_path / '.git').exists():
                    # Check if submodule working tree is dirty (Req 8.3, 8.4)
                    if not is_clean(full_path):
                        add("P1", "submodule_dirty", submodule_path,
                            f"Submodule '{submodule_path}' has uncommitted changes")
                    
                    # Check for unpushed commits (Req 8.6, 8.7)
                    try:
                        result = subprocess.run(
                            ['git', 'log', '--oneline', '@{u}..HEAD'],
                            capture_output=True, text=True, cwd=full_path, timeout=10
                        )
                        if result.returncode == 0 and result.stdout.strip():
                            unpushed_count = len(result.stdout.strip().split('\n'))
                            add("P2", "submodule_unpushed", submodule_path,
                                f"Submodule '{submodule_path}' has {unpushed_count} unpushed commit(s)")
                    except subprocess.TimeoutExpired:
                        pass
                    except Exception:
                        pass  # No upstream configured, skip
        except subprocess.TimeoutExpired:
            add("P2", "submodule_timeout", ".gitmodules", "Submodule status check timed out")
        except Exception as e:
            add("P2", "submodule_error", ".gitmodules", f"Submodule check failed: {str(e)}")
    
    return {"timestamp_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "root": str(root), "findings": findings,
            "summary": {"p0": len([f for f in findings if f["severity"]=="P0"]), "p1": len([f for f in findings if f["severity"]=="P1"]),
                        "p2": len([f for f in findings if f["severity"]=="P2"]), "total": len(findings)}}

def main():
    args = sys.argv[1:]
    log_level, args, log_error = split_log_level(args)
    if log_error:
        print_error(ConfigError(log_error))
        sys.exit(2)
    output_json = '--json' in args

    root = get_repo_root()
    logger = Logger("audit_project", root / '.ai' / 'logs', level=log_level)

    try:
        logger.info("audit start", {"output_json": output_json})
        result = audit_project(root)
        state_dir = root / '.ai' / 'state'
        state_dir.mkdir(parents=True, exist_ok=True)
        state_file = state_dir / 'audit.json'
        with open(state_file, 'w', encoding='utf-8') as f:
            json.dump(result, f, indent=2, ensure_ascii=True)
            f.write('\n')

        if output_json:
            print(json.dumps(result, indent=2, ensure_ascii=True))
        else:
            print("=" * 60 + "\nProject Audit Report\n" + "=" * 60)
            for f in result['findings']:
                print(f"[{f['severity']}] {f['message']}")
            s = result['summary']
            print(f"\nSummary: {s['p0']} P0, {s['p1']} P1, {s['p2']} P2")
        logger.info("audit complete", {"state_file": str(state_file)})

        if result["summary"]["p0"] > 0:
            sys.exit(1)
    except AWKError as err:
        logger.error("audit failed", {"error": err.message})
        print_error(err)
        sys.exit(err.exit_code)
    except Exception as exc:
        err = handle_unexpected_error(exc)
        logger.error("audit failed", {"error": str(exc)})
        print_error(err)
        sys.exit(err.exit_code)

if __name__ == '__main__': main()
