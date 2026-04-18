# Example Spec - Design (Directory Monorepo)

## Overview

這個 example 以 **同一個 Git repo 內的兩個子目錄** 作為目標：

- `backend/`：Go（可在 CI 中跑 `go test ./...`）
- `frontend/`：Unity 專案骨架（CI 僅做結構/JSON sanity，不需要 Unity Editor）

AWK 的配置使用 `type: directory`，表示這兩個子目錄不是獨立 git repo，也不是 submodule。

## Workflow.yaml (concept)

- `repos[]` 包含 `backend` 與 `frontend`
- `git.integration_branch` 為 `feat/example`
- `specs.active` 預設為空，避免 clone 後直接產生 Issue/PR

## Step Dependencies

Define execution order and dependencies between tasks. The `Depends On` column
controls which tasks must complete before a given step can start. The Principal
uses this table to sequence issue creation and worker dispatch.

| Step | Description | Depends On | Acceptance Criteria |
|------|-------------|------------|---------------------|
| 1 | Backend: add health check implementation | - | Health function returns stable payload, unit tests pass |
| 2 | Frontend: show health status (stub) | Step 1 | Placeholder entrypoint exists, localization keys added |
| 3 | CI sanity | Steps 1,2 | Root CI runs AWK offline + tests, backend go tests, frontend sanity |
| 4 | Checkpoint | Step 3 | `awkit evaluate --offline` and `go test ./...` pass |

### Dependency rules

- **No dependency** (`-`): the task can start immediately.
- **Single dependency** (`Step N`): the task starts only after step N is done.
- **Multiple dependencies** (`Steps N,M`): all listed steps must complete first.
- Steps without unmet dependencies may run in parallel when workers are available.

## Verification Strategy

- Offline: `awkit evaluate --offline` + `go test ./...`
- CI:
  - AWK Offline + strict（只檢 P0）
  - Backend Go tests（`working-directory: backend`）
  - Frontend sanity checks（檢查 `Packages/manifest.json` 為有效 JSON、`Assets/` 存在）
