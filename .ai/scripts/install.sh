#!/usr/bin/env bash
# install.sh - Install AI Workflow Kit to a new project
# Usage: bash install.sh <target_directory>

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

usage() {
    cat << EOF
Usage: bash install.sh <target_directory>

Install AI Workflow Kit to a target project directory.

Options:
  -h, --help    Show this help message

Example:
  bash install.sh /path/to/my-project
  bash install.sh ../another-project
EOF
    exit 0
}

# Parse arguments
[[ "${1:-}" == "-h" || "${1:-}" == "--help" ]] && usage
[[ -z "${1:-}" ]] && { log_error "Target directory required"; usage; }

TARGET_DIR="$1"

# Validate target
if [[ ! -d "$TARGET_DIR" ]]; then
    log_error "Target directory does not exist: $TARGET_DIR"
    exit 1
fi

if [[ ! -d "$TARGET_DIR/.git" ]]; then
    log_warn "Target is not a git repository. Continue anyway? (y/N)"
    read -r response
    [[ "$response" != "y" && "$response" != "Y" ]] && exit 1
fi

log_info "Installing AI Workflow Kit to: $TARGET_DIR"

# Create .ai directory structure
log_info "Creating directory structure..."
mkdir -p "$TARGET_DIR/.ai/config"
mkdir -p "$TARGET_DIR/.ai/scripts"
mkdir -p "$TARGET_DIR/.ai/templates"
mkdir -p "$TARGET_DIR/.ai/rules"
mkdir -p "$TARGET_DIR/.ai/commands"
mkdir -p "$TARGET_DIR/.ai/specs"
mkdir -p "$TARGET_DIR/.ai/docs"
mkdir -p "$TARGET_DIR/.ai/tests"
mkdir -p "$TARGET_DIR/.ai/state"
mkdir -p "$TARGET_DIR/.ai/results"
mkdir -p "$TARGET_DIR/.ai/runs"
mkdir -p "$TARGET_DIR/.ai/exe-logs"

# Copy scripts (bash + python)
log_info "Copying scripts..."
cp -R "$KIT_ROOT/scripts/"* "$TARGET_DIR/.ai/scripts/" 2>/dev/null || true

# Copy templates
log_info "Copying templates..."
cp -R "$KIT_ROOT/templates/"* "$TARGET_DIR/.ai/templates/" 2>/dev/null || true

# Copy rules (kit + examples)
log_info "Copying rules..."
cp -R "$KIT_ROOT/rules/"* "$TARGET_DIR/.ai/rules/" 2>/dev/null || true

# Copy commands
log_info "Copying commands..."
cp -R "$KIT_ROOT/commands/"* "$TARGET_DIR/.ai/commands/" 2>/dev/null || true

# Copy docs (required for evaluate.sh version sync)
log_info "Copying docs..."
cp -R "$KIT_ROOT/docs/"* "$TARGET_DIR/.ai/docs/" 2>/dev/null || true

# Copy tests (required for Offline Gate O10)
log_info "Copying tests..."
cp -R "$KIT_ROOT/tests/"* "$TARGET_DIR/.ai/tests/" 2>/dev/null || true

# Copy config schemas and defaults
log_info "Copying config files..."
cp -R "$KIT_ROOT/config/"* "$TARGET_DIR/.ai/config/" 2>/dev/null || true

# Create sample config if not exists
if [[ ! -f "$TARGET_DIR/.ai/config/workflow.yaml" ]]; then
    log_info "Creating sample workflow.yaml..."
    cat > "$TARGET_DIR/.ai/config/workflow.yaml" << 'YAML'
# AI Workflow Kit Configuration
# Customize this for your project

version: "1.0"

project:
  name: "my-project"
  description: "Project description"
  type: "single-repo"  # monorepo | single-repo

# Repo types:
#   - root: single repo (root)
#   - directory: monorepo directories (no submodules)
#   - submodule: git submodule
# Example 1: Single repo
# repos:
#   - name: root
#     path: ./
#     type: root
#
# Example 2: Monorepo with directories (no submodules)
# repos:
#   - name: backend
#     path: backend/
#     type: directory
#   - name: frontend
#     path: frontend/
#     type: directory
#
# Example 3: Monorepo with submodules
# repos:
#   - name: backend
#     path: backend/
#     type: submodule

repos:
  - name: root
    path: ./
    type: root  # root | directory | submodule
    language: typescript  # go | typescript | python | unity | etc.
    rules:
      - git-workflow
    verify:
      build: "npm run build"
      test: "npm test"

