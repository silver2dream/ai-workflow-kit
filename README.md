# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![Bash](https://img.shields.io/badge/Bash-required-4EAA25?logo=gnubash&logoColor=white)]()
[![Python](https://img.shields.io/badge/Python-3.11-3776AB?logo=python&logoColor=white)](https://www.python.org/)
[![GitHub CLI](https://img.shields.io/badge/gh-required-181717?logo=github&logoColor=white)](https://cli.github.com/)

> An AI-assisted development workflow kit that drives **Spec → Implement → PR → Merge**, designed to work with **Claude Code (Principal)** + **Codex (Worker)**, and compatible with **Kiro-style specs**.

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
│  You ──► kickoff.sh ──► Claude Code (Principal)               │
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
- `python3` + `pyyaml` + `jsonschema` + `jinja2`

### Online / E2E (optional)
- `gh` (GitHub CLI) + `gh auth login`
- `claude` (Claude Code)
- `codex` (Worker)

---

## 📁 Project Structure

```
.
├── .ai/                         # kit (scripts/templates/rules/specs)
│   ├── config/workflow.yaml     # main config
│   ├── scripts/                 # automation scripts
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
# From anywhere, specify the project path
awkit install /path/to/your-project --preset react-go

# Or from within the project directory
cd /path/to/your-project
awkit install . --preset react-go
```

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

Update kit files inside a project:

```bash
awkit install /path/to/your-project --force

# Or from within the project directory
awkit install . --force
```

### 1) Install offline dependencies

```bash
pip3 install pyyaml jsonschema jinja2
```

### 2) Generate outputs

```bash
bash .ai/scripts/generate.sh
```

### 3) (Optional) Run the full workflow

```bash
gh auth login
bash .ai/scripts/kickoff.sh --dry-run
bash .ai/scripts/kickoff.sh
```

Stop:

```bash
touch .ai/state/STOP
```

---

## ⚙️ Configuration

Main config: `.ai/config/workflow.yaml`

### Repo type

- `type: directory`: monorepo subdirectories (single git repo)
- `type: submodule`: git submodules (independent repos)
- `type: root`: single-repo

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

Note: this repo ships a hand-maintained CI example. `bash .ai/scripts/generate.sh` does **not** modify workflows unless you pass `--generate-ci`.

It runs:
- AWK evaluation: `bash .ai/scripts/evaluate.sh --offline` and `--offline --strict`
- Kit tests: `bash .ai/tests/run_all_tests.sh`
- Backend tests: `go test ./...` (in `backend/`)
- Frontend sanity: `frontend/Packages/manifest.json` JSON validation + folder checks

---

## 🧪 Evaluation

- For kit maintainers/CI only; regular users can skip.
- Standard: `.ai/docs/evaluate.md`
- Executor: `.ai/scripts/evaluate.sh`

---

## 📚 Documentation

### For Users

| Document | Description |
|----------|-------------|
| [Getting Started](docs/user/getting-started.md) | Quick start guide |
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

No license file is provided yet. Treat this repository as “all rights reserved” until a license is added.
