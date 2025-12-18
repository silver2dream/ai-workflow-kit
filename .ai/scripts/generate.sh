#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# generate.sh - 從模板生成 CLAUDE.md 和 AGENTS.md
# ============================================================================
# 用法:
#   bash .ai/scripts/generate.sh
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

CONFIG_FILE="$AI_ROOT/config/workflow.yaml"
TEMPLATES_DIR="$AI_ROOT/templates"

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "ERROR: Config file not found: $CONFIG_FILE"
  exit 1
fi

echo "[generate] Reading config from $CONFIG_FILE"

# 使用 Python 生成文件
python3 - "$CONFIG_FILE" "$TEMPLATES_DIR" "$MONO_ROOT" <<'PYTHON'
import sys
import yaml
import os

config_file = sys.argv[1]
templates_dir = sys.argv[2]
output_dir = sys.argv[3]

# 讀取配置
with open(config_file, 'r', encoding='utf-8') as f:
    config = yaml.safe_load(f)

# 檢查是否有 submodules
has_submodules = any(r.get('type') == 'submodule' for r in config.get('repos', []))

# 簡單的模板渲染（不依賴 jinja2）
def render_template(template_path, context):
    with open(template_path, 'r', encoding='utf-8') as f:
        content = f.read()
    
    # 簡單替換 {{ variable }}
    for key, value in context.items():
        if isinstance(value, str):
            content = content.replace('{{ ' + key + ' }}', value)
            content = content.replace('{{' + key + '}}', value)
    
    return content

def generate_claude_md():
    """生成 CLAUDE.md"""
    lines = []
    lines.append("# CLAUDE.md (Principal + Worker Orchestration)")
    lines.append("")
    lines.append("This file provides guidance for Claude Code (Principal) when working with this project.")
    lines.append("")
    lines.append("## Project Overview")
    lines.append("")
    lines.append(f"**Name:** {config['project']['name']}")
    lines.append(f"**Type:** {config['project']['type']}")
    repos_str = ", ".join(r['name'] for r in config['repos'])
    lines.append(f"**Repos:** {repos_str}")
    lines.append("")
    lines.append("## Rule Routing (IMPORTANT)")
    lines.append("")
    lines.append("Before coding, ALWAYS identify which area the task touches, then apply the corresponding rules:")
    lines.append("")
    
    for repo in config['repos']:
        lines.append(f"### {repo['name'].capitalize()} work")
        rules_str = ", ".join(f"`.ai/rules/{r}.md`" for r in repo['rules'])
        lines.append(f"- Follow: {rules_str}")
        lines.append("")
    
    lines.append("### Git & PR workflow (ALWAYS)")
    lines.append("- Follow: `.ai/rules/git-workflow.md` (commit format + PR base)")
    lines.append("")
    lines.append("---")
    lines.append("")
    lines.append("## Principal Autonomous Flow (MUST FOLLOW)")
    lines.append("")
    lines.append("### Phase A — Project Audit (BEFORE tasks)")
    lines.append("")
    lines.append("Run:")
    lines.append("```bash")
    lines.append("bash .ai/scripts/scan_repo.sh")
    lines.append("bash .ai/scripts/audit_project.sh")
    lines.append("```")
    lines.append("")
    lines.append("Read output:")
    lines.append("- `.ai/state/repo_scan.json`")
    lines.append("- `.ai/state/audit.json`")
    lines.append("")
    lines.append("Decision rule:")
    lines.append("- If audit has any **P0/P1** findings: create fix tickets FIRST.")
    lines.append("- Only if audit has no P0/P1 blockers, proceed to Phase B.")
    lines.append("")
    lines.append("### Phase B — Task Selection")
    lines.append("")
    lines.append("Read active specs:")
    for spec in config['specs']['active']:
        lines.append(f"- `{config['specs']['base_path']}/{spec}/tasks.md`")
    lines.append("")
    lines.append(f"Find uncompleted tasks (`{config['tasks']['format']['uncompleted']}`) and create GitHub Issues.")
    lines.append("")
    lines.append("### Phase C — Execution (Worker / Codex)")
    lines.append("")
    lines.append("Worker takes one ticket and runs:")
    lines.append("```bash")
    lines.append("bash .ai/scripts/run_issue_codex.sh <id> <ticket_file> <repo>")
    lines.append("```")
    lines.append("")
    lines.append("Worker MUST:")
    lines.append("1. Implement within scope")
    lines.append("2. Verify (commands listed in ticket)")
    lines.append(f"3. Commit using `{config['git']['commit_format']}` (lowercase)")
    lines.append("4. Push branch")
    lines.append(f"5. Create PR (base: `{config['git']['integration_branch']}`, release: `{config['git']['release_branch']}`)")
    lines.append("6. Write `.ai/results/issue-<id>.json` with PR URL")
    lines.append("")
    lines.append("### Phase D — Review")
    lines.append("")
    lines.append("Review PR using:")
    lines.append("```bash")
    lines.append("gh pr diff <PR_NUMBER>")
    lines.append("```")
    lines.append("")
    lines.append("Check:")
    lines.append(f"- Commit format: `{config['git']['commit_format']}`")
    lines.append(f"- PR base: `{config['git']['integration_branch']}`")
    lines.append("- Changes within scope")
    lines.append("- Architecture rules compliance")
    lines.append("")
    lines.append("---")
    lines.append("")
    lines.append("## Quick Reference")
    lines.append("")
    lines.append("### Start Work")
    lines.append("```bash")
    lines.append("bash .ai/scripts/kickoff.sh")
    lines.append("```")
    lines.append("")
    lines.append("### Check Status")
    lines.append("```bash")
    lines.append("bash .ai/scripts/stats.sh")
    lines.append("```")
    lines.append("")
    lines.append("### Stop Work")
    lines.append("```bash")
    lines.append("touch .ai/state/STOP")
    lines.append("```")
    lines.append("")
    lines.append("## File Locations")
    lines.append("")
    lines.append("| What | Where |")
    lines.append("|------|-------|")
    lines.append("| Config | `.ai/config/workflow.yaml` |")
    lines.append("| Scripts | `.ai/scripts/` |")
    lines.append("| Rules | `.ai/rules/` |")
    lines.append("| Commands | `.ai/commands/` |")
    lines.append(f"| Specs | `{config['specs']['base_path']}/` |")
    lines.append("| Results | `.ai/results/` |")
    
    return "\n".join(lines)

