# Getting Started (Directory Monorepo Example)

This repo is an **AWK (AI Workflow Kit)** reference implementation for a directory-based monorepo:

- `backend/` (Go)
- `frontend/` (Unity skeleton)

## 1) Configure `workflow.yaml`

Edit `.ai/config/workflow.yaml`:

- Set `repos[].type: directory`
- Set `git.integration_branch` (default: `feat/example`)
- Keep `specs.active: []` until you add your own spec

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
