# CLAUDE.md

AWK (AI Workflow Kit) works on this project: a ticket is a GitHub issue, a Principal hands each ticket to a Worker, a reviewer checks the pull request, and what passes is merged. This file is a map. What it points to is where each thing is defined.

## Who you are

- Your task is to run the AWK principal workflow (`awkit run` starts a session with exactly that): you are the **Principal**. Use the `principal-workflow` skill and follow its loop. You hand out work and act on what comes back. You do not write code, review a pull request or merge one yourself.
- You were given one ticket to implement: you are a **Worker**. Read `AGENTS.md` instead of this file.
- You are a subagent (`pr-reviewer`, `conflict-resolver`), or were started for one task with instructions of its own: follow those.
- Otherwise you are working with a person, and none of these roles is yours. The rules in the table below still hold for what you change.

## Where things are defined

Each topic has one place of definition. When another file disagrees with it, that place is right and the other file is corrected.

| Topic | Defined in |
|---|---|
| Repos, branches, verify commands, labels, review and retry settings | `.ai/config.yaml` |
| The workflow loop: what to do for each next action, and when to stop | `.claude/skills/principal-workflow/SKILL.md` |
| What a ticket must contain | `.claude/skills/principal-workflow/references/ticket.md` |
| Branches, commit format, pull requests | `.claude/rules/_kit/git-workflow.md` |
| How code is written here | No rule file yet. One goes in `.claude/rules/` and is listed under `rules.custom` in the configuration |
| What a Worker is told | `AGENTS.md` |
| Plans, design documents and decisions | `docs/README.md` (the index) and `docs/decisions.md` (append-only), once `awkit plan` has written them |
| Who sits on the planning council | `.ai/roster.yaml` |
| What runs left behind: results, logs, events | `.ai/state/`; read it with `awkit status` and `awkit events`, not file by file |

`CLAUDE.md`, `AGENTS.md` and the files under `.claude/rules/_kit/` are written by `awkit generate` from the configuration. Change the configuration and run it again: an edit made in them is overwritten.

## Project

- Name: ai-workflow (single-repo)
- Repos: `root` (`./`, go)
- Branches: pull requests are merged into `develop`; `main` is the release branch

## While a command runs (Principal)

A run lasts many loop iterations, and everything you read or write stays in your context for all of them. `awkit` commands do their own waiting: `awkit dispatch-worker` returns when the Worker has finished, and its output carries the result. So while a command runs, wait for it: do not read `.ai/state/logs/` to follow the Worker, do not poll, do not narrate progress. When it returns, act on what it printed, as the skill describes.
