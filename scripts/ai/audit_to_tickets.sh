#!/usr/bin/env bash
set -euo pipefail

MONO_ROOT="$(git rev-parse --show-toplevel)"
STATE_DIR="$MONO_ROOT/.ai/state"
QUEUE_DIR="$MONO_ROOT/tasks/queue"
mkdir -p "$STATE_DIR" "$QUEUE_DIR"

if [[ ! -f "$STATE_DIR/audit.json" ]]; then
  bash "$MONO_ROOT/scripts/ai/audit_project.sh" >/dev/null
fi

python3 - <<'PY'
import json, os, re, glob, subprocess

root = subprocess.check_output(["git","rev-parse","--show-toplevel"], text=True).strip()
state_dir = os.path.join(root, ".ai", "state")
queue_dir = os.path.join(root, "tasks", "queue")
os.makedirs(queue_dir, exist_ok=True)

audit = json.load(open(os.path.join(state_dir, "audit.json"), "r", encoding="utf-8"))

def next_issue_id():
    ids=[]
    for p in glob.glob(os.path.join(queue_dir, "issue-*.md")):
        m=re.search(r"issue-(\d+)\.md$", p)
        if m:
            ids.append(int(m.group(1)))
    return (max(ids) + 1) if ids else 1

def repo_from_text(title):
    t=title.lower()
    if ("後端" in title) or ("backend" in t) or ("nakama" in t) or ("mongo" in t) or ("redis" in t) or ("postgres" in t):
        return "backend"
    if ("前端" in title) or ("frontend" in t) or ("unity" in t) or ("ui" in t) or ("uxml" in t) or ("uitoolkit" in t):
        return "frontend"
    return "root"

def default_verification(repo):
    if repo == "backend":
        return "- `go test ./...` (or targeted module tests if specified)\n- `go vet ./...` (optional)"
    if repo == "frontend":
        return "- Ensure Unity compiles (open project, no console errors)\n- (If available) run Unity Test Runner (EditMode/PlayMode smoke tests)"
    return "- `git status --porcelain` is clean after commit\n- `git submodule status --recursive`"

def write_ticket(issue_id, title, repo, severity, source, objective, scope, nong):
    body = (
f"# {title}\n\n"
f"- Repo: {repo}\n"
f"- Severity: {severity}\n"
f"- Source: {source}\n"
f"- Release: false\n\n"
"## Objective\n"
f"{objective}\n\n"
"## Scope\n"
f"{scope}\n\n"
"## Non-goals\n"
f"{nong}\n\n"
"## Constraints\n"
"- obey AGENTS.md\n"
"- obey `.claude/rules/git-workflow.md`\n"
"- obey routed architecture rules:\n"
"  - backend: `.claude/rules/backend-nakama-architecture-and-patterns.md`\n"
"  - frontend/unity: `.claude/rules/unity-architecture-and-patterns.md`\n"
"  - ui toolkit conversion: `.claude/rules/ui-toolkit-react-to-uxml.md`\n\n"
"## Plan\n"
"1) Read relevant rules and existing code paths\n"
"2) Make minimal change that satisfies acceptance criteria\n"
"3) Add/adjust tests if applicable (prefer small unit tests / smoke tests)\n"
"4) Run verification commands\n\n"
"## Verification\n"
f"{default_verification(repo)}\n\n"
"## Acceptance Criteria\n"
"- [ ] Implementation matches objective and scope\n"
"- [ ] Verification commands executed and pass\n"
"- [ ] Commit message uses `[type] subject` (lowercase)\n"
f"- [ ] PR created to `feat/aether` with body containing `Closes #{issue_id}`\n"
    )
    out = os.path.join(queue_dir, f"issue-{issue_id}.md")
    with open(out, "w", encoding="utf-8") as f:
        f.write(body)
    return out

p0p1 = [f for f in audit.get("findings", []) if f.get("severity") in ("P0","P1")]
created=[]
issue_id = next_issue_id()

if p0p1:
    sev_rank={"P0":0,"P1":1,"P2":2}
    for f in sorted(p0p1, key=lambda x: sev_rank.get(x.get("severity","P2"), 9)):
        repo = f.get("repo","root")
        if repo not in ("root","backend","frontend"):
            repo = "root"
        title = f"[fix] {f.get('title','').lower()}"
        objective = f"Resolve audit finding {f.get('id')}: {f.get('title')}."
        detail = (f.get("detail") or "").strip() or "(no detail)"
        scope = f"- Fix the issue described below.\n\nDetails:\n{detail}"
        nong = "- No unrelated refactors.\n- Do not change public APIs unless required."
        created.append(write_ticket(issue_id, title, repo, f.get("severity","P1"), f"audit:{f.get('id')}", objective, scope, nong))
        issue_id += 1
else:
    tasks_path = os.path.join(root, ".kiro\specs\cultivation-mvp\tasks.md")
    if not os.path.exists(tasks_path):
        raise SystemExit("tasks.md not found")

    lines = open(tasks_path, "r", encoding="utf-8", errors="ignore").read().splitlines()

    task_blocks=[]
    i=0
    while i < len(lines):
        line=lines[i]
        m=re.match(r"^- \[\s\]\s*(\d+)\.\s*(.+)$", line)
        if m:
            num=int(m.group(1))
            title=m.group(2).strip()
            j=i+1
            bullets=[]
            while j < len(lines):
                if re.match(r"^- \[[ xX]\]\s*\d+\.", lines[j]):
                    break
                bm=re.match(r"^\s{2,}-\s+(.+)$", lines[j])
                if bm:
                    bullets.append(bm.group(1).strip())
                j+=1
            task_blocks.append((num,title,bullets))
            i=j
        else:
            i+=1

    for num,title,bullets in task_blocks:
        repo = repo_from_text(title)
        if bullets:
            for b in bullets:
                t = f"[feat] task {num}: {b.lower()}"
                objective = f"Implement task {num} subtask: {b}."
                scope = "- Implement only this subtask.\n- Keep changes minimal and focused.\n- Update docs if needed."
                nong = "- Do not implement other subtasks unless required by this one."
                created.append(write_ticket(issue_id, t, repo, "P2", f"tasks.md:{num}", objective, scope, nong))
                issue_id += 1
        else:
            t = f"[feat] task {num}: {title.lower()}"
            objective = f"Implement task {num}: {title}."
            scope = "- Implement the task as described in tasks.md.\n- Keep changes minimal and focused."
            nong = "- No unrelated refactors."
            created.append(write_ticket(issue_id, t, repo, "P2", f"tasks.md:{num}", objective, scope, nong))
            issue_id += 1

print("\n".join(created))
PY
