# AI Workflow Kit v3.1 - Implementation Plan

## 目標
修復 evaluate.md 評估框架與實際實作之間的不一致問題。

---

## Tasks

### P0 - 關鍵修復

- [x] 1. Python 腳本落盤
  - [x] 1.1 更新 scan_repo.py 寫入 .ai/state/repo_scan.json
    - 執行後自動產生 state 文件
    - stdout 輸出保持不變
    - _Requirements: Offline Gate O2 一致性_
  - [x] 1.2 更新 audit_project.py 寫入 .ai/state/audit.json
    - 執行後自動產生 state 文件
    - stdout 輸出保持不變
    - _Requirements: Offline Gate O4 一致性_
  - [x]* 1.3 新增測試案例

- [x] 2. 創建 evaluate.sh
  - [x] 2.1 實作 evaluate.sh
    - 包含 Offline Gate 檢查
    - 包含 Online Gate 檢查（含 rollback）
    - 支援 --offline / --online 參數
    - _Requirements: 文檔一致性_
  - [x]* 2.2 新增測試案例

- [x] 3. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

### P1 - 重要修復

- [x] 4. 加強配置一致性檢查
  - [x] 4.1 在 validate_config.py 加入 type-specific 驗證
    - submodule: 檢查 .gitmodules 存在且包含該 path
    - directory: 檢查是目錄且不是獨立 git repo
    - root: 檢查 path 是 ./
    - _Requirements: R2 配置一致性_
  - [x]* 4.2 新增測試案例

- [x] 5. 更新 evaluate.md
  - [x] 5.1 加入前置條件章節
    - 列出 Python 依賴
    - 提供安裝指令
  - [x] 5.2 統一 Online Gate 檢查項目
    - 確保表格和腳本一致
    - 加入 rollback 檢查

- [x] 6. Final Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

---

## 當前進度
- 開始時間: 2025-12-19
- 狀態: 完成
- 已完成: 全部 (75 tests pass, Offline Gate PASS)