def generate_agents_md():
    """生成 AGENTS.md"""
    lines = []
    lines.append("# AGENTS.md (STRICT, LAZY, HIGH-CORRECTNESS)")
    lines.append("")
    lines.append("You are an engineering agent working in this repository.")
    lines.append("Default priority: correctness > minimal diff > speed.")
    lines.append("")
    lines.append("## MUST-READ (before any work)")
    lines.append("")
    
    seen_rules = set()
    for repo in config['repos']:
        for rule in repo['rules']:
            if rule not in seen_rules:
                seen_rules.add(rule)
                suffix = " (CRITICAL for commit format)" if rule == 'git-workflow' else ""
                lines.append(f"- Read and obey: `.ai/rules/{rule}.md`{suffix}")
    lines.append("")
    lines.append("Do not proceed if these files are missing—stop and report what you cannot find.")
    lines.append("")
    lines.append("---")
    lines.append("")
    lines.append("## NON-NEGOTIABLE HARD RULES")
    lines.append("")
    lines.append("### Start-of-work gate (MANDATORY)")
    lines.append("- At the start of each session, run project audit once before implementing tickets:")
    lines.append("  - `bash .ai/scripts/scan_repo.sh`")
    lines.append("  - `bash .ai/scripts/audit_project.sh`")
    lines.append("- If audit contains any P0/P1 findings, create/fix those tickets first.")
    lines.append("")
    lines.append("### 0. Use existing architecture & do not reinvent")
    lines.append("- Do not create parallel systems. Extend existing patterns.")
    lines.append("- Keep changes minimal. Avoid wide refactors.")
    lines.append("")
    lines.append("### 1. Always read before writing")
    lines.append("- Search the repo for the existing pattern before adding a new one.")
    lines.append("- Prefer local conventions (naming, folder structure, module boundaries).")
    lines.append("")
    lines.append("### 2. Tests & verification are part of the change")
    lines.append("- If changing logic: add/adjust tests when feasible.")
    lines.append("- If no tests exist: add at least a small smoke test or verification note.")
    lines.append("")
    
    rule_num = 3
    if has_submodules:
        lines.append(f"### {rule_num}. Submodule safety")
        lines.append("- Do NOT change submodule pinned commits unless ticket explicitly requires it.")
        lines.append("- If you are working in a submodule repo, the PR must be created in that repo.")
        lines.append("")
        rule_num += 1
    
    lines.append(f"### {rule_num}. Git discipline")
    lines.append("1. **Commit**: One commit per ticket unless the ticket explicitly needs multiple commits.")
    lines.append("2. **Commit message**: Must follow `.ai/rules/git-workflow.md`.")
    lines.append(f"   - Format: `{config['git']['commit_format']}`")
    lines.append(f"3. **PR**: Create PR targeting `{config['git']['integration_branch']}` with \"Closes #<IssueID>\" in the body.")
    lines.append(f"   - `{config['git']['release_branch']}` is release-only. Target `{config['git']['release_branch']}` ONLY when the ticket explicitly says `Release: true`.")
    lines.append("4. **No direct pushes** to protected branches.")
    lines.append("")
    lines.append("---")
    lines.append("")
    lines.append("## DEFAULT VERIFY COMMANDS")
    lines.append("")
    
    for repo in config['repos']:
        lines.append(f"### {repo['name']}")
        lines.append(f"- Build: `{repo['verify']['build']}`")
        lines.append(f"- Test: `{repo['verify']['test']}`")
        lines.append("")
    
    return "\n".join(lines)

