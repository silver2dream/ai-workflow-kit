# AI Workflow Kit v3 - Implementation Plan

## 目標
修復 v2 遺留的跨平台、路徑引用、配置一致性問題。

---

## Tasks

### P0 - 關鍵修復

- [x] 1. 入口腳本跨平台支援
  - [x] 1.1 在 kickoff.sh 加入 run_script() 函數
    - 自動選擇 .py 或 .sh 版本
    - 優先使用 Python（跨平台）
    - _Requirements: 跨平台可執行性_
  - [x] 1.2 更新 kickoff.sh 使用 run_script 呼叫 scan_repo 和 audit_project
  - [x]* 1.3 新增測試案例

- [x] 2. 路徑引用統一
  - [x] 2.1 更新 docs/ai-workflow-architecture.md
    - 將 `scripts/ai/` 改為 `.ai/scripts/`
    - _Requirements: 單一真相_
  - [x] 2.2 更新 .ai/commands/dispatch-worker.md
    - 將 `scripts/ai/` 改為 `.ai/scripts/`
  - [x] 2.3 更新 .ai/commands/stop-work.md
    - 將 `scripts/ai/` 改為 `.ai/scripts/`
  - [x] 2.4 更新 .ai/commands/review-pr.md
    - 將 `.ai/rules/git-workflow.md` 改為 `.ai/rules/_kit/git-workflow.md`
    - 移除硬編碼 `feat/aether`，改為從配置讀取
  - [x]* 2.5 新增測試案例（grep 檢查舊路徑）

- [x] 3. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. workflow.yaml 與 repo 現實一致
  - [x] 4.1 將 repos[].type 從 submodule 改為 directory
    - backend: submodule → directory
    - frontend: submodule → directory
    - _Requirements: 配置一致性_
  - [x] 4.2 移除或更新 validate-submodules.yml
    - 因為沒有 submodule，此 workflow 不需要
  - [x]* 4.3 新增測試案例

### P1 - 重要修復

- [x] 5. cleanup.sh 分支命名修復
  - [x] 5.1 更新遠端分支匹配模式
    - `origin/issue-*` → `origin/feat/ai-issue-*`
  - [x] 5.2 更新本地分支匹配模式
    - `issue-*` → `feat/ai-issue-*`
  - [x]* 5.3 新增測試案例

- [x] 6. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. 測試套件加強
  - [x] 7.1 新增腳本可執行性測試
    - 測試 scan_repo.py 可執行
    - 測試 audit_project.py 可執行
    - 測試 kickoff.sh --help
  - [x] 7.2 新增路徑引用檢查測試
    - grep 檢查不應存在 `scripts/ai/` 引用
  - [x]* 7.3 新增 CRLF 檢測測試

- [x] 8. validate_config.py 依賴處理
  - [x] 8.1 移除自動 pip install
    - 改為報錯並提示手動安裝
    - _Requirements: 受限環境相容_
  - [x]* 8.2 新增測試案例

- [x] 9. Final Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

---

## 當前進度
- 開始時間: 2025-12-19
- 完成時間: 2025-12-19
- 狀態: 完成
- 測試結果: 61 passed, 0 failed, 0 skipped
