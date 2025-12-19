# AI Workflow Kit v2 - Design Document

## Overview

AI Workflow Kit v2 是對現有工作流系統的增強，專注於提高可靠性、可觀測性和跨平台支援。

### 目標
- 提高系統可靠性（錯誤恢復、回滾機制）
- 增強可觀測性（歷史追蹤、成本監控）
- 改善開發體驗（任務依賴、人工升級觸發）
- 跨平台支援（Windows 原生支援）

---

## P1 - 重要功能

### 1. 智能錯誤恢復機制

**現狀問題：**
- `attempt_guard.sh` 只有簡單的重試計數
- 沒有分析失敗原因
- 相同錯誤不會學習避免

**設計：**

```
.ai/scripts/
├── analyze_failure.sh    # 分析失敗類型
└── failure_patterns.json # 已知失敗模式庫

.ai/state/
└── failure_history.jsonl # 失敗歷史記錄
```

**失敗類型分類：**
- `compile_error` - 編譯錯誤（語法、類型）
- `test_failure` - 測試失敗
- `lint_error` - Lint 錯誤
- `network_error` - 網路問題（可重試）
- `timeout` - 超時
- `unknown` - 未知錯誤

**analyze_failure.sh 邏輯：**
```bash
# 輸入：失敗日誌
# 輸出：JSON { type, retryable, suggestion }

# 1. 解析錯誤日誌
# 2. 匹配 failure_patterns.json 中的模式
# 3. 返回分類和建議
```

### 2. 任務依賴圖 (Task DAG)

**現狀問題：**
- tasks.md 的任務是線性執行
- 無法並行執行獨立任務
- 無法表達依賴關係

**設計：**

在 tasks.md 中支援依賴標記：
```markdown
- [ ] 1. 建立資料庫 schema
- [ ] 2. 實作 API endpoint
  - _depends_on: 1_
- [ ] 3. 實作前端 UI
  - _depends_on: 2_
- [ ] 4. 寫測試
  - _depends_on: 1_  # 只依賴 1，可與 2,3 並行
```

**解析邏輯：**
- 讀取 tasks.md
- 建立依賴圖
- 拓撲排序
- 識別可並行的任務

**執行策略：**
- 單 Worker：按拓撲順序執行
- 多 Worker：並行執行無依賴的任務

### 3. Rollback 機制

**現狀問題：**
- PR 合併後發現問題，沒有自動化回滾

**設計：**

```bash
# .ai/scripts/rollback.sh <PR_NUMBER>

# 1. 獲取 PR 資訊
gh pr view <PR_NUMBER> --json mergeCommit,headRefName,body

# 2. 創建 revert commit
git revert <merge_commit> --no-edit

# 3. 創建 revert PR
gh pr create --title "Revert: <original_title>" ...

# 4. 從 PR body 提取原 issue 編號
# 5. 重新開啟原 issue
gh issue reopen <ISSUE_NUMBER>

# 6. 通知
bash .ai/scripts/notify.sh "Rollback PR #<PR_NUMBER>"
```

### 4. 跨 Repo 協調

**現狀問題：**
- 每個 ticket 只能指定一個 Repo
- 無法處理需要同時改 backend + frontend 的任務

**設計：**

Ticket 格式擴展：
```markdown
- Repo: backend, frontend
- Coordination: sequential  # sequential | parallel
- Sync: required           # required | independent
```

**執行策略：**
- `sequential`: 先完成 backend，再做 frontend
- `parallel`: 同時執行（需要多 Worker）
- `sync: required`: 兩個 PR 必須同時合併
- `sync: independent`: 可以分開合併

### 5. 人工升級觸發點

**現狀問題：**
- 什麼情況下 Principal 應該停下來問人不明確
- 只有連續 3 個 issue 失敗才停

**設計：**

在 workflow.yaml 新增配置：
```yaml
escalation:
  triggers:
    - pattern: "security|vulnerability|credential"
      action: "pause_and_ask"
    - pattern: "delete|drop|truncate"
      action: "require_human_approval"
  max_consecutive_failures: 3
  max_single_pr_files: 50  # 太大的 PR 要人審
  max_single_pr_lines: 500
```

---

## P2 - 改善項目

### 6. 歷史趨勢追蹤

**設計：**

```
.ai/state/
├── stats_history.jsonl  # 每次執行追加一行
└── trends.json          # 計算後的趨勢數據
```

**stats_history.jsonl 格式：**
```json
{"timestamp":"2025-01-15T10:00:00Z","issues":{"total":10,"closed":8},"prs":{"merged":5,"rejected":1}}
```

**trends.json 格式：**
```json
{
  "daily_avg_closed": 3.5,
  "success_rate_7d": 0.85,
  "avg_time_to_merge": "2h30m"
}
```

### 7. 成本追蹤

**設計：**

在 result.json 中新增：
```json
{
  "issue_id": 123,
  "status": "success",
  "pr_url": "...",
  "metrics": {
    "duration_seconds": 180,
    "api_calls": {
      "codex": { "calls": 3, "tokens_in": 10000, "tokens_out": 5000 },
      "claude": { "calls": 1, "tokens_in": 3000, "tokens_out": 2000 }
    },
    "estimated_cost_usd": 0.45
  }
}
```

### 8. 分支/Worktree 清理

**設計：**

```bash
# .ai/scripts/cleanup.sh [--dry-run] [--days 7]

# 1. 列出所有 worktrees
git worktree list

# 2. 檢查對應的 PR 狀態
# 3. 如果 PR 已合併/關閉超過 N 天，清理
# 4. 清理遠端分支
git push origin --delete <branch>
```

### 9. Windows 原生支援

**設計：**

為關鍵腳本提供 Python 跨平台版本：
```
.ai/scripts/
├── scan_repo.sh      # Unix
├── scan_repo.py      # 跨平台
├── audit_project.sh
├── audit_project.py
└── ...
```

**Python 版本優先級：**
1. 如果在 Windows 且沒有 bash → 使用 .py
2. 否則使用 .sh

---

## 實作優先順序

1. **P1.5 人工升級觸發點** - 最重要的安全機制
2. **P1.1 智能錯誤恢復** - 提高自動化成功率
3. **P1.3 Rollback 機制** - 安全網
4. **P2.8 清理機制** - 維護性
5. **P1.2 任務依賴圖** - 效率提升
6. **P2.6 歷史追蹤** - 可觀測性
7. **P2.7 成本追蹤** - 可觀測性
8. **P1.4 跨 Repo 協調** - 進階功能
9. **P2.9 Windows 原生** - 跨平台

---

## 測試策略

每個功能都需要：
1. 單元測試（在 `.ai/tests/` 中）
2. 整合測試（模擬完整流程）
3. 文檔更新

