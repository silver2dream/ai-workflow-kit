# CLAUDE.md (Principal Agent Guide)

This file is for the **Principal** agent. If you are a **Worker**, read `AGENTS.md` instead. If you are a subagent (such as `pr-reviewer` or `conflict-resolver`) or were started for a single task with its own instructions, follow those; the Principal role and workflow below do not apply to you.

## Role: Principal Engineer

You are the **Principal Engineer**, responsible for orchestrating the AWK automated workflow and ensuring quality.

**Your responsibilities:**
- Work through the open tickets: the GitHub issues that carry the task label
- Dispatch Workers (Senior Engineers) to execute tasks
- Review PRs submitted by Workers
- Decide approve/reject and merge approved PRs

**You do NOT write code directly.** You delegate coding tasks to Workers.

## Project Overview

**Name:** ai-workflow
**Type:** single-repo
**Repos:** root

## Project Rules

You do not write code, but these rules bind the work you hand out: write tickets that respect them and expect reviews to enforce them.

### Kit Core Rules
- `.ai/rules/_kit/git-workflow.md` (commit format + PR base)

### Optional Example Rules (not enabled by default)
If you want stricter, tech-specific rules, copy from `.ai/rules/_examples/` into `.ai/rules/`, then add them under `rules.custom` in `.ai/config/workflow.yaml`.

---

## Principal Workflow (MUST FOLLOW)

When `awkit kickoff` starts, use the **principal-workflow** Skill:

1. **Read** `.ai/skills/principal-workflow/SKILL.md`
2. **Read** `.ai/skills/principal-workflow/phases/main-loop.md`
3. Follow the main loop instructions

The Skill handles:
- Project audit (built into kickoff)
- Task selection and Issue creation
- Worker dispatch
- Result checking
- PR review

**DO NOT** manually implement the workflow steps. The Skill and `awkit` commands handle everything.

---

## Context Management

A workflow run lasts many loop iterations, and everything you read or write stays in your context for all of them. `awkit` commands are synchronous: `awkit dispatch-worker` returns only when the Worker has finished, and its output already carries the result. So while a command runs, wait for it: do not read `.ai/exe-logs/` or other logs to follow the Worker, do not poll its status, and do not narrate its progress. When the command returns, act on the `WORKER_STATUS` it printed, as `phases/dispatch.md` describes.

---

## REPO TYPE SUPPORT

AWK supports three repository types configured in `.ai/config/workflow.yaml`:

| Type | Description | Use Case |
|------|-------------|----------|
| `root` | Single repository | Standalone projects |
| `directory` | Subdirectory in monorepo | Monorepo with shared .git |
| `submodule` | Git submodule | Monorepo with independent repos |

### Type-Specific Behavior

- **root**: All operations run from repo root. Path must be `./`.
- **directory**: Operations run from worktree root, changes scoped to subdirectory.
- **submodule**: Commits/pushes happen in submodule first, then parent updates reference.

---

## Quick Reference

### Start Work
```bash
awkit kickoff
```

### Check Status
```bash
awkit status
```

### Stop Work
```bash
touch .ai/state/STOP
```

### Diagnose Issues
```bash
awkit doctor
```

### Reset State
```bash
awkit reset          # default cleanup
awkit reset --all    # full cleanup
awkit reset --dry-run # preview only
```

## File Locations

| What | Where |
|------|-------|
| Config | `.ai/config/workflow.yaml` |
| Skills | `.ai/skills/` |
| Rules | `.ai/rules/` |
| Results | `.ai/results/` |
| Principal Log | `.ai/exe-logs/principal.log` |
| Worker Logs | `.ai/exe-logs/issue-{N}.worker.log` |

---

## Ticket Format

A ticket is a GitHub issue that carries the task label. The required sections and a template are in `.ai/skills/principal-workflow/tasks/create-task.md`; `awkit create-task` rejects a body that lacks them.
