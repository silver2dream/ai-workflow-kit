# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Bash](https://img.shields.io/badge/Bash-required-4EAA25?logo=gnubash&logoColor=white)]()
[![GitHub CLI](https://img.shields.io/badge/gh-required-181717?logo=github&logoColor=white)](https://cli.github.com/)

> An AI-assisted development workflow kit that drives **Ticket → Implement → Review → Merge** with a **self-improving** review loop. The work is a queue of GitHub issues; a **Principal** (Claude Code) orchestrates a **Worker** (Codex or Claude Code), and quality is enforced by deterministic Go gates — not by trusting LLM prose.

[![Download](https://img.shields.io/badge/Download-Latest%20Release-brightgreen?style=for-the-badge&logo=github)](https://github.com/silver2dream/ai-workflow-kit/releases/latest)

[English](README.md) | [繁體中文](README-zh-TW.md)

---

## 🌟 What Makes AWK Different

Most AI workflow tools are a dispatch loop that trusts whatever the model says. AWK is built on two ideas that set it apart:

### 🧠 A Self-Improving Learning Loop

AWK **learns from its own review history**. Every rejection runs through a four-step loop — **record → distill → inject → verify**:

- When a rejected PR is finally approved, an LLM **distills** the case — what the reviewer objected to and what the fix changed — into a compact, **committable lesson** (`.ai/state/lessons.json`): a durable check, not a raw log line.
- Relevant lessons are **injected** into future Worker/Reviewer prompts (scoped to the files you're touching, via the same relevance engine that powers knowledge-graph grounding).
- Each lesson is **settled hit/miss** against real outcomes — counted only for PRs that touched the lesson's scope — and promoted through `candidate → active → proven`; ineffective lessons retire automatically.
- A **proven** lesson can be promoted (`awkit lessons promote`) into a **hard gate** (rule / audit check) via a human-reviewed issue.
- **A person's correction is the strongest signal.** Review a PR on GitHub while the workflow runs: AWK will not merge over your open comments, sends the Worker back with them, and distills them into lessons that are trusted from the start.

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
- **Planning by argument**: `awkit plan` has the roles of a roster argue a brief out, write the documents each owns, record the decisions with who held otherwise, and propose the tickets. Nothing is created until you approve the work list
- **Issues are the work**: a ticket is a GitHub issue with the task label and a small `yaml awk-ticket` block that names its repo and the tickets it waits for. No spec files, no task lists to keep in sync
- **In order, by dependency**: tickets run in the order of their numbers, each once what it depends on has landed; `awkit run --issue N` does one ticket and stops
- **GitHub as state machine**: uses issues/PRs + labels to track progress
- **Dispatch + review loop**: dispatches the implementation to a Worker, reviews the PR, then merges it or sends the Worker back with what the review found
- **Worker backend selection**: `codex` (default) or `claude-code` via `worker.backend`

### Review Quality
- **Structured review submission**: reviewer submits `review.json` (`submit-review --body-file`); schema errors are corrected in-session, only real evidence failures block
- **Evidence verification gate**: re-runs the test suite and verifies each acceptance criterion maps to a passing test with a real assertion before merge
- **Multi-model consensus** (opt-in): `review.multi_model` runs secondary reviewers in parallel and applies weighted scoring with an `[ERROR]`-severity cap — enforced in Go, not agent-side math
- **Severity/verdict consistency gate**: a below-threshold score must carry a Critical/Important finding; a passing score must not carry a Critical one
- **JiT tests** (opt-in): generates independent tests from the PR diff at review time

### Learning Loop
- **Record → distill → inject → verify**: rejections that end in an approval are distilled, together with the fix, into compact, committable lessons (`.ai/state/lessons.json`), injected into future Worker/Reviewer prompts and settled hit/miss against the PRs they applied to
- **Promotable to hard gates**: `awkit lessons promote` opens a human-gated issue to harden a proven lesson into a rule/audit check

### Context Grounding
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
│  You ──► awkit run ──► Claude Code (Principal)                │
│                              │                               │
│                              ├─► pick the next ticket         │
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
- `dotnet` 8 SDK — builds and tests a C# / Godot frontend (the `godot-go` preset)

### Online / E2E (optional)
- `gh` (GitHub CLI) + `gh auth login`
- `claude` (Claude Code) — required for the Principal, and for the Worker when `worker.backend: claude-code`
- `codex` — required for the Worker when `worker.backend: codex` (default)

---

## 📁 Project Structure

```
.
├── .ai/
│   ├── config.yaml              # the project's configuration
│   ├── roster.yaml              # who sits at the table when `awkit plan` plans
│   └── state/                   # what a run leaves behind: results, logs (gitignored)
├── .claude/                     # what agents read, where Claude Code looks for it
│   ├── rules/_kit/              # the kit's rules; your own rules go next to this folder
│   ├── skills/                  # what the Principal follows: the loop, the ticket format
│   └── agents/                  # the subagents the Principal hands work to
├── docs/                        # the council's documents, decisions.md and plans/
├── CLAUDE.md, AGENTS.md         # instructions for the Principal and the Worker (generated)
└── .github/workflows/           # CI, one workflow per repo, generated from the config
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
# Initialize AWK in the current directory: any stack, you name the build and test commands
awkit init

# Go backend + Godot C# frontend, with a project skeleton that builds and passes its tests
awkit init --preset godot-go --scaffold

# Preview what would be created
awkit init --preset godot-go --scaffold --dry-run
```

### Available Presets

| Preset | What you get |
|--------|--------------|
| `generic` (default) | A single repo of any stack. You set `verify.build` and `verify.test` in `.ai/config.yaml`; nothing else is assumed. |
| `godot-go` | A monorepo: `backend/` in Go, `frontend/` in Godot C#. With `--scaffold`, a skeleton that builds and passes its tests (.NET 8 SDK to build, Godot 4 .NET to open the game). |

Run `awkit list-presets` for details. See [Getting Started](docs/getting-started.md) for the skeleton's files.

`awkit init` also writes a CI workflow for every repo that has build or test commands, from the same commands the Worker verifies with: `.github/workflows/ci.yml` for a single repo, `ci-<repo>.yml` for each directory repo. A workflow keeps following the config until you delete the comment at its top; from then on it is yours and awkit leaves it alone.

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

Update kit files inside a project (preserves your config.yaml):

```bash
awkit upgrade
```

A project set up by an earlier awkit keeps its files elsewhere (`.ai/config/workflow.yaml`, `.ai/rules/`, `.ai/skills/`, `.ai/results/` and so on). `awkit upgrade` moves them to where they are kept now and commits the move; `awkit upgrade --dry-run` shows every step first. Until a project is upgraded, the other commands stop and say so.

Other update options:

```bash
# Apply a different preset to config.yaml only
awkit init --preset godot-go --force-config

# Upgrade kit files and overwrite config.yaml (requires --preset)
awkit upgrade --force-config --preset godot-go

# Full reset: update kit files AND apply preset to config.yaml
awkit init --preset godot-go --force
```

### 1) Generate outputs

```bash
awkit generate
```

### 2) Plan the work: the council

```bash
awkit plan "A daily login reward: seven days, the reward grows, a missed day starts over"
```

The roles of `.ai/roster.yaml` argue the brief out. Each one states where it stands; they object to each other; the objections that matter most go back and forth between the two roles concerned; a moderator who takes no side says where things stand; the deciding role rules on what is still disputed. Then every role writes the documents it owns, and the planning role draws up the tickets, which the table gets to object to once.

The order of play is in code, not in a prompt: awkit asks each role in turn, counts the rounds and the model calls, and is the only one that writes files, so a role cannot write a document another role owns.

| What a plan leaves | |
|---|---|
| `docs/plans/<plan>/minutes.md` | who said what: positions, objections, each dispute and how it ended |
| `docs/decisions.md` | the decisions, each under a name that stays (`D7`), with who held otherwise, and what the plan assumes (`A3`); append-only |
| `docs/<area>/*.md` | the documents, each written by the one role that owns it |
| `docs/plans/<plan>/tickets.yaml` | the proposed tickets, in working order |

Nothing is on GitHub yet. Read the work list, change what you want changed, then:

```bash
awkit plan approve <plan>    # creates the issues, each after the ones it depends on
```

**What only you can answer.** A plan never waits for a person: where the roles need an answer only you have, the deciding role states what the plan assumes, the assumption goes on record under its own name, and the end of the plan lists them. To be asked instead, start the plan with `--ask`: a minute or so into the meeting the council puts its questions to you, at most seven, and argues from your answers. A question you skip is left to the council to assume.

```bash
awkit plan --ask "A daily login reward: seven days, the reward grows, a missed day starts over"
```

**The next plan builds on this one.** Every role reads the decisions on record and its own documents as they stand. It changes what the new brief changes, and leaves a document the brief does not touch as it is. The planning role is shown what earlier plans ticketed, so the same work is not ticketed twice. A new ticket can wait for an issue that exists (`depends_on: ["#12"]`), or for a ticket of an earlier plan that is not on GitHub yet (`depends_on: ["<plan>/<key>"]`); `awkit plan approve` then tells you to approve that plan first.

The roster is yours to change: the roles, what each stands for, the documents each owns, the model that plays it, how many model calls a plan may make. `awkit init` writes one that fits the preset: a small game studio for `godot-go`. A plan that stops half way goes on with `awkit plan --resume <plan>`; what was answered is kept, and a work list you changed by hand stays as you left it.

### 3) Tickets and the run

The work is a queue of tickets: GitHub issues with the task label, each with a `yaml awk-ticket` block in its body. `awkit plan approve` creates them from the council's work list; you can also write one yourself:

````markdown
## Summary
Add a health endpoint.

## Scope
- GET /healthz in the backend

## Acceptance Criteria
- [ ] GET /healthz answers 200 with the build version

## Testing Requirements
- A unit test covers the handler

```yaml awk-ticket
schema: 1
repo: backend          # a repo of .ai/config.yaml, or root
depends_on: [12]       # optional: issues that must be done first
```
````

```bash
gh auth login

awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md
awkit run --dry-run    # pre-flight checks only
awkit run              # work through every open ticket
awkit run --issue 42   # or one ticket only
```

The run stops when no ticket is left, or when something needs a person: a ticket whose block cannot be used, a Worker that gave up, a ticket that waits for an issue that will not be done. `awkit status` says where things stand.

Stop:

```bash
touch .ai/state/STOP
```

---

## ⚙️ Configuration

Main config: `.ai/config.yaml`

### Repo type

AWK supports three repository types configured in `.ai/config.yaml`:

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

### Tickets

A ticket's `yaml awk-ticket` block is read strictly. An unknown field, or a repo the config does not have, is refused: by `awkit create-task` before the issue exists, by the workflow with a comment on the issue otherwise.

| Field | Meaning |
|-------|---------|
| `schema` | always `1` |
| `repo` | a repo of `config.yaml`, or `root` |
| `depends_on` | issues that must be done before this ticket starts |
| `severity` | `P0` to `P3` (optional) |
| `release` | `true` sends the PR to the release branch (root repo only) |
| `allow_script_changes`, `allow_parent_changes`, `allow_secrets` | safety flags; off unless the ticket sets them |

Full description: `.claude/skills/principal-workflow/tasks/create-task.md`.

### Config sections

`config.yaml` has these top-level sections (full reference: [docs/user/configuration.md](docs/user/configuration.md)):

| Section | Purpose |
|---------|---------|
| `project` / `repos` | Repo layout, types, per-repo `verify` commands |
| `git` | Integration/release branches, commit format, PR template |
| `audit` | Audit checks |
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
  human_feedback: true          # a maintainer's PR review outranks the automated verdict (default on)
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

## 📦 Project Skeleton (`godot-go`)

`awkit init --preset godot-go --scaffold` writes a project that builds and passes its tests as it is:

- `backend/`: a Go module with a health handler and its test (`go build ./...`, `go test ./...`)
- `frontend/`: a Godot C# solution (`dotnet build`, `dotnet test`)
  - `game/`: the Godot project
  - `src/Game.Core/`: game logic that does not depend on the engine
  - `tests/Game.Core.Tests/`: xUnit tests of that logic
- `.github/workflows/ci-backend.yml`, `ci-frontend.yml`: CI that runs those same commands

This repository's own CI initializes such a project on every push and runs its generated workflows' commands, so the skeleton, the preset and the CI generator cannot drift apart.

- Guide: `docs/getting-started.md`

---

## 🔁 CI

**For user projects:**
- `awkit init` writes a CI workflow for every repo that has build or test commands, from the commands in `config.yaml`
- `awkit upgrade` and `awkit generate` keep those workflows in step with the config. A workflow whose first-line comment you delete is yours, and awkit leaves it alone

**For this repo (awkit itself):**
`.github/workflows/ci.yml` is written by hand, so `awkit generate` does not touch it. It runs three jobs:
- `awkit_cli` (ubuntu): `go vet`, `go test -race` with a 60% coverage threshold gate, and AWK evaluation (`awkit evaluate --offline` and `--offline --strict`)
- `awkit_cli_windows` (**windows-latest**): the full `go test ./...` suite on Windows, guarding native path/ConPTY/process behavior
- `skeleton` (ubuntu): initializes a `godot-go` project with `--scaffold`, then builds and tests it with the commands of the workflows awkit generated for it

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
| [Configuration](docs/user/configuration.md) | Complete `config.yaml` reference |
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
- Branch model and commit format: `.claude/rules/_kit/git-workflow.md`
- PRs target `develop`; `main` is for releases.

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
