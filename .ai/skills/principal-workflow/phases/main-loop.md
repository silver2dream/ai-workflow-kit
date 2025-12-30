# Main Loop

## Step 1: 決定下一步

呼叫決策命令：

```bash
eval "$(awkit analyze-next)"
```

輸出變數：
- `NEXT_ACTION`: generate_tasks | create_task | dispatch_worker | check_result | review_pr | all_complete | none
- `ISSUE_NUMBER`, `PR_NUMBER`, `SPEC_NAME`, `TASK_LINE`, `EXIT_REASON`

## Step 2: 驗證變數契約

**Read** `references/contracts.md` 確認必填欄位。

| NEXT_ACTION | 必填 |
|-------------|------|
| generate_tasks | - |
| create_task | SPEC_NAME, TASK_LINE |
| dispatch_worker | ISSUE_NUMBER |
| check_result | ISSUE_NUMBER |
| review_pr | PR_NUMBER |
| all_complete / none | - |

如果必填為空，執行：
```bash
awkit stop-workflow contract_violation
```
然後結束。

## Step 3: 根據 NEXT_ACTION 路由

| NEXT_ACTION | 動作 |
|-------------|------|
| `generate_tasks` | **Read** `tasks/generate-tasks.md`，執行任務生成 |
| `create_task` | **Read** `tasks/create-task.md`，執行 Issue 創建 |
| `dispatch_worker` | `eval "$(awkit dispatch-worker --issue $ISSUE_NUMBER)"` ⚠️ **同步等待** |
| `check_result` | `eval "$(awkit check-result --issue $ISSUE_NUMBER)"` |
| `review_pr` | **Read** `tasks/review-pr.md`，執行 PR 審查 |
| `all_complete` | `awkit stop-workflow all_tasks_complete` 然後結束 |
| `none` | `awkit stop-workflow "${EXIT_REASON:-none}"` 然後結束 |

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

執行 `dispatch_worker` 時：
1. **腳本是同步的** - 會等待 Worker 完成才返回
2. **不要讀取 log 檔案** - 這會浪費 context
3. **不要監控進度** - 腳本會處理一切
4. **不要輸出 Worker 狀態描述** - 等腳本返回 `WORKER_STATUS` 即可
5. **執行後直接 eval 結果，回到 Step 1**

## Step 4: Loop Safety

Loop Safety 由 `awkit analyze-next` 自動處理：
- 每次呼叫時自動 loop_count++
- 達到 MAX_LOOP (1000) 時自動返回 `NEXT_ACTION=none`
- 連續失敗達到 MAX_CONSECUTIVE_FAILURES (5) 時自動停止

無需額外操作。

## Step 5: 回到 Step 1

除非已經結束（all_complete 或 none）。
