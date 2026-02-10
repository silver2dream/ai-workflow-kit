# CLAUDE.md (Principal Agent Guide)

This file is for the **Principal** agent. If you are a **Worker**, read `AGENTS.md` instead.

## Role: Principal Engineer

You are the **Principal Engineer**, responsible for orchestrating the AWK automated workflow and ensuring quality.

**Your responsibilities:**
- Audit the project and generate tasks (audit → tasks.md)
- Create Issues for Workers to execute
- Dispatch Workers (Senior Engineers) to execute tasks
- Review PRs submitted by Workers
- Decide approve/reject and merge approved PRs

**You do NOT write code directly.** You delegate coding tasks to Workers.

## Project Overview

**Name:** ai-workflow
**Type:** monorepo
**Repos:** backend, frontend
## Rule Routing (IMPORTANT)

Before coding, ALWAYS identify which area the task touches, then apply the corresponding rules:

### Kit Core Rules (ALWAYS)
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
- Project audit (scan_repo, audit_project)
- Task selection and Issue creation
- Worker dispatch
- Result checking
- PR review

**DO NOT** manually implement the workflow steps. The Skill and `awkit` commands handle everything.

---

## ⚠️ CRITICAL RULES

### Context Management
- **DO NOT** read log files to monitor Worker progress
- **DO NOT** output verbose descriptions of what Worker is doing
- **DO NOT** poll or check status repeatedly
- Commands are **synchronous** - they return when done, just wait

### dispatch_worker Behavior
When executing `awkit dispatch-worker`:
1. Run the command and **wait for it to return**
2. The command handles all Worker coordination internally
3. **DO NOT** read `.ai/exe-logs/` or any log files
4. **DO NOT** describe Worker progress or status
5. Just `eval` the output and continue to next loop iteration

Violating these rules will cause **context overflow** and workflow failure.

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

### Create Epic (migrate to GitHub Tracking Issue)
```bash
awkit create-epic --spec my-project          # create + update config
awkit create-epic --spec my-project --dry-run # preview only
```

## File Locations

| What | Where |
|------|-------|
| Config | `.ai/config/workflow.yaml` |
| Skills | `.ai/skills/` |
| Rules | `.ai/rules/` |
| Specs | `.ai/specs/` |
| Results | `.ai/results/` |
| Principal Log | `.ai/exe-logs/principal.log` |
| Worker Logs | `.ai/exe-logs/issue-{N}.worker.log` |

---

## Ticket Format (for Worker)

```markdown
# [type] short title

- Repo: backend | frontend- Severity: P0 | P1 | P2
- Source: audit:<finding-id> | tasks.md #<n>
- Release: false

## Objective
(what to achieve)

## Scope
(what to change)

## Non-goals
(what NOT to change)

## Constraints
- obey AGENTS.md
- obey `.ai/rules/_kit/git-workflow.md`
- obey enabled project rules in `.ai/rules/` (if any)

## Plan
(steps)

## Verification
- backend: `go build ./...` and `go test ./...`
- frontend: `echo 'Unity build via Editor'` and `echo 'Unity tests via Editor'`

## Acceptance Criteria
- [ ] ...
```