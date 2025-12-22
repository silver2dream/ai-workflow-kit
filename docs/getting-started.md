# Getting Started (Directory Monorepo Example)

This repo is an **AWK (AI Workflow Kit)** reference implementation for a directory-based monorepo:

- `backend/` (Go)
- `frontend/` (Unity skeleton)

## 0) Install AWK into your repo

Recommended: install the `awkit` CLI from GitHub Releases, then run:

```bash
# In your project directory
awkit init --preset generic

# Or specify a path
awkit init /path/to/your-repo --preset generic
```

Presets:
- `generic`: single-repo template (edit to match your structure)
- `react-go`: directory monorepo template for React (frontend) + Go (backend)

Run `awkit list-presets` to see all available presets.

## 1) Configure `workflow.yaml`

Edit `.ai/config/workflow.yaml`:

- Set `repos[].type` based on your project structure:
  - `root`: Single repository (path must be `./`)
  - `directory`: Subdirectory in monorepo (shared .git)
  - `submodule`: Git submodule (independent .git)
- Set `git.integration_branch` (default: `feat/example`)
- Keep `specs.active: []` until you add your own spec

Example configurations:

```yaml
# Single repo
repos:
  - name: root
    path: ./
    type: root

# Monorepo with directories
repos:
  - name: backend
    path: backend/
    type: directory
  - name: frontend
    path: frontend/
    type: directory

# Monorepo with submodules
repos:
  - name: backend
    path: backend/
    type: submodule
```

If you don't use an integration/release branch split, set `git.integration_branch` to the same value as `git.release_branch` (for example: both `main`).

## 2) (Optional) Enable rule packs

AWK defaults to a minimal rule set under `.ai/rules/_kit/`.

If you want stricter, tech-specific rules, copy them from `.ai/rules/_examples/` into `.ai/rules/`, then enable them in `.ai/config/workflow.yaml` under `rules.custom`.

Example:

- Copy `.ai/rules/_examples/backend-go.md` → `.ai/rules/backend-go.md`
- Update `.ai/config/workflow.yaml`:

```yaml
rules:
  kit:
    - git-workflow
  custom:
    - backend-go
```

Then regenerate helper docs (recommended):

```bash
bash .ai/scripts/generate.sh
```

This will also generate `.claude/settings.local.json` with pre-approved permissions for `gh`, `git`, `codex`, and your verify commands. This enables true autopilot mode without manual approval prompts.

## 3) Add your first spec

Create a new folder under `.ai/specs/<your-feature>/` and add at least `tasks.md`.

You can use `.ai/specs/example/` as a template (it includes `requirements.md`, `design.md`, and `tasks.md`).

## 4) Run offline verification

```bash
bash .ai/scripts/evaluate.sh --offline
bash .ai/tests/run_all_tests.sh
```

## 5) Enable CI (GitHub Actions)

Add a workflow under `.github/workflows/` that runs:

- `bash .ai/scripts/evaluate.sh --offline`
- `bash .ai/tests/run_all_tests.sh`
- backend tests: `go test ./...` in `backend/`
- frontend sanity checks in `frontend/`

This repo ships with a working example workflow in `.github/workflows/ci.yml`.

If you prefer generating CI from templates, run:

```bash
bash .ai/scripts/generate.sh --generate-ci
```
