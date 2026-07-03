# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Bash](https://img.shields.io/badge/Bash-required-4EAA25?logo=gnubash&logoColor=white)]()
[![GitHub CLI](https://img.shields.io/badge/gh-required-181717?logo=github&logoColor=white)](https://cli.github.com/)

> An AI-assisted development workflow kit that drives **Spec → Implement → PR → Merge**, designed to work with **Claude Code (Principal)** + **Codex (Worker)**, and compatible with **Kiro-style specs**.

[![Download](https://img.shields.io/badge/Download-Latest%20Release-brightgreen?style=for-the-badge&logo=github)](https://github.com/silver2dream/ai-workflow-kit/releases/latest)

[English](README.md) | [繁體中文](README-zh-TW.md)

---

## 📋 Table of Contents

- [Features](#-features)
- [Architecture Overview](#-architecture-overview)
- [Technology Stack](#-technology-stack)
- [Project Structure](#-project-structure)
- [Quick Start](#-quick-start)
- [Configuration](#-configuration)
- [Directory Monorepo Example](#-directory-monorepo-example)
- [CI](#-ci)
- [Evaluation](#-evaluation)
- [Docs](#-docs)
- [Contributing](#-contributing)
- [License](#-license)

---

## ✨ Features

### Core Workflow
- **Spec-driven**: reads `.ai/specs/<name>/tasks.md` (Kiro-compatible) to decide what to do next
- **GitHub as state machine**: uses issues/PRs + labels to track progress
- **Dispatch + review loop**: dispatches implementation to Worker, then reviews/merges or creates fix issues

### Kit Quality
- **Offline Gate**: deterministic verification (no network required)
- **Strict mode**: `--strict` enforces “no P0 findings” in audit (CI/release checks)
- **Extensibility checks**: validates CI triggers on `feat/example` (branch alignment)

---

## 🏗️ Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                                                              │
│  You ──► awkit kickoff ──► Claude Code (Principal)            │
│                              │                               │
│                              ├─► read specs/tasks.md          │
│                              ├─► create GitHub Issue          │
│                              ├─► dispatch to Codex (Worker)   │
│                              ├─► review PR                    │
│                              ├─► merge or reject              │
│                              └─► loop                         │
│                                                              │
│  Morning ──► gh pr list ──► harvest                            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

More details: `docs/ai-workflow-architecture.md`.

---

## 🛠️ Technology Stack

### Offline (required)
- `bash` (Windows: Git Bash / WSL)
- `git`
- `go` 1.25+

### Offline (optional)
- `python3` + `pyyaml` + `jsonschema` + `jinja2` (only needed for legacy scripts; generation is built into `awkit`)

### Online / E2E (optional)
- `gh` (GitHub CLI) + `gh auth login`
- `claude` (Claude Code)
- `codex` (Worker)

---

## 📁 Project Structure

```
.
├── .ai/                         # kit (config/templates/rules/specs)
│   ├── config/workflow.yaml     # main config
│   ├── templates/               # generators (CLAUDE/AGENTS/CI)
│   ├── rules/                   # architecture + git workflow rules
│   ├── docs/evaluate.md         # evaluation standard
│   └── specs/                   # Kiro-style specs
├── .github/workflows/ci.yml     # root CI example
├── backend/                     # directory example (Go)
└── frontend/                    # directory example (Unity skeleton)
```

---

## 🚀 Quick Start

### 0) Install `awkit` (recommended)

`awkit` is the cross-platform AWK installer CLI (named `awkit` to avoid clashing with the system `awk` command).

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex
```

Install AWK into a project:

```bash
# Initialize AWK in current directory
awkit init

# With a preset and scaffold
awkit init --preset go --scaffold

# Monorepo with React + Go
awkit init --preset react-go --scaffold

# Preview what would be created
awkit init --preset python --scaffold --dry-run
```

### Available Presets

| Category | Presets |
|----------|---------|
| Single-Repo | `generic`, `go`, `python`, `rust`, `dotnet`, `node` |
| Monorepo | `react-go`, `react-python`, `unity-go`, `godot-go`, `unreal-go` |

Run `awkit list-presets` for details. See [Getting Started](docs/getting-started.md) for scaffold file structures.

Note: `awkit install` is an alias for `awkit init` (backward compatible).

For a complete workflow walkthrough, see the [Quick Start Guide](docs/user/quick-start.md).

### 0.1) Update `awkit`

Check version and updates:

```bash
awkit version
awkit check-update
```

Update the CLI:

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

Update kit files inside a project (preserves your workflow.yaml):

```bash
awkit upgrade
awkit generate
```

Other update options:

```bash
# Apply a different preset to workflow.yaml only
awkit init --preset react-go --force-config

# Upgrade kit files and overwrite workflow.yaml (requires --preset)
awkit upgrade --force-config --preset react-go

# Full reset: update kit files AND apply preset to workflow.yaml
awkit init --preset react-go --force
```

### 1) Generate outputs

```bash
awkit generate
```

### 2) (Optional) Run the full workflow

```bash
gh auth login

# Using awkit CLI (recommended)
awkit kickoff --dry-run    # Preview what would happen
awkit kickoff              # Start the workflow
awkit kickoff --resume     # Resume from saved state
awkit validate             # Validate config only

# Legacy bash scripts have been removed; use awkit commands above
```

Stop:

```bash
touch .ai/state/STOP
```

---

## ⚙️ Configuration

Main config: `.ai/config/workflow.yaml`

### Repo type

AWK supports three repository types configured in `.ai/config/workflow.yaml`:

| Type | Description | Use Case |
|------|-------------|----------|
| `root` | Single repository | Standalone projects |
| `directory` | Subdirectory in monorepo | Monorepo with shared .git |
| `submodule` | Git submodule | Monorepo with independent repos |

**Type-Specific Behavior:**
- **root**: All operations run from repo root. Path must be `./`.
- **directory**: Operations run from worktree root, changes scoped to subdirectory.
- **submodule**: Commits/pushes happen in submodule first, then parent updates reference.

Example:
```yaml
repos:
  - name: backend
    path: backend/
    type: directory  # or: root, submodule
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
```

### Specs

Spec folder structure (Kiro compatible):

```
.ai/specs/<feature-name>/
├── requirements.md   # optional
├── design.md         # optional
└── tasks.md          # required
```

To enable a spec, add its folder name to `specs.active` in `.ai/config/workflow.yaml`.

---

## 📦 Directory Monorepo Example

This repo ships with a minimal directory-type example:

- `backend/`: a tiny Go module + unit test (`go test ./...`)
- `frontend/`: Unity skeleton (CI runs structure + JSON sanity only)
- Spec example: `.ai/specs/example/`
- Guide: `docs/getting-started.md`

---

## 🔁 CI

Root CI workflow: `.github/workflows/ci.yml`

**For user projects:**
- `awkit init` automatically creates a CI workflow for your project
- `awkit upgrade` automatically migrates deprecated CI configurations (removes old `awk` job)

**For this repo (awkit itself):**
This repo ships a hand-maintained CI example. `awkit generate` does **not** modify workflows unless you pass `--generate-ci`.

It runs:
- AWK evaluation: `awkit evaluate --offline` and `--offline --strict`
- Kit tests: `go test ./...`
- Backend tests: `go test ./...` (in `backend/`)
- Frontend sanity: `frontend/Packages/manifest.json` JSON validation + folder checks

---

## 🧪 Evaluation

- For kit maintainers/CI only; regular users can skip.
- Standard: `.ai/docs/evaluate.md`
- Executor: `awkit evaluate --offline` (report-only) and `awkit evaluate --offline --strict` (fails on any gate failure, e.g. P0 audit findings — used by CI/release checks)

---

## 📚 Documentation

### For Users

| Document | Description |
|----------|-------------|
| [Quick Start](docs/user/quick-start.md) | 5-minute setup guide |
| [Getting Started](docs/user/getting-started.md) | Detailed setup guide (zh-TW) |
| [Configuration](docs/user/configuration.md) | workflow.yaml reference |
| [Troubleshooting](docs/user/troubleshooting.md) | Error solutions |
| [FAQ](docs/user/faq.md) | Common questions |

### For Developers

| Document | Description |
|----------|-------------|
| [Architecture](docs/developer/architecture.md) | System internals |
| [API Reference](docs/developer/api-reference.md) | Scripts & modules |
| [Contributing](docs/developer/contributing.md) | Development guide |
| [Testing](docs/developer/testing.md) | Test framework |

### Other

- [Architecture Overview](docs/ai-workflow-architecture.md) - High-level system design

---

## 🤝 Contributing

See [Contributing Guide](docs/developer/contributing.md) for:
- Development setup
- Code standards
- PR workflow

Quick reference:
- Branch model and commit format: `.ai/rules/_kit/git-workflow.md`
- PR base should target `feat/example` by default.

---

## 📄 License

This project is licensed under the [Apache License 2.0](LICENSE).

## 🔒 Security & Trust

AWK follows open source security best practices and is monitored by [OpenSSF Scorecard](https://securityscorecards.dev/).

[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)

### Security Features

| Feature | Status | Description |
|---------|--------|-------------|
| **SECURITY.md** | ✅ | Vulnerability reporting policy and SLA |
| **Branch Protection** | ✅ | Required reviews and CI checks |
| **CI/CD** | ✅ | Automated testing on all PRs |
| **Dependency Updates** | ✅ | Dependabot enabled |
| **Static Analysis** | ✅ | CodeQL scanning |
| **Token Permissions** | ✅ | Minimal GitHub token permissions |

See [SECURITY.md](SECURITY.md) for full security policy and vulnerability reporting.
