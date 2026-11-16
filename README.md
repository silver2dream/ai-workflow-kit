# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

[English](README.md) | [繁體中文](README-zh-TW.md)

AWK turns a brief into merged pull requests. A council of model-played roles argues the brief into documents and a list of tickets; you approve the list; then a **Principal** (Claude Code) hands each ticket to a **Worker** (Codex or Claude Code), has the pull request reviewed, and merges what passes. The tickets are GitHub issues, and what counts as "passes" is checked by code, not taken from a model's word.

```
awkit plan "…"  ──►  documents + decisions + proposed tickets  ──►  you approve
                                                                        │
awkit run  ──►  next ticket ──► Worker writes the change ──► review ──► merge
                     ▲                                          │
                     └──────── sent back with what was found ───┘
```

## What you need

| | |
|---|---|
| `git`, and a GitHub repository as the project's `origin` | |
| [`gh`](https://cli.github.com/), logged in with write access to that repository | the workflow lives in its issues and pull requests |
| [`claude`](https://claude.com/claude-code) (Claude Code) | plays the Principal, the reviewer and the council |
| `codex`, or `claude` again | plays the Worker (`worker.backend`, `codex` unless you change it) |
| On Windows: Git for Windows | Claude Code needs its Git Bash, and the build and test commands run through it |

`awkit doctor` checks all of this in a project and says what is missing.

## Install

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex
```

The command is `awkit`, so that it does not clash with the system's `awk`.

## The first run

In the repository you want worked on:

```bash
awkit init                       # or: awkit init --preset godot-go --scaffold
git add -A && git commit -m "[chore] set up awkit" && git push
awkit doctor                     # the tools, the remote, the configuration
awkit plan "A daily login reward: seven days, the reward grows, a missed day starts over"
awkit plan approve <plan>        # creates the issues
awkit run                        # works through them
```

| Preset | What you get |
|---|---|
| `generic` (default) | One repository of any stack. You set its build and test commands in `.ai/config.yaml`. |
| `godot-go` | `backend/` in Go and `frontend/` in Godot C#, with rules for both. `--scaffold` adds a skeleton that builds and passes its tests. |

`awkit init` writes the configuration with the branches your repository has, the kit under `.claude/`, the guides (`CLAUDE.md`, `AGENTS.md`) and a CI workflow for every repo that has build or test commands. It ends by saying what it wrote and what to do next.

While it works:

```bash
awkit status     # the run, every ticket by its stage, what to do next
awkit stop       # ask the run to stop once the step it is on is done
```

A run ends when no ticket is left, or when something needs you: a Worker that failed on every attempt, a review that keeps turning a change back, a ticket that waits for an issue that will not be done. It says which, and `awkit status` shows the ticket.

[Getting started](docs/user/getting-started.md) goes through these steps one by one.

## Planning: the council

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

Nothing is on GitHub yet. Read the work list, change what you want changed, then `awkit plan approve <plan>`.

- **What only you can answer.** A plan does not wait for a person: where the roles need an answer only you have, the deciding role states what the plan assumes, and the end of the plan lists the assumptions. To be asked instead, start with `awkit plan --ask "…"`: the council puts at most seven questions to you and argues from your answers.
- **The next plan builds on this one.** Every role reads the decisions on record and its own documents as they stand, and the planning role is shown what earlier plans ticketed.
- **The roster is yours to change:** the roles, what each stands for, the documents each owns, the model that plays it, how many model calls a plan may make. A plan that stopped half way goes on with `awkit plan --resume <plan>`.

## Tickets

A ticket is a GitHub issue with the `ai-task` label and a small block that says which repo it is for and what it waits for. `awkit plan approve` creates them; you can also write one yourself:

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
awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md
awkit run --issue 42   # work on one ticket only
```

Tickets are taken in the order of their numbers, each once the issues it depends on are done. The block is read strictly: an unknown field, or a repo the configuration does not have, is refused before the issue is created. The full format is in `.claude/skills/principal-workflow/references/ticket.md`.

## What "passes" means

- **The review is a form, not prose.** The reviewer hands in a structured `review.json`; a mistake in its form is corrected in the same session, and only a mistake in its evidence costs a new review.
- **The evidence is checked.** awkit runs the tests again and checks that every acceptance criterion is tied to a test that ran and passed.
- **CI has to be green**, and the score and the findings have to agree with each other.
- **Your word outranks the review.** Review a pull request on GitHub while a run is going (with write access to the repository): it is not merged over your open review comments, the Worker is sent back with them, and they are kept as lessons. A merge never waits for you; what you write after it is picked up before the next ticket.
- Optional: several models review and their scores are combined (`review.multi_model`); tests are generated from the diff at review time (`review.jittest`).

What a review turned back is distilled, once the change is finally approved, into a short lesson that later Workers and reviewers are shown when they touch the same files. Lessons are kept in `.ai/state/lessons.json`, which is local to a checkout and not committed; `awkit lessons list` shows them, and `awkit lessons promote` turns a proven one into an issue that asks for a rule.

## What it keeps in your project

```
.ai/
  config.yaml        the configuration: repos, branches, build and test commands
  roster.yaml        who sits on the planning council
  state/             what runs leave behind: results, logs, lessons (ignored by git)
.claude/
  rules/_kit/        the kit's rule (branches, commits, pull requests), generated
  rules/             your project's rules; the godot-go preset starts you with three
  skills/            what the Principal follows: the loop, the ticket format
  agents/            the subagents it hands reviews and conflicts to
CLAUDE.md, AGENTS.md   what agents are told, generated from the configuration
docs/                  what the council writes: documents, decisions.md, plans/
.github/workflows/     CI, one workflow per repo, generated from the configuration
```

A CI workflow follows the configuration until you delete the comment at its top; from then on it is yours. Your own rules, skills and local Claude Code settings are never written over.

## Commands

| | |
|---|---|
| `awkit init` | set a project up |
| `awkit doctor` | check everything a run needs, and say how to fix what is missing |
| `awkit generate`, `awkit validate` | after the configuration changed: write the guides and the CI workflows again; check the file |
| `awkit plan` | plan with the council; `awkit plan approve <plan>` creates the tickets |
| `awkit create-task` | add one ticket by hand |
| `awkit run` | work through the open tickets (`--issue N` for one, `--dry-run` for the checks only) |
| `awkit status` | where the work is |
| `awkit stop` | ask the run to stop |
| `awkit events`, `awkit lessons` | what a run did; what the reviews taught |
| `awkit upgrade` | bring a project's kit files up to the installed version |
| `awkit reset` | clear what earlier runs left behind |
| `awkit uninstall` | take AWK out of a project |

`awkit help <command>` says more; `awkit help --all` lists the commands the workflow itself runs.

## Configuration

`.ai/config.yaml`. A new project's file is this short, and a key that is left out has its default:

```yaml
version: "1.2"

project:
  name: "demo"
  type: "monorepo"              # or single-repo

repos:
  - name: backend
    path: backend/
    type: directory             # root | directory | submodule
    language: go
    verify:                     # what the Worker and the review run
      build: "go build ./..."
      test: "go test ./..."

git:
  integration_branch: "develop" # pull requests are merged here
  release_branch: "main"        # the same branch, in a project with one trunk

rules:
  custom: [backend]             # .claude/rules/backend.md

worker:
  backend: codex                # or claude-code

review:
  score_threshold: 7
  merge_strategy: squash
```

After changing it, `awkit generate` writes the guides and the CI workflows again, and `awkit validate` checks the file. It also names the keys in the file that nothing reads, so a setting that does nothing does not pass for one that does. Every key is described in [docs/user/configuration.md](docs/user/configuration.md).

## Upgrading

```bash
awkit check-update     # is a newer awkit out
awkit upgrade          # bring this project's kit files up to the installed awkit
```

`awkit upgrade` keeps your configuration, rules and skills, and commits what it changed. A project set up by an earlier awkit keeps its files elsewhere (`.ai/config/workflow.yaml`, `.ai/rules/`, `.ai/skills/`); the upgrade moves them, and `awkit upgrade --dry-run` shows every step first.

## Documentation

| | |
|---|---|
| [Getting started](docs/user/getting-started.md) | the first run, step by step |
| [Configuration](docs/user/configuration.md) | every key of `config.yaml`, and the roster |
| [Skills and rules](docs/user/skills.md) | what agents are told, and how to add your own |
| [Troubleshooting](docs/user/troubleshooting.md), [FAQ](docs/user/faq.md) | when something does not work |
| [Architecture](docs/developer/architecture.md), [Contributing](docs/developer/contributing.md) | how awkit itself is built |

The documents under `docs/` are written in Traditional Chinese.

## License and security

[Apache License 2.0](LICENSE). Vulnerabilities are reported as [SECURITY.md](SECURITY.md) says; the repository is watched by [OpenSSF Scorecard](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit).
