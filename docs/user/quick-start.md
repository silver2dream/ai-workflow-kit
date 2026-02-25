# Quick Start Guide

Get AWKit running in 5 minutes. This guide covers the fastest path from zero to `awkit kickoff`.

---

## Prerequisites

Install these **before** anything else:

| Tool | Install | Why |
|------|---------|-----|
| **GitHub CLI (`gh`)** | [cli.github.com](https://cli.github.com/) | Issue/PR operations (required) |
| **`gh auth login`** | Run after installing `gh` | **#1 first-time blocker** — authenticate now |
| **Claude Code (`claude`)** | [claude.ai/download](https://claude.ai/download) | Principal AI agent |
| **Git** | [git-scm.com](https://git-scm.com/) | Version control |

```bash
# Verify everything is ready
gh auth status        # must show "Logged in"
claude --version      # must print a version
git --version         # must print a version
```

---

## Step 1: Install awkit

**macOS / Linux:**

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex
```

Verify:

```bash
awkit version
```

---

## Step 2: Init your project

```bash
cd /path/to/your-project

# Basic init
awkit init --preset go --scaffold

# Or for a monorepo
awkit init --preset react-go --scaffold

# Preview without writing files
awkit init --preset go --scaffold --dry-run
```

Run `awkit list-presets` to see all available presets (generic, go, python, rust, dotnet, node, react-go, unity-go, etc.).

---

## Step 3: Configure workflow.yaml

Open `.ai/config/workflow.yaml` and verify the basics:

```yaml
project:
  name: "my-project"
  type: "single-repo"        # or "monorepo"

repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."

git:
  integration_branch: "feat/my-feature"   # PR target branch

specs:
  base_path: ".ai/specs"
  active:
    - my-feature              # must match a folder under .ai/specs/
```

Key points:
- `specs.active` **must not be empty** — this tells AWKit which specs to process
- `git.integration_branch` is the PR target (not `main`)

---

## Step 4: Write your spec

Create a spec folder under `.ai/specs/`:

```bash
mkdir -p .ai/specs/my-feature
```

### Option A: Epic mode (recommended)

Write a `design.md` — the Principal will generate tasks from it:

```markdown
# My Feature Design

## Overview
Brief description of what this feature does.

## Requirements
- Requirement 1
- Requirement 2

## Technical Design
Describe the implementation approach.
```

### Option B: tasks.md mode

Write a `tasks.md` directly:

```markdown
# My Feature

Repo: root
Coordination: sequential

## Tasks

- [ ] 1. Implement data model
  - [ ] 1.1 Create schema
  - [ ] 1.2 Add validation

- [ ] 2. Add API endpoint
  - [ ] 2.1 Create handler
  - [ ] 2.2 Write tests
```

---

## Step 5: Generate and validate

```bash
awkit generate       # generates CLAUDE.md, AGENTS.md, rules, settings
awkit validate       # checks workflow.yaml for errors
awkit doctor         # full health check (CLI tools, config, permissions)
```

Fix any issues `doctor` reports before continuing.

---

## Step 6: Kickoff

```bash
# Preview what would happen (no side effects)
awkit kickoff --dry-run

# Start the workflow
awkit kickoff
```

AWKit will:
1. Audit the project
2. Create GitHub Issues from your spec
3. Dispatch Workers to implement each task
4. Review and merge PRs automatically

---

## Stopping the workflow

```bash
touch .ai/state/STOP
```

The Principal will finish its current task and stop gracefully.

To resume later:

```bash
rm .ai/state/STOP
awkit kickoff --resume
```

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `gh not authenticated` | Run `gh auth login` |
| `specs.active is empty` | Add a spec name to `specs.active` in workflow.yaml |
| `spec missing tasks.md or design.md` | Create the file in `.ai/specs/<name>/` |
| `repo path does not exist` | Check `repos[].path` in workflow.yaml |
| `Working directory not clean` | Commit or stash your changes |

For more: [Troubleshooting](troubleshooting.md) | [FAQ](faq.md) | [Configuration Reference](configuration.md)
