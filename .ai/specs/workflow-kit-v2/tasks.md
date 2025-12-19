# AI Workflow Kit v2 - Implementation Plan

## 目標
增強 AI Workflow Kit 的可靠性、可觀測性和跨平台支援。

---

## Tasks

### P1 - 重要功能

- [x] 1. 人工升級觸發點
  - [x] 1.1 在 workflow.yaml 新增 escalation 配置區塊
    - 新增 triggers（pattern + action）
    - 新增 max_consecutive_failures
    - 新增 max_single_pr_files / max_single_pr_lines
    - _Requirements: 安全機制_
  - [x] 1.2 更新 workflow.schema.json 加入 escalation schema
  - [x] 1.3 更新 start-work.md 加入升級檢查邏輯
    - 在 Step 5 審查時檢查 PR 大小
    - 在 Step 4 檢查失敗模式
  - [x]* 1.4 新增測試案例

- [x] 2. Checkpoint

  - Ensure all tests pass, ask the user if questions arise.



- [x] 3. 智能錯誤恢復機制
  - [x] 3.1 創建 failure_patterns.json 定義已知錯誤模式
    - compile_error, test_failure, lint_error, network_error, timeout
    - 每個模式包含 regex, type, retryable, suggestion
  - [x] 3.2 創建 analyze_failure.sh 腳本
    - 讀取失敗日誌
    - 匹配模式
    - 輸出 JSON 結果
  - [x] 3.3 更新 attempt_guard.sh 整合錯誤分析
    - 調用 analyze_failure.sh
    - 根據 retryable 決定是否重試
    - 記錄到 failure_history.jsonl





  - [x]* 3.4 新增測試案例

- [x] 4. Rollback 機制
  - [x] 4.1 創建 rollback.sh 腳本
    - 接受 PR_NUMBER 參數

    - 獲取 PR 資訊
    - 創建 revert commit





    - 創建 revert PR
    - 重新開啟原 issue
  - [x] 4.2 更新 start-work.md 加入 rollback 指引


  - [x]* 4.3 新增測試案例

- [x] 5. Checkpoint ✅
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. 分支/Worktree 清理機制
  - [x] 6.1 創建 cleanup.sh 腳本
    - 支援 --dry-run 和 --days 參數
    - 列出所有 worktrees
    - 檢查對應 PR 狀態
    - 清理已合併/關閉的 worktrees 和分支
  - [x] 6.2 更新 README 加入清理說明
  - [x]* 6.3 新增測試案例

- [x] 7. Checkpoint ✅
  - Ensure all tests pass, ask the user if questions arise.

### P2 - 改善項目

- [x] 8. 歷史趨勢追蹤
  - [x] 8.1 更新 stats.sh 追加記錄到 stats_history.jsonl
  - [x] 8.2 新增 trends 計算邏輯
    - daily_avg_closed
    - success_rate_7d
    - avg_time_to_merge
  - [x] 8.3 更新 stats.sh --json 輸出包含趨勢
  - [x]* 8.4 新增測試案例

- [x] 9. 成本追蹤
  - [x] 9.1 定義 metrics schema 在 result.json
  - [x] 9.2 更新 run_issue_codex.sh 記錄執行時間
  - [x] 9.3 更新 stats.sh 彙總成本資訊
  - [x]* 9.4 新增測試案例

- [x] 10. 任務依賴圖 (Task DAG)
  - [x] 10.1 設計 _depends_on 語法解析
  - [x] 10.2 創建 parse_tasks.py 解析 tasks.md
    - 建立依賴圖
    - 拓撲排序
    - 識別可並行任務
  - [x] 10.3 更新 start-work.md 使用依賴圖選擇任務
  - [x]* 10.4 新增測試案例

- [x] 11. 跨 Repo 協調
  - [x] 11.1 擴展 ticket 格式支援多 Repo
  - [x] 11.2 更新 start-work.md 處理 multi-repo tickets
  - [x] 11.3 實作 sequential 和 parallel 執行策略
  - [x]* 11.4 新增測試案例

- [x] 12. Windows 原生支援
  - [x] 12.1 創建 scan_repo.py 跨平台版本
  - [x] 12.2 創建 audit_project.py 跨平台版本
  - [x] 12.3 更新腳本入口點自動選擇 .sh 或 .py
  - [x]* 12.4 新增跨平台測試

- [x] 13. Final Checkpoint
  - All 45 tests pass ✅

---

## 當前進度
- 開始時間: 2025-12-19
- 完成時間: 2025-12-19
- 狀態: 完成
- 已完成: 所有 P1 和 P2 任務（共 13 項）
- 測試: 45 tests passing

