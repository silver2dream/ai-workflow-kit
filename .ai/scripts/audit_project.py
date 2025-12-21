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
