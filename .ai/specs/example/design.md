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

## Verification Strategy

- Offline: `awkit evaluate --offline` + `go test ./...`
- CI:
  - AWK Offline + strict（只檢 P0）
  - Backend Go tests（`working-directory: backend`）
  - Frontend sanity checks（檢查 `Packages/manifest.json` 為有效 JSON、`Assets/` 存在）

