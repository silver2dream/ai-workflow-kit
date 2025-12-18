# Git & Workflow Rules (STRICT)

## 0. Branch Model (Root + Submodules)

This project uses submodules:
- `backend` (Nakama / Go)
- `frontend` (Unity / C#)

Branches:
- **Integration branch**: `feat/aether` (daily development, **target for all PRs**)
- **Release branch**: `main` (release-only; merge from `feat/aether` when releasing)

PR base rules:
- root repo PR base: `feat/aether` (default)
- backend repo PR base: `feat/aether` (default)
- frontend repo PR base: `feat/aether` (default)

Release rule (root only):
- Only create PR from `feat/aether` -> `main` for release tickets.
- Do NOT target `main` unless the ticket explicitly says `Release: true`.

## 1. Branching Strategy

- **Feature branches**: `feat/<topic>` (e.g., `feat/ui-login`, `feat/nakama-auth`)
- **Fix branches**: `fix/<topic>`
- **Automation branches** (AI): `feat/ai-issue-<id>`

## 2. Commit Message Format (CUSTOM & STRICT)

You MUST use the bracket `[]` format. Do not use standard Conventional Commits (no colons).

- **Format**: `[<type>] <subject>`
- **Rules**:
  1. Type MUST be inside square brackets `[]`.
  2. Subject MUST be lowercase.
  3. NO colon after the bracket.
- **Allowed Types**:
  - `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`
- **Examples**:
  - ✅ `[feat] add google wire framework`
  - ✅ `[refactor] update chapter module`
  - ✅ `[chore] add agent.md for codex cli`
  - ❌ `feat: add feature` (Forbidden)

## 3. Pull Requests (MANDATORY)

- Any code change MUST go through a PR (no push-only changes).
- PR title SHOULD match commit style: `[type] subject`.
- PR body MUST include: `Closes #<IssueID>`
- Required checks must pass before merge (branch protection / rulesets).

## 4. Submodule Safety

- Root repo should not point submodules to commits that are not reachable from the submodule's allowed branches (normally `feat/aether`).
- Do NOT change submodule pinned commits unless the ticket explicitly requires it.
