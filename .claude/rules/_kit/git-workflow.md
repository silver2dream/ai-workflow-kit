# Git & Workflow Rules

Scope: every branch, commit and pull request of this project. `awkit generate` writes this file from `.ai/config.yaml`: change the configuration, not this file.

## 1. Branches

- Integration branch: `develop`. Every pull request targets it, whichever repo the change is in.
- Release branch: `main`. Nothing targets it except a release: a pull request from `develop`, or the pull request of a ticket whose `awk-ticket` block sets `release: true`.
- Working branches: `feat/<topic>` or `fix/<topic>`. The workflow names its own `feat/ai-issue-<id>`.

## 2. Commit messages

- Format: `[type] subject`
- `type` is one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`.
- The type is in square brackets with no colon after them, and the subject is lowercase. Conventional Commits (`feat: add feature`) is not the format here.
- Example: `[feat] add login endpoint`

## 3. Pull requests

- A change reaches `develop` or `main` through a pull request, never by a direct push.
- The title is in the commit format.
- The body contains `Closes #<issue>`, which ties the pull request to its ticket.
- It is merged once the required checks have passed.

## 4. Rule files

- The files under `.claude/rules/_kit/` come with AWK and are rewritten when it is upgraded. The files beside that directory are the project's own.
- A rule file holds rules: what is required, what is forbidden, what to check. The state of the project is not a rule; point to where it is defined.
- When a rule disagrees with the code or the git log, the code and the log are right, and the rule file is corrected.
