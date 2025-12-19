# AI Workflow Kit v5.0 - Tasks

## Task 1: 定義統一 schema

- [x] 1.1 建立 `.ai/config/repo_scan.schema.json`
- [x] 1.2 建立 `.ai/config/audit.schema.json`
- [x] 1.3 在 `validate_config.py` 中加入 schema 驗證

## Task 2: 統一 repo_scan 產出

- [x] 2.1 修改 `scan_repo.py` 產出符合新 schema 的 JSON
  - 加入 `root.clean`, `root.status`, `root.branch`, `root.head`
  - 加入 `presence` 區塊
  - 加入 `timestamp_utc`
- [x] 2.2 修改 `scan_repo.sh` 確保與 `.py` 產出一致
- [x] 2.3 加入測試驗證兩者產出相同 schema

## Task 3: 統一 audit 產出

- [x] 3.1 修改 `audit_project.py` 產出符合新 schema 的 JSON
  - 加入 `id` 欄位
  - 加入 `timestamp_utc`
  - 統一 `dirty_worktree` 為 P1
- [x] 3.2 修改 `audit_project.sh` 確保與 `.py` 產出一致
  - 移除 `repo`, `title`, `detail` 欄位，改用 `type`, `path`, `message`
  - 統一 `dirty_worktree` 為 P1
- [x] 3.3 加入測試驗證兩者產出相同 schema

## Task 4: Offline Gate 移除網路操作

- [x] 4.1 在 `audit_project.sh` 中將 `git fetch` 檢查移到獨立函數
- [x] 4.2 在 `evaluate.sh` 中加入 `--check-origin` 選項
- [x] 4.3 `--check-origin` 執行 submodule pinned sha 檢查
- [x] 4.4 更新 `evaluate.md` 說明 `--check-origin` 用法

## Task 5: 更新文檔

- [x] 5.1 更新 `evaluate.md` 版本至 v5.0
- [x] 5.2 更新 `evaluate.sh` 版本至 v5.0
- [x] 5.3 在版本紀錄中加入 v5.0 變更說明
- [x] 5.4 更新 Offline Gate 說明，確認無網路操作

## Task 6: 測試驗證 *

- [x] 6.1 執行 `evaluate.sh` 驗證 Offline Gate
- [x] 6.2 驗證 `audit.json` 符合 schema
- [x] 6.3 驗證 `repo_scan.json` 符合 schema
- [x] 6.4 驗證 `dirty_worktree` 在兩個版本都是 P1
