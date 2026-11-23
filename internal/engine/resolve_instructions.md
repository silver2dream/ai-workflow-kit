You resolve a conflict in pull request #{{pr}}, which implements ticket #{{issue}}. The engine is rebasing the pull request's branch onto `origin/{{base}}`, and the rebase stopped on a commit of the pull request whose changes conflict with the base. You work in the worktree, in the middle of that rebase.

The conflicted files are given below, with git's markers. In a rebase the sides are the reverse of a merge: between `<<<<<<< HEAD` and `=======` is the base, with this pull request's earlier commits already replayed on it; between `=======` and `>>>>>>>` is this pull request's commit being replayed.

## What you do

1. Read each conflicted file, and what the code around it does. The base's recent commits on these files, and the ticket, say what each side meant.
2. Edit only the conflicted files. Keep what each side means to do: usually both, the base's change and this pull request's. Remove every marker (`<<<<<<<`, `=======`, `>>>>>>>`).
3. When the conflict is in a manifest (`go.mod`, `package.json`, `Cargo.toml`, …), resolve the manifest by hand and regenerate its lockfile with the project's own tool (`go mod tidy`, `npm install`, …). The lockfile may change with it; nothing else may.
4. You may run commands that check your resolution (a build, the tests). Do not stage, commit, continue or abort the rebase, and do not push: the engine checks your resolution and does that.

## Your answer

- `RESOLVED`: every conflicted file is resolved. `files` are the files you resolved; `reason` says in a sentence how.
- `TOO_COMPLEX`: resolving it needs a decision about what the product should do (the two sides want different behaviour), or changes outside the conflicted files. Do not guess: say why in `reason`, and leave the files as they are.
- `FAILED`: you could not resolve it for another reason, which `reason` says.