git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"  # spec requirements
    design: "design.md"              # design doc (optional)
    tasks: "tasks.md"                # task list
  auto_generate_tasks: true  # derive tasks.md from design.md
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - missing-tests
  custom: []

notifications:
  slack_webhook: "${AI_SLACK_WEBHOOK}"
  discord_webhook: "${AI_DISCORD_WEBHOOK}"
  system_notify: true
YAML
fi

# Create .gitkeep files
touch "$TARGET_DIR/.ai/state/.gitkeep"
touch "$TARGET_DIR/.ai/results/.gitkeep"
touch "$TARGET_DIR/.ai/runs/.gitkeep"
touch "$TARGET_DIR/.ai/exe-logs/.gitkeep"

# Update .gitignore (append if not already present)
AI_GITIGNORE_MARKER="# >>> AI Workflow Kit >>>"
AI_GITIGNORE_CONTENT="$AI_GITIGNORE_MARKER
# Runtime state (do not commit)
.ai/state/
.ai/results/
.ai/runs/
.ai/exe-logs/
.worktrees/
# <<< AI Workflow Kit <<<"

if [[ -f "$TARGET_DIR/.gitignore" ]]; then
    if ! grep -q "$AI_GITIGNORE_MARKER" "$TARGET_DIR/.gitignore"; then
        log_info "Appending AI Workflow entries to .gitignore..."
        echo "" >> "$TARGET_DIR/.gitignore"
        echo "$AI_GITIGNORE_CONTENT" >> "$TARGET_DIR/.gitignore"
    else
        log_info ".gitignore already contains AI Workflow entries, skipping..."
    fi
else
    log_info "Creating .gitignore..."
    echo "$AI_GITIGNORE_CONTENT" > "$TARGET_DIR/.gitignore"
fi

# Create symlinks .claude/ -> .ai/ (cross-platform)
log_info "Creating symlinks for Claude Code compatibility..."

create_symlink() {
    local source="$1"
    local target="$2"
    local name="$3"
    
    # Remove existing target
    if [[ -L "$target" ]]; then
        rm "$target"
    elif [[ -d "$target" ]]; then
        rm -rf "$target"
    fi
    
    # Calculate relative path
    local rel_source
    rel_source=$(python3 -c "import os; print(os.path.relpath('$source', os.path.dirname('$target')))")
    
    # Try to create symlink
    if ln -s "$rel_source" "$target" 2>/dev/null; then
        log_info "Created symlink: $target -> $rel_source"
        return 0
    else
        # Fallback to copy on Windows without Developer Mode
        if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
            log_warn "Cannot create symlink for $name."
            log_warn "On Windows, enable Developer Mode or run as Administrator:"
            log_warn "  Settings -> Update & Security -> For developers -> Developer Mode: ON"
            log_warn "Falling back to copy..."
        else
            log_warn "Symlink failed for $name, falling back to copy..."
        fi
        cp -r "$source" "$target"
        return 1
    fi
}

mkdir -p "$TARGET_DIR/.claude"
SYMLINK_OK=true

if [[ -d "$TARGET_DIR/.ai/rules" ]]; then
    create_symlink "$TARGET_DIR/.ai/rules" "$TARGET_DIR/.claude/rules" "rules" || SYMLINK_OK=false
fi

if [[ -d "$TARGET_DIR/.ai/commands" ]]; then
    create_symlink "$TARGET_DIR/.ai/commands" "$TARGET_DIR/.claude/commands" "commands" || SYMLINK_OK=false
fi

log_info "Installation complete!"
log_info ""
log_info "Next steps:"
log_info "  1. Edit .ai/config/workflow.yaml for your project"
log_info "  2. Run: bash .ai/scripts/generate.sh"
log_info "     This will generate:"
log_info "     - CLAUDE.md and AGENTS.md"
log_info "     - .ai/rules/_kit/git-workflow.md"
log_info "     - .claude/{rules,commands} (symlink or copy)"
log_info "     - (optional) CI workflows: use 'bash .ai/scripts/generate.sh --generate-ci'"
log_info "  3. Run: bash .ai/scripts/kickoff.sh --dry-run"
log_info ""
log_info "Note: Files in .ai/ are the source of truth."
log_info "      .claude/ uses symlinks to .ai/ for Claude Code compatibility."

if [[ "$SYMLINK_OK" == "false" ]]; then
    log_warn ""
    log_warn "Symlinks could not be created. Using file copies instead."
    log_warn "Remember to run 'bash .ai/scripts/generate.sh' after modifying .ai/rules/ or .ai/commands/"
fi
