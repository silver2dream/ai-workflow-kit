#!/usr/bin/env bash
set -euo pipefail

MONO_ROOT="$(git rev-parse --show-toplevel)"
OUT_DIR="$MONO_ROOT/.ai/state"
mkdir -p "$OUT_DIR"

python3 - <<'PY'
import json, os, subprocess, time, re

root = subprocess.check_output(["git","rev-parse","--show-toplevel"], text=True).strip()

def sh(cmd, cwd=root, ok=True):
    try:
        return subprocess.check_output(cmd, cwd=cwd, text=True, stderr=subprocess.STDOUT).strip()
    except subprocess.CalledProcessError as e:
        if ok:
            return e.output.strip()
        raise

def is_clean(cwd):
    return sh(["git","status","--porcelain"], cwd=cwd) == ""

def submodule_paths():
    gm = os.path.join(root, ".gitmodules")
    if not os.path.exists(gm):
        return []
    paths=[]
    for line in open(gm, "r", encoding="utf-8", errors="ignore"):
        m=re.match(r"\s*path\s*=\s*(.+)\s*$", line)
        if m:
            paths.append(m.group(1).strip())
    return paths

scan = {
  "timestamp_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
  "root": {
    "path": root,
    "branch": sh(["git","rev-parse","--abbrev-ref","HEAD"]),
    "head": sh(["git","rev-parse","HEAD"]),
    "clean": is_clean(root),
    "status": sh(["git","status","-sb"]),
    "workflows": os.path.isdir(os.path.join(root, ".github", "workflows")),
  },
  "submodules": [],
  "presence": {
    "gitmodules": os.path.exists(os.path.join(root, ".gitmodules")),
    "claude_md": os.path.exists(os.path.join(root, "CLAUDE.md")),
    "agents_md": os.path.exists(os.path.join(root, "AGENTS.md")),
    "tasks_md": os.path.exists(os.path.join(root, "tasks.md")),
    "design_md": os.path.exists(os.path.join(root, "design.md")),
    "validate_submodules_workflow": os.path.exists(os.path.join(root, ".github", "workflows", "validate-submodules.yml")),
  }
}

for p in submodule_paths():
    sp = os.path.join(root, p)
    entry = {"path": p, "exists": os.path.isdir(sp)}
    if entry["exists"]:
        entry["clean"] = is_clean(sp)
        entry["branch"] = sh(["git","rev-parse","--abbrev-ref","HEAD"], cwd=sp)
        entry["head"] = sh(["git","rev-parse","HEAD"], cwd=sp)
        entry["has_workflows"] = os.path.isdir(os.path.join(sp, ".github", "workflows"))

        if os.path.exists(os.path.join(sp, "go.mod")):
            entry["kind"]="go"
            count = sh(["bash","-lc","ls -1 **/*_test.go 2>/dev/null | wc -l"], cwd=sp)
            entry["test_files"] = int(count.strip() or "0")
        elif os.path.exists(os.path.join(sp, "ProjectSettings")) or os.path.exists(os.path.join(sp, "Assets")):
            entry["kind"]="unity"
            entry["test_folders_present"] = any(os.path.isdir(os.path.join(sp, x)) for x in ["Assets/Tests","Assets/Editor/Tests"])
        else:
            entry["kind"]="unknown"
    scan["submodules"].append(entry)

out = os.path.join(root, ".ai", "state", "repo_scan.json")
os.makedirs(os.path.dirname(out), exist_ok=True)
open(out, "w", encoding="utf-8").write(json.dumps(scan, indent=2, ensure_ascii=False) + "\n")
print(out)
PY