# 生成文件
claude_md = generate_claude_md()
agents_md = generate_agents_md()

# 寫入文件
claude_path = os.path.join(output_dir, 'CLAUDE.md')
agents_path = os.path.join(output_dir, 'AGENTS.md')

with open(claude_path, 'w', encoding='utf-8') as f:
    f.write(claude_md)
print(f"[generate] Created: {claude_path}")

with open(agents_path, 'w', encoding='utf-8') as f:
    f.write(agents_md)
print(f"[generate] Created: {agents_path}")

# 創建符號連結 .claude/ -> .ai/（跨平台）
import shutil
import platform

ai_root = os.path.dirname(os.path.dirname(os.path.abspath(sys.argv[1])))
claude_dir = os.path.join(output_dir, '.claude')
os.makedirs(claude_dir, exist_ok=True)

ai_rules = os.path.join(ai_root, 'rules')
ai_commands = os.path.join(ai_root, 'commands')
claude_rules = os.path.join(claude_dir, 'rules')
claude_commands = os.path.join(claude_dir, 'commands')

def create_symlink(source, target, name):
    """創建符號連結，失敗時回退到複製"""
    # 如果目標已存在
    if os.path.islink(target):
        os.unlink(target)
    elif os.path.isdir(target):
        shutil.rmtree(target)
    
    try:
        # 使用相對路徑創建符號連結
        rel_source = os.path.relpath(source, os.path.dirname(target))
        os.symlink(rel_source, target, target_is_directory=True)
        print(f"[generate] Created symlink: {target} -> {rel_source}")
        return True
    except OSError as e:
        if platform.system() == 'Windows':
            print(f"[generate] WARNING: Cannot create symlink for {name}.")
            print(f"[generate] On Windows, enable Developer Mode or run as Administrator.")
            print(f"[generate] Settings -> Update & Security -> For developers -> Developer Mode: ON")
            print(f"[generate] Falling back to copy...")
        else:
            print(f"[generate] WARNING: Symlink failed for {name}: {e}")
            print(f"[generate] Falling back to copy...")
        
        # 回退到複製
        if os.path.exists(source):
            shutil.copytree(source, target)
            print(f"[generate] Copied {name} to: {target}")
        return False

# 創建符號連結
symlink_ok = True
if os.path.exists(ai_rules):
    if not create_symlink(ai_rules, claude_rules, 'rules'):
        symlink_ok = False

if os.path.exists(ai_commands):
    if not create_symlink(ai_commands, claude_commands, 'commands'):
        symlink_ok = False

if not symlink_ok:
    print(f"[generate] NOTE: Using file copy instead of symlinks.")
    print(f"[generate] Run 'bash .ai/scripts/generate.sh' after modifying .ai/rules/ or .ai/commands/")

print("[generate] Done!")
PYTHON
