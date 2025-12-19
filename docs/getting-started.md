# Getting Started (Directory Monorepo Example)

This repo is an **AWK (AI Workflow Kit)** reference implementation for a directory-based monorepo:

- `backend/` (Go)
- `frontend/` (Unity skeleton)

## 1) Configure `workflow.yaml`

Edit `.ai/config/workflow.yaml`:

- Set `repos[].type: directory`
- Set `git.integration_branch` (default: `feat/example`)
- Keep `specs.active: []` until you add your own spec

## 2) Add your first spec

Create a new folder under `.ai/specs/<your-feature>/` and add at least `tasks.md`.

You can use `.ai/specs/example/` as a template (it includes `requirements.md`, `design.md`, and `tasks.md`).

## 3) Run offline verification

```bash
bash .ai/scripts/evaluate.sh --offline
bash .ai/tests/run_all_tests.sh
```

## 4) Enable CI (GitHub Actions)

Add a workflow under `.github/workflows/` that runs:

- `bash .ai/scripts/evaluate.sh --offline`
- `bash .ai/tests/run_all_tests.sh`
- backend tests: `go test ./...` in `backend/`
- frontend sanity checks in `frontend/`

This repo ships with a working example workflow in `.github/workflows/ci.yml`.

