#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# generate.sh - 從模板生成所有配置文件
# ============================================================================
# 用法:
#   bash .ai/scripts/generate.sh
#
# 生成:
#   - CLAUDE.md (Principal 指南)
#   - AGENTS.md (Worker 指南)
#   - .github/workflows/ci.yml (每個 repo)
#   - .github/workflows/validate-submodules.yml (monorepo)
#   - .claude/rules/ 和 .claude/commands/ 符號連結
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
echo "[generate] Templates dir: $TEMPLATES_DIR"

# 驗證配置
echo "[generate] Validating config..."
if ! python3 "$AI_ROOT/scripts/validate_config.py" "$CONFIG_FILE"; then
  echo "[generate] ERROR: Config validation failed"
  exit 1
fi

# 檢查 jinja2 是否安裝
if ! python3 -c "import jinja2" 2>/dev/null; then
  echo "[generate] Installing jinja2..."
  pip3 install jinja2 pyyaml --quiet
fi

# 使用 Python + Jinja2 生成文件
python3 - "$CONFIG_FILE" "$TEMPLATES_DIR" "$MONO_ROOT" <<'PYTHON'
import sys
import os
import yaml
import shutil
import platform
from jinja2 import Environment, FileSystemLoader

config_file = sys.argv[1]
templates_dir = sys.argv[2]
output_dir = sys.argv[3]

# 讀取配置
with open(config_file, 'r', encoding='utf-8') as f:
    config = yaml.safe_load(f)

# 設置 Jinja2 環境
env = Environment(
    loader=FileSystemLoader(templates_dir),
    trim_blocks=True,
    lstrip_blocks=True
)

# 檢查 repo 類型
has_submodules = any(r.get('type') == 'submodule' for r in config.get('repos', []))
has_directories = any(r.get('type') == 'directory' for r in config.get('repos', []))
is_single_repo = config['project']['type'] == 'single-repo' or any(r.get('type') == 'root' for r in config.get('repos', []))

# 準備模板上下文
context = {
    **config,
    'has_submodules': has_submodules,
    'has_directories': has_directories,
    'is_single_repo': is_single_repo
}

# ============================================================
# 生成 CLAUDE.md
# ============================================================
try:
    template = env.get_template('CLAUDE.md.j2')
    content = template.render(**context)
    output_path = os.path.join(output_dir, 'CLAUDE.md')
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"[generate] Created: {output_path}")
except Exception as e:
    print(f"[generate] ERROR generating CLAUDE.md: {e}")

# ============================================================
# 生成 AGENTS.md
# ============================================================
try:
    template = env.get_template('AGENTS.md.j2')
    content = template.render(**context)
    output_path = os.path.join(output_dir, 'AGENTS.md')
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"[generate] Created: {output_path}")
except Exception as e:
    print(f"[generate] ERROR generating AGENTS.md: {e}")

# ============================================================
# 生成 Kit 核心規則到 .ai/rules/_kit/
# ============================================================
ai_root = os.path.dirname(os.path.dirname(os.path.abspath(config_file)))
kit_rules_dir = os.path.join(ai_root, 'rules', '_kit')
os.makedirs(kit_rules_dir, exist_ok=True)

# 生成 git-workflow.md
try:
    template = env.get_template('git-workflow.md.j2')
    content = template.render(**context)
    output_path = os.path.join(kit_rules_dir, 'git-workflow.md')
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"[generate] Created: {output_path}")
except Exception as e:
    print(f"[generate] ERROR generating git-workflow.md: {e}")

# ============================================================
# 生成 CI workflows
# ============================================================
integration_branch = config['git']['integration_branch']
release_branch = config['git']['release_branch']

