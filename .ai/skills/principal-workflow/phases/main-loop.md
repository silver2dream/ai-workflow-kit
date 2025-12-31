# Main Loop

## Step 1: 決定下一步

執行決策命令並獲取 JSON 輸出：

```bash
awkit analyze-next --json
```

輸出 JSON 包含：
- `next_action`: generate_tasks | create_task | dispatch_worker | check_result | review_pr | all_complete | none
- `issue_number`, `pr_number`, `spec_name`, `task_line`, `exit_reason`

**重要**：解析 JSON 輸出，記住這些值用於後續步驟。

## Step 2: 根據 next_action 路由

根據 `next_action` 的值執行對應動作：

| next_action | 動作 |
|-------------|------|
| `generate_tasks` | **Read** `tasks/generate-tasks.md`，執行任務生成 |
| `create_task` | **Read** `tasks/create-task.md`，使用 `spec_name` 和 `task_line` 執行 Issue 創建 |
| `dispatch_worker` | 執行 `awkit dispatch-worker --issue <issue_number>` ⚠️ **同步等待** |
| `check_result` | 執行 `awkit check-result --issue <issue_number>` |
| `review_pr` | **Read** `tasks/review-pr.md`，使用 `pr_number` 執行 PR 審查 |
| `all_complete` | 執行 `awkit stop-workflow all_tasks_complete` 然後結束 |
| `none` | 執行 `awkit stop-workflow <exit_reason>` 然後結束 |

### check_result 狀態說明

| 狀態 | 含義 | 系統行為 |
|------|------|----------|
| `success` | Worker 成功完成 | 繼續 review_pr |
| `crashed` | Worker 異常終止 | 自動移除 in-progress，可重試 |
| `timeout` | Worker 超時 (30分鐘) | 自動移除 in-progress，可重試 |
| `not_found` | 結果未就緒 | 已等待 30 秒，回到 Step 1 |
| `failed_will_retry` | 失敗但未超過重試上限 | 移除 in-progress，下輪重試 |
| `failed_max_retries` | 超過重試上限 (3次) | 標記 worker-failed，需人工介入 |

Principal 收到任何狀態都直接回到 Step 1，Go 命令會自動處理恢復邏輯。

## ⚠️ CRITICAL: dispatch_worker 行為規範

執行 `awkit dispatch-worker` 時：
1. **命令是同步的** - 會等待 Worker 完成才返回
2. **不要讀取 log 檔案** - 這會浪費 context
3. **不要監控進度** - 命令會處理一切
4. **不要輸出 Worker 狀態描述** - 等命令完成即可
5. **執行完成後，回到 Step 1**

## Step 3: Loop Safety

Loop Safety 由 `awkit analyze-next` 自動處理：
- 每次呼叫時自動 loop_count++
- 達到 MAX_LOOP (1000) 時自動返回 `next_action=none`
- 連續失敗達到 MAX_CONSECUTIVE_FAILURES (5) 時自動停止

無需額外操作。

## Step 4: 回到 Step 1

除非已經結束（`all_complete` 或 `none`）。
