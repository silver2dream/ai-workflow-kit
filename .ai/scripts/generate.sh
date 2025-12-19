#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# generate.sh - Generate AWK helper docs and optional scaffolding
#
# Usage:
#   bash .ai/scripts/generate.sh [--generate-ci] [--install-deps]
#
# Generates:
#   - CLAUDE.md
#   - AGENTS.md
#   - .ai/rules/_kit/git-workflow.md
#   - .claude/{rules,commands} (symlink or copy to .ai/{rules,commands})
#
# Optional:
#   --generate-ci   Generate workflow(s) under .github/workflows/ from templates
#   --install-deps  Install python deps via pip (requires network)
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
MONO_ROOT="$(dirname "$AI_ROOT")"

CONFIG_FILE="$AI_ROOT/config/workflow.yaml"
TEMPLATES_DIR="$AI_ROOT/templates"

GENERATE_CI=false
INSTALL_DEPS=false

usage() {
  cat <<'EOF'
Usage:
  bash .ai/scripts/generate.sh [--generate-ci] [--install-deps]

Options:
  --generate-ci    Generate GitHub Actions workflow(s) from templates
  --install-deps   Install python deps via pip (requires network)
  -h, --help       Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --generate-ci) GENERATE_CI=true; shift ;;
    --install-deps) INSTALL_DEPS=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR: unknown argument: $1"; usage; exit 2 ;;
  esac
done

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "ERROR: Config file not found: $CONFIG_FILE"
  exit 1
fi

echo "[generate] Reading config from $CONFIG_FILE"
echo "[generate] Templates dir: $TEMPLATES_DIR"

# 撽??蔭
echo "[generate] Validating config..."
if ! python3 "$AI_ROOT/scripts/validate_config.py" "$CONFIG_FILE"; then
  echo "[generate] ERROR: Config validation failed"
  exit 1
fi

if ! python3 -c "import jinja2" 2>/dev/null; then
  if [[ "$INSTALL_DEPS" == "true" ]]; then
    if ! command -v pip3 >/dev/null 2>&1; then
      echo "ERROR: pip3 not found. Install python3-pip or pipx."
      exit 1
    fi
    echo "[generate] Installing python deps (jinja2)..."
    pip3 install jinja2 --quiet
  else
    echo "ERROR: python package 'jinja2' is required."
    echo "Install: pip3 install jinja2"
    echo "Or re-run: bash .ai/scripts/generate.sh --install-deps (requires network)"
    exit 1
  fi
fi

python3 - "$CONFIG_FILE" "$TEMPLATES_DIR" "$MONO_ROOT" "$GENERATE_CI" <<'PYTHON'
import sys
import os
import yaml
import shutil
import platform
from jinja2 import Environment, FileSystemLoader

config_file = sys.argv[1]
templates_dir = sys.argv[2]
output_dir = sys.argv[3]
generate_ci = (sys.argv[4].strip().lower() == "true")

with open(config_file, "r", encoding="utf-8") as f:
    config = yaml.safe_load(f) or {}

env = Environment(
    loader=FileSystemLoader(templates_dir),
    trim_blocks=True,
    lstrip_blocks=True
)

has_submodules = any(r.get('type') == 'submodule' for r in config.get('repos', []))
has_directories = any(r.get('type') == 'directory' for r in config.get('repos', []))
is_single_repo = config['project']['type'] == 'single-repo' or any(r.get('type') == 'root' for r in config.get('repos', []))

context = {
    **config,
    'has_submodules': has_submodules,
    'has_directories': has_directories,
    'is_single_repo': is_single_repo
}

# ============================================================
# Generate CLAUDE.md
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
# Generate AGENTS.md
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
# Generate kit rules under .ai/rules/_kit/
# ============================================================
ai_root = os.path.dirname(os.path.dirname(os.path.abspath(config_file)))
kit_rules_dir = os.path.join(ai_root, 'rules', '_kit')
os.makedirs(kit_rules_dir, exist_ok=True)

