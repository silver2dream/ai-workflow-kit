# Quick Start Guide

Get AWKit running in 5 minutes. This guide covers the fastest path from zero to `awkit run`.

---

## Prerequisites

Install these **before** anything else:

| Tool | Install | Why |
|------|---------|-----|
| **GitHub CLI (`gh`)** | [cli.github.com](https://cli.github.com/) | Issue/PR operations (required) |
| **`gh auth login`** | Run after installing `gh` | **#1 first-time blocker** — authenticate now |
| **Claude Code (`claude`)** | [claude.ai/download](https://claude.ai/download) | Principal AI agent |
| **Codex (`codex`)** | per Codex docs | Default Worker backend (`worker.backend: codex`) — needed for `awkit run` to dispatch work; skip if you set `worker.backend: claude-code` |
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

# Any stack: a single repo, you name the build and test commands
awkit init

# Go backend + Godot C# frontend, with a project skeleton
awkit init --preset godot-go --scaffold

# Preview without writing files
awkit init --preset godot-go --scaffold --dry-run
```

There are two presets, `generic` (the default) and `godot-go`; `awkit list-presets` describes them. With `generic`, set `verify.build` and `verify.test` in `.ai/config.yaml` before the first run.

---

## Step 3: Configure config.yaml

Open `.ai/config.yaml` and verify the basics:

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
  integration_branch: "develop"           # PR target branch
```

Key points:
- `repos[].name` is what a ticket names as its `repo`; the repository root is always available as `root`
- `repos[].verify` holds the commands the Worker, the review and CI run
- `git.integration_branch` is the PR target (not `main`)

---

## Step 4: Plan the work, or write a ticket

The work is a queue of tickets. A ticket is a GitHub issue with the task label (`ai-task`), and its body says what to do and, in a small block, which repo it is for.

For anything larger than one ticket, let the council plan it:

```bash
awkit plan "A health endpoint for the API server: build version and database state"
awkit plan approve <plan>    # after you have read docs/plans/<plan>/tickets.yaml
```

The roles of `.ai/roster.yaml` argue the brief out, write the documents they own under `docs/`, put their decisions on record in `docs/decisions.md` and propose the tickets. Nothing is created on GitHub until you approve the list. With `--ask` the council puts its questions to you before it argues; without it, it says what it assumed. After the approval, go on with Step 5 and `awkit run`.

For one piece of work, write the ticket yourself.

Write the body to a file, for example `ticket.md`:

````markdown
## Summary
Add a health endpoint.

## Scope
- GET /healthz in the API server

## Acceptance Criteria
- [ ] GET /healthz answers 200 with the build version
- [ ] Unit tests added for new functionality

## Testing Requirements
- A unit test covers the handler, the happy path and the error cases

```yaml awk-ticket
schema: 1
repo: root
```
````

The `yaml awk-ticket` block:

| Field | |
|-------|---|
| `schema` | always `1` |
| `repo` | a repo name from `config.yaml`, or `root` |
| `depends_on` | optional: issues that must be done first, e.g. `[12, 15]` |

Acceptance criteria describe behaviour, not test names: the review checks that each one is covered by a test that ran.

---

## Step 5: Generate and validate

```bash
awkit generate       # generates CLAUDE.md, AGENTS.md, rules, settings
awkit validate       # checks config.yaml for errors
awkit doctor         # full health check (CLI tools, config, permissions)
```

Fix any issues `doctor` reports before continuing.

---

## Step 6: Create the ticket and run

```bash
# Create the issue. The body is checked first; nothing is created from one that would not work
awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md

# Pre-flight checks only (no side effects)
awkit run --dry-run

# Work through every open ticket
awkit run

# Or one ticket only
awkit run --issue 42
```

For each ticket, in the order of the issue numbers and once what it depends on is done, AWKit will:
1. Dispatch a Worker to implement it and open a pull request
2. Review the pull request: run the tests, check every acceptance criterion against a test that ran
3. Merge it, or send the Worker back with what the review found

The run ends when no ticket is left, or when something needs a person. `awkit status` says where things stand.

---

## Stopping the workflow

```bash
touch .ai/state/STOP
```

The Principal will finish its current task and stop gracefully.

To resume later:

```bash
rm .ai/state/STOP
awkit run
```

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `gh not authenticated` | Run `gh auth login` |
| `ticket body has no yaml awk-ticket block` | Add the block to the ticket body (Step 4) |
| `Workflow stopped (invalid_ticket)` | The issue has a comment that says what is wrong with its block. Correct it, remove the `needs-human-review` label, run again |
| `Workflow stopped (blocked_by_dependencies)` | Every ticket waits for an issue in its `depends_on` that is not done. Finish or reopen that issue, or correct `depends_on` |
| `Workflow stopped (no_actionable_tasks)` | There is no open ticket to work on. Create one (Step 6) |
| `repo path does not exist` | Check `repos[].path` in config.yaml |
| `Working directory not clean` | Commit or stash your changes |

For more: [Troubleshooting](troubleshooting.md) | [FAQ](faq.md) | [Configuration Reference](configuration.md)
