# Git & Workflow Rules (STRICT)

## 0. Branch Model

This project uses directories (monorepo without submodules):
- `backend/` - backend (go)
- `frontend/` - frontend (unity)

Branches:
- **Integration branch**: `feat/example` (daily development, **target for all PRs**)
- **Release branch**: `main` (release-only; merge from `feat/example` when releasing)

PR base rules:
- backend repo PR base: `feat/example` (default)
- frontend repo PR base: `feat/example` (default)

Release rule (root only):
- Only create PR from `feat/example` -> `main` for release tickets.
- Do NOT target `main` unless the ticket explicitly says `Release: true`.

## 1. Branching Strategy

- **Feature branches**: `feat/<topic>`
- **Fix branches**: `fix/<topic>`
- **Automation branches** (AI): `feat/ai-issue-<id>`

## 2. Commit Message Format (CUSTOM & STRICT)

You MUST use the bracket `[]` format. Do not use standard Conventional Commits (no colons).

- **Format**: `[type] subject`
- **Rules**:
  1. Type MUST be inside square brackets `[]`.
  2. Subject MUST be lowercase.
  3. NO colon after the bracket.
- **Allowed Types**:
  - `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`
- **Examples**:
  - ✅ `[feat] add new feature`
  - ✅ `[refactor] update module structure`
  - ✅ `[chore] add configuration file`
  - ❌ `feat: add feature` (Forbidden)

## 3. Pull Requests (MANDATORY)

- Any code change MUST go through a PR (no push-only changes).
- PR title SHOULD match commit style: `[type] subject`.
- PR body MUST include: `Closes #<IssueID>`
- Required checks must pass before merge (branch protection / rulesets).
