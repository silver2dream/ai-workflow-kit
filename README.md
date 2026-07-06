# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Bash](https://img.shields.io/badge/Bash-required-4EAA25?logo=gnubash&logoColor=white)]()
[![GitHub CLI](https://img.shields.io/badge/gh-required-181717?logo=github&logoColor=white)](https://cli.github.com/)

> An AI-assisted development workflow kit that drives **Spec → Implement → PR → Merge** with a **self-improving** review loop. A **Principal** (Claude Code) orchestrates a **Worker** (Codex or Claude Code), and quality is enforced by deterministic Go gates — not by trusting LLM prose. Compatible with **Kiro-style specs**.

[![Download](https://img.shields.io/badge/Download-Latest%20Release-brightgreen?style=for-the-badge&logo=github)](https://github.com/silver2dream/ai-workflow-kit/releases/latest)

[English](README.md) | [繁體中文](README-zh-TW.md)

---

## 🌟 What Makes AWK Different

Most AI workflow tools are a dispatch loop that trusts whatever the model says. AWK is built on two ideas that set it apart:

### 🧠 A Self-Improving Learning Loop

AWK **learns from its own review history**. Every rejection runs through a four-step loop — **record → distill → inject → verify**:

- A rejected PR is **distilled** by an LLM into a compact, **committable lesson** (`.ai/state/lessons.json`) — a durable check, not a raw log line.
- Relevant lessons are **injected** into future Worker/Reviewer prompts (scoped to the files you're touching, via the same relevance engine that powers knowledge-graph grounding).
- Each lesson is **settled hit/miss** against real outcomes, and promoted through `candidate → active → proven` — ineffective lessons retire automatically.
- A **proven** lesson can be promoted (`awkit lessons promote`) into a **hard gate** (rule / audit check) via a human-reviewed issue.

The result: **a mistake caught once becomes a guardrail that stops it recurring.** The system gets harder to fool over time — and because lessons are committable JSON, the whole team shares the learning (unlike agent memory locked in a private runtime).

### 🤝 Agent-Friendly Interfaces (ACI)

AWK treats its agents as first-class users of a real interface, not prose-in / regex-out:

- The reviewer submits a **structured `review.json`**, and AWK *renders* the human-readable review from it — nothing is parsed back out of hand-formatted markdown.
- **Format errors are corrected in the same session** (exit code 2, seconds) — only genuine **evidence failures** cost a fresh re-review. The interface tells the agent exactly which field to fix.
- Quality is enforced by **deterministic Go gates**: evidence verification (re-runs the tests, checks each criterion maps to a real passing assertion), severity/verdict consistency, and multi-model consensus — all in code, not agent-side math.

---

## 📋 Table of Contents

- [What Makes AWK Different](#-what-makes-awk-different)
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
- **Worker backend selection**: `codex` (default) or `claude-code` via `worker.backend`

### Review Quality
- **Structured review submission**: reviewer submits `review.json` (`submit-review --body-file`); schema errors are corrected in-session, only real evidence failures block
- **Evidence verification gate**: re-runs the test suite and verifies each acceptance criterion maps to a passing test with a real assertion before merge
- **Multi-model consensus** (opt-in): `review.multi_model` runs secondary reviewers in parallel and applies weighted scoring with an `[ERROR]`-severity cap — enforced in Go, not agent-side math
- **Severity/verdict consistency gate**: a below-threshold score must carry a Critical/Important finding; a passing score must not carry a Critical one
- **JiT tests** (opt-in): generates independent tests from the PR diff at review time

### Learning Loop
- **Record → distill → inject → verify**: review rejections are distilled into compact, committable lessons (`.ai/state/lessons.json`) injected into future Worker/Reviewer prompts and settled hit/miss against outcomes
- **Promotable to hard gates**: `awkit lessons promote` opens a human-gated issue to harden a proven lesson into a rule/audit check

### Context Grounding
- **Design-doc injection**: relevant `design.md` context is added to the Worker prompt
- **Knowledge-graph injection**: when `.understand-anything/knowledge-graph.json` exists, a ticket-relevant slice of the codebase map (files + dependents) is injected (`worker.knowledge_graph`)

### Kit Quality & Ops
- **Offline Gate**: deterministic verification (no network required) via `awkit evaluate --offline`
- **Strict mode**: `--strict` enforces “no P0 findings” in audit (CI/release checks)
- **Cross-platform**: native Windows support (ConPTY) plus Linux/macOS; the full test suite runs on `windows-latest` in CI
- **Token/cost observability**: per-session and per-worker LLM token/cost tracking (`awkit events`, `ResultMetrics`)
- **Lifecycle hooks**: run shell commands at pre/post dispatch, pre/post review, on merge, on failure

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
- `bash` (Windows: Git Bash — some verification steps shell out to `sh`; the Principal runner itself has native Windows/ConPTY support)
- `git`
- `go` 1.25+

### Offline (optional)
- `python3` (used only for the frontend CI JSON-validation example; core generation is built into `awkit`)

### Online / E2E (optional)
- `gh` (GitHub CLI) + `gh auth login`
- `claude` (Claude Code) — required for the Principal, and for the Worker when `worker.backend: claude-code`
- `codex` — required for the Worker when `worker.backend: codex` (default)

---

## 📁 Project Structure

```
.
├── .ai/                         # kit (config/templates/rules/specs)
│   ├── config/workflow.yaml     # main config
│   ├── templates/               # generators (CLAUDE/AGENTS/CI)
│   ├── rules/                   # architecture + git workflow rules
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

### Config sections

`workflow.yaml` has these top-level sections (full reference: [docs/user/configuration.md](docs/user/configuration.md)):

| Section | Purpose |
|---------|---------|
| `project` / `repos` | Repo layout, types, per-repo `verify` commands |
| `git` | Integration/release branches, commit format, PR template |
| `specs` / `tasks` / `audit` | Spec sources, task format, audit checks |
| `github` | Issue/PR labels, repo override |
| `rules` / `agents` | Enabled kit/custom rules and subagents |
| `timeouts` / `escalation` | Operation timeouts, retry/failure limits, PR-size caps |
| `review` | Score threshold, merge strategy, **multi-model consensus**, **severity gate**, JiT tests |
| `feedback` | Review feedback recording/injection |
| `lessons` | **Learning loop**: distillation/injection budget, distiller model |
| `worker` | Backend (`codex`/`claude-code`), **knowledge-graph injection** |
| `hooks` | Lifecycle shell commands |

Highlighted newer options:

```yaml
review:
  score_threshold: 7
  severity_consistency: true    # gate: score vs Critical/Important findings (default on)
  multi_model: false            # run secondary reviewers + weighted consensus
  # secondary_reviewers:
  #   - backend: claude
  #     model: opus
  #     focus_area: architecture

worker:
  backend: codex                # codex (default) | claude-code
  knowledge_graph: auto         # auto (inject when present) | off

lessons:
  enabled: true                 # learning loop: distill review rejections into reusable lessons
```

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

It runs four jobs:
- `awkit_cli` (ubuntu): `go vet`, `go test -race` with a 60% coverage threshold gate, and AWK evaluation (`awkit evaluate --offline` and `--offline --strict`)
- `awkit_cli_windows` (**windows-latest**): the full `go test ./...` suite on Windows, guarding native path/ConPTY/process behavior
- `backend` (ubuntu): `go test -race ./...` in `backend/`
- `frontend` (ubuntu): `frontend/Packages/manifest.json` JSON validation + folder checks

---

## 🧪 Evaluation

- For kit maintainers/CI only; regular users can skip.
- Standard: `docs/developer/evaluation.md`
- Executor: `awkit evaluate --offline` (report-only) and `awkit evaluate --offline --strict` (fails on any gate failure, e.g. P0 audit findings — used by CI/release checks)

---

## 📚 Documentation

### For Users

| Document | Description |
|----------|-------------|
| [Quick Start](docs/user/quick-start.md) | 5-minute setup guide |
| [Getting Started](docs/user/getting-started.md) | Detailed setup guide |
| [Configuration](docs/user/configuration.md) | Complete `workflow.yaml` reference |
| [Skills](docs/user/skills.md) | Slash-command skills reference |
| [Troubleshooting](docs/user/troubleshooting.md) | Error solutions (incl. Windows) |
| [FAQ](docs/user/faq.md) | Common questions |

### For Developers

| Document | Description |
|----------|-------------|
| [Architecture](docs/developer/architecture.md) | System internals |
| [API Reference](docs/developer/api-reference.md) | Modules & commands |
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
