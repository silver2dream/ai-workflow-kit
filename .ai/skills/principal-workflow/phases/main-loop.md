# Main Loop

## Step 1: 決定下一步

執行決策命令並獲取 JSON 輸出：

```bash
awkit analyze-next --json
```

輸出 JSON 包含：
- `next_action`: generate_tasks | create_task | dispatch_worker | check_result | review_pr | all_complete | none
- `issue_number`, `pr_number`, `spec_name`, `task_line`, `exit_reason`
- `merge_issue`: conflict | rebase（當 Worker 需要處理 merge 問題時）

**重要**：解析 JSON 輸出，記住這些值用於後續步驟。

## Step 2: 根據 next_action 路由

根據 `next_action` 的值執行對應動作：

| next_action | 動作 |
|-------------|------|
| `generate_tasks` | **Read** `tasks/generate-tasks.md`，執行任務生成 |
| `create_task` | **Read** `tasks/create-task.md`，使用 `spec_name` 和 `task_line` 執行 Issue 創建 |
| `dispatch_worker` | 執行 dispatch（**必須先檢查 merge_issue**）⚠️ **同步等待** |
| `check_result` | 執行 `awkit check-result --issue <issue_number>` |
| `review_pr` | 調用 `pr-reviewer` subagent（見下方詳細說明） |
| `all_complete` | 執行 `awkit stop-workflow all_tasks_complete` 然後結束 |
| `none` | 執行 `awkit stop-workflow <exit_reason>` 然後結束 |

### ⚠️ CRITICAL: dispatch_worker 命令格式（MANDATORY CHECK）

當 `next_action` 為 `dispatch_worker` 時，**必須**按以下步驟執行：

**Step 1: 檢查 merge_issue 欄位（不可跳過）**

從 JSON 輸出中讀取 `merge_issue` 的值。

**Step 2: 根據 merge_issue 選擇命令格式**

| merge_issue 值 | 命令格式 |
|---------------|----------|
| `conflict` 或 `rebase` | `awkit dispatch-worker --issue <N> --merge-issue <merge_issue> --pr <pr_number>` |
| 空或不存在 | `awkit dispatch-worker --issue <N>` |

**範例**：
```json
{"next_action": "dispatch_worker", "issue_number": 27, "pr_number": 30, "merge_issue": "conflict"}
```
→ 執行：`awkit dispatch-worker --issue 27 --merge-issue conflict --pr 30`

⚠️ **WARNING**: 忽略 merge_issue 會導致 merge conflict/rebase 無法修復，造成無限循環！

### ⚠️ CRITICAL: review_pr 必須使用 Task Tool

當 `next_action` 為 `review_pr` 時，**你必須使用 Task tool 調用 pr-reviewer subagent**。

**絕對禁止**：
- ❌ 直接執行 `awkit prepare-review` 命令
- ❌ 直接執行 `awkit submit-review` 命令
- ❌ 自己讀取 PR 代碼進行審查
- ❌ 自己撰寫 review body

**你必須做的**：使用 Task tool，設定以下參數：
- `subagent_type`: `"pr-reviewer"`
- `description`: `"Review PR #<pr_number>"`
- `prompt`: `"Review PR #<pr_number> for Issue #<issue_number>"`

Subagent 會獨立執行完整審查流程並返回結果：
- `merged`: PR 已合併
- `changes_requested`: 審查不通過
- `review_blocked`: Evidence 驗證失敗
- `merge_failed`: 合併失敗（如 conflict）

**收到結果後，直接回到 Step 1**，不要嘗試修正或重試。

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
5. **執行完成後，檢查 WORKER_STATUS 並處理（見下方）**

### dispatch_worker 結果處理

執行 `awkit dispatch-worker` 後，解析輸出的 `WORKER_STATUS`：

| WORKER_STATUS | 動作 |
|---------------|------|
| `success` | 回到 Step 1 |
| `failed` | 回到 Step 1（下輪會重試或標記 worker-failed） |
| `needs_conflict_resolution` | 調用 conflict-resolver subagent（見下方） |

### ⚠️ 處理 needs_conflict_resolution

當 `WORKER_STATUS=needs_conflict_resolution` 時，表示自動 rebase 發現實際衝突需要 AI 解決。

**Step 1**: 從輸出中讀取以下變數：
- `WORKTREE_PATH`: worktree 路徑
- `ISSUE_NUMBER`: Issue 編號
- `PR_NUMBER`: PR 編號

**Step 2**: 使用 Task tool 調用 conflict-resolver subagent：

使用 Task tool，設定以下參數：
- `subagent_type`: `"conflict-resolver"`
- `description`: `"Resolve conflict for Issue #<n>"`
- `prompt`: `"Resolve merge conflict. WORKTREE_PATH=<path> ISSUE_NUMBER=<n> PR_NUMBER=<n>"`

**Step 3**: 根據 subagent 返回結果執行對應動作：

| 結果 | 動作 |
|------|------|
| `RESOLVED` | 1. 移除 `in-progress` 和 `merge-conflict` 標籤<br>2. 添加 `pr-ready` 標籤<br>3. 回到 Step 1 |
| `TOO_COMPLEX` | 1. 移除 `in-progress` 標籤<br>2. 添加 `needs-human-review` 和 `merge-conflict` 標籤<br>3. 在 Issue 添加評論說明需要人工介入<br>4. 執行 `awkit stop-workflow needs_human_review` |
| `FAILED` 或其他 | 1. 移除 `in-progress` 標籤<br>2. 添加 `merge-conflict` 標籤<br>3. 回到 Step 1（下輪會重試） |

**標籤操作範例**：
```bash
# RESOLVED 後
gh issue edit <issue_number> --remove-label in-progress,merge-conflict
gh issue edit <issue_number> --add-label pr-ready

# TOO_COMPLEX 後
gh issue edit <issue_number> --remove-label in-progress
gh issue edit <issue_number> --add-label needs-human-review,merge-conflict
gh issue comment <issue_number> --body "Merge conflict 過於複雜，需要人工介入解決"

# FAILED 後
gh issue edit <issue_number> --remove-label in-progress
gh issue edit <issue_number> --add-label merge-conflict
```

## Step 3: Loop Safety

Loop Safety 由 `awkit analyze-next` 自動處理：
- 每次呼叫時自動 loop_count++
- 達到 MAX_LOOP (1000) 時自動返回 `next_action=none`
- 連續失敗達到 MAX_CONSECUTIVE_FAILURES (5) 時自動停止

無需額外操作。

## Step 4: 回到 Step 1

除非已經結束（`all_complete` 或 `none`）。
