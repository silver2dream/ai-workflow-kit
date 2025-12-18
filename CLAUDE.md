# CLAUDE.md (Principal + Worker Orchestration)

This repo contains BOTH:
- Unity client (C#) using R3 + UniTask + MessagePipe + UI Toolkit + Unity Localization + Firebase
- Nakama backend (Go) with modular runtime (MongoDB/Postgres/Redis/JWT) and AI gateway

## Rule Routing (IMPORTANT)
Before coding, ALWAYS identify which area the task touches, then apply the corresponding rules:

1) Unity / Frontend work
- Follow: `.claude/rules/unity-architecture-and-patterns.md`
- If converting React/Tailwind -> UI Toolkit, ALSO follow:
  `.claude/rules/ui-toolkit-react-to-uxml.md`

2) Backend / Nakama work
- Follow: `.claude/rules/backend-nakama-architecture-and-patterns.md`

3) Git & PR workflow (ALWAYS)
- Follow: `.claude/rules/git-workflow.md` (commit format + PR base)

---

## Principal Autonomous Flow (MUST FOLLOW)

### Phase A — Project Audit (BEFORE tasks.md)
Proof the project is healthy enough to proceed (this is NOT a greenfield repo).

Run:
- `bash scripts/ai/scan_repo.sh`
- `bash scripts/ai/audit_project.sh`

Read output:
- `.ai/state/repo_scan.json`
- `.ai/state/audit.json`

Decision rule:
- If audit has any **P0/P1** findings: create fix tickets FIRST (do not touch tasks.md yet).
- Only if audit has no P0/P1 blockers, proceed to Phase B.

### Phase B — Ticketization (audit/tasks.md -> executable tickets)
Run:
- `bash scripts/ai/audit_to_tickets.sh`

Output:
- `tasks/queue/issue-<id>.md` (each ticket is directly runnable by Worker)

### Phase C — Execution (Worker / Codex)
Worker takes one ticket and runs:
- root ticket: `bash scripts/ai/run_issue_codex.sh <id> tasks/queue/issue-<id>.md root`
- backend ticket: `bash scripts/ai/run_issue_codex.sh <id> tasks/queue/issue-<id>.md backend`
- frontend ticket: `bash scripts/ai/run_issue_codex.sh <id> tasks/queue/issue-<id>.md frontend`

Worker MUST:
1) implement within scope
2) verify (commands listed in ticket)
3) commit using `[type] subject` (lowercase)
4) push branch
5) create PR (base defaults to `feat/aether`, release tickets target `main`)
6) write `.ai/results/issue-<id>.json` with PR URL

---

## Ticket Format (STRICT TEMPLATE)

Every ticket file `tasks/queue/issue-<id>.md` MUST follow:

# [type] short title

- Repo: root | backend | frontend
- Severity: P0 | P1 | P2
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
- obey `.claude/rules/git-workflow.md`
- obey the routed architecture rules (see Rule Routing above)

## Plan
(steps)

## Verification
(commands that must pass)

## Acceptance Criteria
- [ ] ...

---

## Worker Prompt (Codex CLI) (NO PLACEHOLDERS)

Use this message emphasising strict scope + PR + verification:

'You are an automated coding agent running inside a git worktree.

Repo rules:
- Read and follow CLAUDE.md and AGENTS.md.
- Keep changes minimal and within scope.
- Do not change submodule pinned commits unless ticket requires it.
- Use commit format: [type] subject (lowercase).
- Create PR and include "Closes #<IssueID>" in PR body.

Ticket:
<PASTE THE WHOLE TICKET CONTENT>

After changes:
- Print: git status --porcelain
- Print: git diff
- Run verification commands from ticket
- Ensure PR base is correct (feat/aether by default)'
