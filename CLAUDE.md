# CLAUDE.md (Principal + Worker Orchestration)

This file provides guidance for Claude Code (Principal) when working with this project.

## Project Overview

**Name:** ai-workflow
**Type:** monorepo
**Repos:** backend, frontend
## Rule Routing (IMPORTANT)

Before coding, ALWAYS identify which area the task touches, then apply the corresponding rules:

### Backend work
- Follow: `.ai/rules/git-workflow.md`, `.ai/rules/backend-go.md`
### Frontend work
- Follow: `.ai/rules/git-workflow.md`, `.ai/rules/frontend-unity.md`

### Git & PR workflow (ALWAYS)
- Follow: `.ai/rules/git-workflow.md` (commit format + PR base)

---

## Principal Autonomous Flow (MUST FOLLOW)

### Phase A — Project Audit (BEFORE tasks)

Run:
```bash
bash .ai/scripts/scan_repo.sh
bash .ai/scripts/audit_project.sh
```

Read output:
- `.ai/state/repo_scan.json`
- `.ai/state/audit.json`

Decision rule:
- If audit has any **P0/P1** findings: create fix tickets FIRST.
- Only if audit has no P0/P1 blockers, proceed to Phase B.

### Phase B — Task Selection

Read active specs:
- `.ai/specs/example/tasks.md`

Find uncompleted tasks (`- [ ]`) and create GitHub Issues.

### Phase C — Execution (Worker / Codex)

Worker takes one ticket and runs:
```bash
bash .ai/scripts/run_issue_codex.sh <id> <ticket_file> <repo>
```

Worker MUST:
1. Implement within scope
2. Verify (commands listed in ticket)
3. Commit using `[type] subject` (lowercase)
4. Push branch
5. Create PR (base: `feat/example`, release: `main`)
6. Write `.ai/results/issue-<id>.json` with PR URL

### Phase D — Review

Review PR using:
```bash
gh pr diff <PR_NUMBER>
```

Check:
- Commit format: `[type] subject`
- PR base: `feat/example`
- Changes within scope
- Architecture rules compliance

---

## Ticket Format (STRICT TEMPLATE)

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
- obey `.ai/rules/git-workflow.md`
- obey the routed architecture rules

## Plan
(steps)

## Verification
- backend: `go build ./...` and `go test ./...`
- frontend: `echo 'Unity build via Editor'` and `echo 'Unity tests via Editor'`

## Acceptance Criteria
- [ ] ...
```

---

## Quick Reference

### Start Work
```bash
bash .ai/scripts/kickoff.sh
```

### Check Status
```bash
bash .ai/scripts/stats.sh
```

### Stop Work
```bash
touch .ai/state/STOP
```

## File Locations

| What | Where |
|------|-------|
| Config | `.ai/config/workflow.yaml` |
| Scripts | `.ai/scripts/` |
| Rules | `.ai/rules/` |
| Commands | `.ai/commands/` |
| Specs | `.ai/specs/` |
| Results | `.ai/results/` |