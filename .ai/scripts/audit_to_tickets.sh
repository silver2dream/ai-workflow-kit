#!/usr/bin/env bash
set -euo pipefail

MONO_ROOT="$(git rev-parse --show-toplevel)"
AI_ROOT="$MONO_ROOT/.ai"
STATE_DIR="$AI_ROOT/state"
CONFIG_FILE="$AI_ROOT/config/workflow.yaml"
QUEUE_DIR="$MONO_ROOT/tasks/queue"
mkdir -p "$STATE_DIR" "$QUEUE_DIR"

if [[ ! -f "$STATE_DIR/audit.json" ]]; then
  bash "$AI_ROOT/scripts/audit_project.sh" >/dev/null
fi

python3 - "$CONFIG_FILE" <<'PY'
import json, os, re, glob, subprocess, sys
import yaml

config_file = sys.argv[1]
root = subprocess.check_output(["git","rev-parse","--show-toplevel"], text=True).strip()
ai_root = os.path.join(root, ".ai")
state_dir = os.path.join(ai_root, "state")
queue_dir = os.path.join(root, "tasks", "queue")
os.makedirs(queue_dir, exist_ok=True)

# Load config
with open(config_file, 'r', encoding='utf-8') as f:
    config = yaml.safe_load(f)

integration_branch = config['git']['integration_branch']
specs_base = config['specs']['base_path']
active_specs = config['specs'].get('active', [])
repos = {r['name']: r for r in config['repos']}

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
    # Check against configured repos
    for name in repos.keys():
        if name.lower() in t:
            return name
    # Fallback heuristics
    if any(x in t for x in ["後端", "backend", "nakama", "mongo", "redis", "postgres", "go"]):
        return "backend" if "backend" in repos else "root"
    if any(x in t for x in ["前端", "frontend", "unity", "ui", "uxml"]):
        return "frontend" if "frontend" in repos else "root"
    return "root"

def default_verification(repo_name):
    if repo_name in repos:
        r = repos[repo_name]
        return f"- Build: `{r['verify']['build']}`\n- Test: `{r['verify']['test']}`"
    return "- `git status --porcelain` is clean after commit"

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
"- obey `.ai/rules/git-workflow.md`\n"
"- obey routed architecture rules in `.ai/rules/`\n\n"
"## Plan\n"
"1) Read relevant rules and existing code paths\n"
"2) Make minimal change that satisfies acceptance criteria\n"
"3) Add/adjust tests if applicable\n"
"4) Run verification commands\n\n"
"## Verification\n"
f"{default_verification(repo)}\n\n"
"## Acceptance Criteria\n"
"- [ ] Implementation matches objective and scope\n"
"- [ ] Verification commands executed and pass\n"
"- [ ] Commit message uses `[type] subject` (lowercase)\n"
f"- [ ] PR created to `{integration_branch}` with body containing `Closes #{issue_id}`\n"
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
        if repo not in repos and repo != "root":
            repo = "root"
        title = f"[fix] {f.get('title','').lower()}"
        objective = f"Resolve audit finding {f.get('id')}: {f.get('title')}."
        detail = (f.get("detail") or "").strip() or "(no detail)"
        scope = f"- Fix the issue described below.\n\nDetails:\n{detail}"
        nong = "- No unrelated refactors.\n- Do not change public APIs unless required."
        created.append(write_ticket(issue_id, title, repo, f.get("severity","P1"), f"audit:{f.get('id')}", objective, scope, nong))
        issue_id += 1
else:
    # Read from active specs
    for spec_name in active_specs:
        tasks_path = os.path.join(root, specs_base, spec_name, "tasks.md")
        if not os.path.exists(tasks_path):
            print(f"WARN: tasks.md not found for spec '{spec_name}': {tasks_path}", file=sys.stderr)
            continue

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
                    scope = "- Implement only this subtask.\n- Keep changes minimal and focused."
                    nong = "- Do not implement other subtasks unless required."
                    created.append(write_ticket(issue_id, t, repo, "P2", f"tasks.md:{num}", objective, scope, nong))
                    issue_id += 1
            else:
                t = f"[feat] task {num}: {title.lower()}"
                objective = f"Implement task {num}: {title}."
                scope = "- Implement the task as described in tasks.md."
                nong = "- No unrelated refactors."
                created.append(write_ticket(issue_id, t, repo, "P2", f"tasks.md:{num}", objective, scope, nong))
                issue_id += 1

if created:
    print("\n".join(created))
else:
    print("No tickets created.")
PY
