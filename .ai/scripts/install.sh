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
mkdir -p "$TARGET_DIR/.ai/state"
mkdir -p "$TARGET_DIR/.ai/results"
mkdir -p "$TARGET_DIR/.ai/runs"
mkdir -p "$TARGET_DIR/.ai/exe-logs"

# Copy scripts
log_info "Copying scripts..."
cp "$KIT_ROOT/scripts/"*.sh "$TARGET_DIR/.ai/scripts/" 2>/dev/null || true

# Copy templates
log_info "Copying templates..."
cp "$KIT_ROOT/templates/"*.j2 "$TARGET_DIR/.ai/templates/" 2>/dev/null || true

# Copy rules (as examples)
log_info "Copying rule templates..."
cp "$KIT_ROOT/rules/"*.md "$TARGET_DIR/.ai/rules/" 2>/dev/null || true

# Copy commands
log_info "Copying commands..."
cp "$KIT_ROOT/commands/"*.md "$TARGET_DIR/.ai/commands/" 2>/dev/null || true

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

repos:
  - name: root
    path: ./
    type: local
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
  structure:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
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

# Update .gitignore
if [[ -f "$TARGET_DIR/.gitignore" ]]; then
    if ! grep -q ".ai/state/" "$TARGET_DIR/.gitignore"; then
        log_info "Updating .gitignore..."
        cat >> "$TARGET_DIR/.gitignore" << 'GITIGNORE'

# AI Workflow - Runtime State
.ai/state/
.ai/results/
.ai/runs/
.ai/exe-logs/
GITIGNORE
    fi
else
    log_info "Creating .gitignore..."
    cat > "$TARGET_DIR/.gitignore" << 'GITIGNORE'
# AI Workflow - Runtime State
.ai/state/
.ai/results/
.ai/runs/
.ai/exe-logs/
GITIGNORE
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
log_info "  3. Run: bash .ai/scripts/kickoff.sh --dry-run"
log_info ""
log_info "Note: Files in .ai/ are the source of truth."
log_info "      .claude/ uses symlinks to .ai/ for Claude Code compatibility."

if [[ "$SYMLINK_OK" == "false" ]]; then
    log_warn ""
    log_warn "Symlinks could not be created. Using file copies instead."
    log_warn "Remember to run 'bash .ai/scripts/generate.sh' after modifying .ai/rules/ or .ai/commands/"
fi