for repo in config['repos']:
    repo_name = repo['name']
    repo_path = repo.get('path', './')
    repo_type = repo.get('type', 'directory')  # 預設為 directory
    language = repo.get('language', 'generic')
    
    # 決定 workflow 輸出路徑
    # submodule: 各自的 .github/workflows/
    # directory/root: 根目錄的 .github/workflows/
    if repo_type == 'submodule':
        workflow_dir = os.path.join(output_dir, repo_path, '.github', 'workflows')
        workflow_file = os.path.join(workflow_dir, 'ci.yml')
    else:
        workflow_dir = os.path.join(output_dir, '.github', 'workflows')
        # directory 類型用 ci-{name}.yml 避免覆蓋
        if repo_type == 'directory':
            workflow_file = os.path.join(workflow_dir, f'ci-{repo_name}.yml')
        else:
            workflow_file = os.path.join(workflow_dir, 'ci.yml')
    
    os.makedirs(workflow_dir, exist_ok=True)
    
    # 選擇模板（根據語言/框架）
    # 語言和框架都會映射到對應的 CI 模板
    language_templates = {
        # Go
        'go': 'ci-go.yml.j2',
        'golang': 'ci-go.yml.j2',
        
        # Node.js / JavaScript / TypeScript 生態系
        'node': 'ci-node.yml.j2',
        'nodejs': 'ci-node.yml.j2',
        'typescript': 'ci-node.yml.j2',
        'javascript': 'ci-node.yml.j2',
        'react': 'ci-node.yml.j2',
        'vue': 'ci-node.yml.j2',
        'angular': 'ci-node.yml.j2',
        'nextjs': 'ci-node.yml.j2',
        'nuxt': 'ci-node.yml.j2',
        'svelte': 'ci-node.yml.j2',
        'express': 'ci-node.yml.j2',
        'nestjs': 'ci-node.yml.j2',
        
        # Python 生態系
        'python': 'ci-python.yml.j2',
        'django': 'ci-python.yml.j2',
        'flask': 'ci-python.yml.j2',
        'fastapi': 'ci-python.yml.j2',
        
        # Rust
        'rust': 'ci-rust.yml.j2',
        
        # .NET / C#
        'dotnet': 'ci-dotnet.yml.j2',
        'csharp': 'ci-dotnet.yml.j2',
        'aspnet': 'ci-dotnet.yml.j2',
        'blazor': 'ci-dotnet.yml.j2',
        
        # 遊戲引擎
        'unity': 'ci-unity.yml.j2',
        'unreal': 'ci-unreal.yml.j2',
        'ue4': 'ci-unreal.yml.j2',
        'ue5': 'ci-unreal.yml.j2',
        'godot': 'ci-godot.yml.j2',
    }
    template_name = language_templates.get(language.lower(), 'ci-generic.yml.j2')
    
    try:
        template = env.get_template(template_name)
        ci_context = {
            'name': repo_name,
            'integration_branch': integration_branch,
            'release_branch': release_branch,
            'build_cmd': repo['verify']['build'],
            'test_cmd': repo['verify']['test'],
            # 語言特定版本
            'go_version': repo.get('go_version', '1.22.x'),
            'node_version': repo.get('node_version', '20'),
            'python_version': repo.get('python_version', '3.11'),
            'rust_version': repo.get('rust_version', 'stable'),
            'dotnet_version': repo.get('dotnet_version', '8.0.x'),
            'package_manager': repo.get('package_manager', 'npm'),
            'requirements_file': repo.get('requirements_file', 'requirements.txt'),
            # 遊戲引擎版本
            'godot_version': repo.get('godot_version', '4.2.2'),
            'use_dotnet': repo.get('use_dotnet', 'false'),
            # Unreal Engine
            'ue_version': repo.get('ue_version', '5.3'),
            'project_name': repo.get('project_name', repo_name),
        }
        content = template.render(**ci_context)
        with open(workflow_file, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"[generate] Created: {workflow_file}")
    except Exception as e:
        print(f"[generate] ERROR generating CI for {repo_name}: {e}")

# 生成 validate-submodules.yml (monorepo only)
if config['project']['type'] == 'monorepo' and has_submodules:
    workflow_dir = os.path.join(output_dir, '.github', 'workflows')
    os.makedirs(workflow_dir, exist_ok=True)
    workflow_file = os.path.join(workflow_dir, 'validate-submodules.yml')
    
    try:
        template = env.get_template('validate-submodules.yml.j2')
        content = template.render(
            integration_branch=integration_branch,
            release_branch=release_branch
        )
        with open(workflow_file, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"[generate] Created: {workflow_file}")
    except Exception as e:
        print(f"[generate] ERROR generating validate-submodules: {e}")

# ============================================================
# 創建符號連結 .claude/ -> .ai/
# ============================================================
ai_root = os.path.dirname(os.path.dirname(os.path.abspath(config_file)))
claude_dir = os.path.join(output_dir, '.claude')
os.makedirs(claude_dir, exist_ok=True)

ai_rules = os.path.join(ai_root, 'rules')
ai_commands = os.path.join(ai_root, 'commands')
claude_rules = os.path.join(claude_dir, 'rules')
claude_commands = os.path.join(claude_dir, 'commands')

def create_symlink(source, target, name):
    """創建符號連結，失敗時回退到複製"""
    if os.path.islink(target):
        os.unlink(target)
    elif os.path.isdir(target):
        shutil.rmtree(target)
    
    try:
        rel_source = os.path.relpath(source, os.path.dirname(target))
        os.symlink(rel_source, target, target_is_directory=True)
        print(f"[generate] Created symlink: {target} -> {rel_source}")
        return True
    except OSError as e:
        if platform.system() == 'Windows':
            print(f"[generate] WARNING: Cannot create symlink for {name}.")
            print(f"[generate] On Windows, enable Developer Mode:")
            print(f"[generate]   Settings -> Update & Security -> For developers -> Developer Mode: ON")
            print(f"[generate] Falling back to copy...")
        else:
            print(f"[generate] WARNING: Symlink failed for {name}: {e}")
            print(f"[generate] Falling back to copy...")
        
        if os.path.exists(source):
            shutil.copytree(source, target)
            print(f"[generate] Copied {name} to: {target}")
        return False

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
