#!/usr/bin/env bash
set -euo pipefail

MONO_ROOT="$(git rev-parse --show-toplevel)"
STATE_DIR="$MONO_ROOT/.ai/state"
mkdir -p "$STATE_DIR"

if [[ ! -f "$STATE_DIR/repo_scan.json" ]]; then
  bash "$MONO_ROOT/.ai/scripts/scan_repo.sh" >/dev/null
fi

python3 - <<'PY'
import json, os, subprocess, time

root = subprocess.check_output(["git","rev-parse","--show-toplevel"], text=True).strip()
scan_path = os.path.join(root, ".ai", "state", "repo_scan.json")
scan = json.load(open(scan_path, "r", encoding="utf-8"))

def add(findings, sev, title, detail="", repo="root", fid=None):
    if fid is None:
        fid = f"F{len(findings)+1:03d}"
    findings.append({
        "id": fid,
        "severity": sev,
        "repo": repo,
        "title": title,
        "detail": (detail or "").strip(),
    })

findings = []

# P0: dirty working trees
if not scan["root"]["clean"]:
    add(findings, "P0", "root working tree not clean", scan["root"]["status"], "root")

for sm in scan["submodules"]:
    if sm.get("exists") and not sm.get("clean", True):
        add(findings, "P0", f"submodule '{sm['path']}' working tree not clean", "", sm["path"].split("/")[0])

# P0: pinned sha not fetchable (not our ref)
for sm in scan["submodules"]:
    if not sm.get("exists"):
        continue
    p = os.path.join(root, sm["path"])
    sha = sm.get("head")
    if not sha:
        continue
    rc = subprocess.call(["git","-C",p,"fetch","-q","origin",sha,"--depth=1"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if rc != 0:
        add(findings, "P0", f"submodule '{sm['path']}' pinned sha not found on origin",
            f"sha: {sha}\nFix: push/restore the commit on origin, or update root to a reachable commit (merged into feat/aether).",
            sm["path"].split("/")[0])

# P1: missing validate-submodules in root
if not scan["presence"]["validate_submodules_workflow"]:
    add(findings, "P1", "missing validate-submodules workflow in root", "Expected .github/workflows/validate-submodules.yml", "root")

# P1/P2: test presence heuristics
for sm in scan["submodules"]:
    if not sm.get("exists"):
        continue
    repo = sm["path"].split("/")[0]
    if sm.get("kind") == "go" and int(sm.get("test_files", 0)) == 0:
        add(findings, "P1", "backend has zero *_test.go files (heuristic)",
            "Consider adding unit tests for critical modules or at least smoke tests.", repo)
    if sm.get("kind") == "unity" and not sm.get("test_folders_present", False):
        add(findings, "P2", "unity tests folder not detected (heuristic)",
            "Consider adding PlayMode/EditMode smoke tests for core systems.", repo)

# P2: workflows missing in submodules
for sm in scan["submodules"]:
    if sm.get("exists") and sm.get("has_workflows") is False:
        repo = sm["path"].split("/")[0]
        add(findings, "P2", f"no .github/workflows detected in submodule '{sm['path']}'",
            "Consider adding CI to enforce build/tests on PR.", repo)

audit = {
    "timestamp_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    "summary": {
        "p0": sum(1 for f in findings if f["severity"] == "P0"),
        "p1": sum(1 for f in findings if f["severity"] == "P1"),
        "p2": sum(1 for f in findings if f["severity"] == "P2"),
        "total": len(findings),
    },
    "findings": findings,
}

out_path = os.path.join(root, ".ai", "state", "audit.json")
open(out_path, "w", encoding="utf-8").write(json.dumps(audit, indent=2, ensure_ascii=False) + "\n")
print(out_path)
PY
