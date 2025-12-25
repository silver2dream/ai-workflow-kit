# Main Loop

## Step 1: 決定下一步

呼叫決策腳本（stdout=變數, stderr=log 顯示在終端）：

```bash
eval "$(bash .ai/scripts/analyze_next.sh)"
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
bash .ai/scripts/stop_work.sh "contract_violation"
```
然後結束。

## Step 3: 根據 NEXT_ACTION 路由

| NEXT_ACTION | 動作 |
|-------------|------|
| `generate_tasks` | **Read** `tasks/generate-tasks.md`，執行任務生成 |
| `create_task` | **Read** `tasks/create-task.md`，執行 Issue 創建 |
| `dispatch_worker` | `bash .ai/scripts/dispatch_worker.sh "$ISSUE_NUMBER"` |
| `check_result` | `bash .ai/scripts/check_result.sh "$ISSUE_NUMBER"` |
| `review_pr` | **Read** `tasks/review-pr.md`，執行 PR 審查 |
| `all_complete` | `bash .ai/scripts/stop_work.sh "all_tasks_complete"` 然後結束 |
| `none` | `bash .ai/scripts/stop_work.sh "${EXIT_REASON:-none}"` 然後結束 |

## Step 4: Loop Safety

Loop Safety 由 `analyze_next.sh` 自動處理：
- 每次呼叫 analyze_next.sh 時自動 loop_count++
- 達到 MAX_LOOP (1000) 時自動返回 `NEXT_ACTION=none`
- 連續失敗達到 MAX_CONSECUTIVE_FAILURES (5) 時自動停止

無需額外操作。

## Step 5: 回到 Step 1

除非已經結束（all_complete 或 none）。
