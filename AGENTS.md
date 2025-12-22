# AGENTS.md (STRICT, LAZY, HIGH-CORRECTNESS)

You are an engineering agent working in this repository.
Default priority: correctness > minimal diff > speed.

## MUST-READ (before any work)


- Read and obey: `.ai/rules/_kit/git-workflow.md` (CRITICAL for commit format)

Do not proceed if these files are missing—stop and report what you cannot find.

---

## NON-NEGOTIABLE HARD RULES

### Start-of-work gate (MANDATORY)
- At the start of each session, run project audit once before implementing tickets:
  - `bash .ai/scripts/scan_repo.sh`
  - `bash .ai/scripts/audit_project.sh`
- If audit contains any P0/P1 findings, create/fix those tickets first.

### 0. Use existing architecture & do not reinvent
- Do not create parallel systems. Extend existing patterns.
- Keep changes minimal. Avoid wide refactors.

### 1. Always read before writing
- Search the repo for the existing pattern before adding a new one.
- Prefer local conventions (naming, folder structure, module boundaries).

### 2. Tests & verification are part of the change
- If changing logic: add/adjust tests when feasible.
- If no tests exist: add at least a small smoke test or verification note.

### 3. Git discipline
1. **Commit**: One commit per ticket unless the ticket explicitly needs multiple commits.
2. **Commit message**: Must follow `.ai/rules/_kit/git-workflow.md`.
   - Format: `[type] subject`
3. **PR**: Create PR targeting `feat/example` with "Closes #<IssueID>" in the body.
   - `main` is release-only. Target `main` ONLY when the ticket explicitly says `Release: true`.
4. **No direct pushes** to protected branches.

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

### Submodule Constraints
- Changes must stay within submodule boundary (unless `allow_parent_changes: true`)
- PRs target parent repo, not submodule remote
- Rollback reverts both submodule and parent commits

---

## DEFAULT VERIFY COMMANDS

### backend
- Build: `go build ./...`
- Test: `go test ./...`

### frontend
- Build: `echo 'Unity build via Editor'`
- Test: `echo 'Unity tests via Editor'`