# ?? git-workflow.md
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
# Optional: generate CI workflows
# ============================================================
if generate_ci:
    integration_branch = config["git"]["integration_branch"]
    release_branch = config["git"]["release_branch"]

    language_templates = {
        "go": "ci-go.yml.j2",
        "golang": "ci-go.yml.j2",
        "node": "ci-node.yml.j2",
        "nodejs": "ci-node.yml.j2",
        "typescript": "ci-node.yml.j2",
        "javascript": "ci-node.yml.j2",
        "react": "ci-node.yml.j2",
        "vue": "ci-node.yml.j2",
        "angular": "ci-node.yml.j2",
        "nextjs": "ci-node.yml.j2",
        "nuxt": "ci-node.yml.j2",
        "svelte": "ci-node.yml.j2",
        "express": "ci-node.yml.j2",
        "nestjs": "ci-node.yml.j2",
        "python": "ci-python.yml.j2",
        "django": "ci-python.yml.j2",
        "flask": "ci-python.yml.j2",
        "fastapi": "ci-python.yml.j2",
        "rust": "ci-rust.yml.j2",
        "dotnet": "ci-dotnet.yml.j2",
        "csharp": "ci-dotnet.yml.j2",
        "aspnet": "ci-dotnet.yml.j2",
        "blazor": "ci-dotnet.yml.j2",
        "unity": "ci-unity.yml.j2",
        "unreal": "ci-unreal.yml.j2",
        "ue4": "ci-unreal.yml.j2",
        "ue5": "ci-unreal.yml.j2",
        "godot": "ci-godot.yml.j2",
    }

    for repo in config.get("repos", []):
        repo_name = repo["name"]
        repo_path = repo.get("path", "./")
        repo_type = repo.get("type", "directory")
        language = (repo.get("language") or "generic").lower()

        if repo_type == "submodule":
            workflow_dir = os.path.join(output_dir, repo_path, ".github", "workflows")
            workflow_file = os.path.join(workflow_dir, "ci.yml")
        else:
            workflow_dir = os.path.join(output_dir, ".github", "workflows")
            if repo_type == "directory":
                workflow_file = os.path.join(workflow_dir, f"ci-{repo_name}.yml")
            else:
                workflow_file = os.path.join(workflow_dir, "ci.yml")

        os.makedirs(workflow_dir, exist_ok=True)

        template_name = language_templates.get(language, "ci-generic.yml.j2")
        try:
            template = env.get_template(template_name)
            verify = repo.get("verify") or {}
            ci_context = {
                "name": repo_name,
                "integration_branch": integration_branch,
                "release_branch": release_branch,
                "build_cmd": verify.get("build", "echo 'build'"),
                "test_cmd": verify.get("test", "echo 'test'"),
                "go_version": repo.get("go_version", "1.22.x"),
                "node_version": repo.get("node_version", "20"),
                "python_version": repo.get("python_version", "3.11"),
                "rust_version": repo.get("rust_version", "stable"),
                "dotnet_version": repo.get("dotnet_version", "8.0.x"),
                "package_manager": repo.get("package_manager", "npm"),
                "requirements_file": repo.get("requirements_file", "requirements.txt"),
                "godot_version": repo.get("godot_version", "4.2.2"),
                "use_dotnet": repo.get("use_dotnet", "false"),
                "ue_version": repo.get("ue_version", "5.3"),
                "project_name": repo.get("project_name", repo_name),
            }
            content = template.render(**ci_context)
            with open(workflow_file, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"[generate] Created: {workflow_file}")
        except Exception as e:
            print(f"[generate] ERROR generating CI for {repo_name}: {e}")

    if config.get("project", {}).get("type") == "monorepo" and has_submodules:
        workflow_dir = os.path.join(output_dir, ".github", "workflows")
        os.makedirs(workflow_dir, exist_ok=True)
        workflow_file = os.path.join(workflow_dir, "validate-submodules.yml")

        try:
            template = env.get_template("validate-submodules.yml.j2")
            content = template.render(
                integration_branch=integration_branch,
                release_branch=release_branch
            )
            with open(workflow_file, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"[generate] Created: {workflow_file}")
        except Exception as e:
            print(f"[generate] ERROR generating validate-submodules: {e}")
else:
    print("[generate] Skipping CI generation (use --generate-ci to enable).")

# ============================================================
# Mirror .ai/{rules,commands} into .claude/{rules,commands}
# ============================================================
ai_root = os.path.dirname(os.path.dirname(os.path.abspath(config_file)))
claude_dir = os.path.join(output_dir, '.claude')
os.makedirs(claude_dir, exist_ok=True)

ai_rules = os.path.join(ai_root, 'rules')
ai_commands = os.path.join(ai_root, 'commands')
claude_rules = os.path.join(claude_dir, 'rules')
claude_commands = os.path.join(claude_dir, 'commands')

def create_symlink(source, target, name):
    """Create symlink from target to source; fall back to copy if needed."""
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
            shutil.copytree(source, target, dirs_exist_ok=True)
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
