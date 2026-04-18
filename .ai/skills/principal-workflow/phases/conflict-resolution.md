# Conflict Resolution

## 處理 needs_conflict_resolution

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
| `RESOLVED` | 1. 移除 `in-progress` 和 `merge-conflict` 標籤<br>2. 添加 `pr-ready` 標籤<br>3. 回到 main-loop Step 1 |
| `TOO_COMPLEX` | 1. 移除 `in-progress` 標籤<br>2. 添加 `needs-human-review` 和 `merge-conflict` 標籤<br>3. 在 Issue 添加評論說明需要人工介入<br>4. 執行 `awkit stop-workflow needs_human_review` |
| `FAILED` 或其他 | 1. 移除 `in-progress` 標籤<br>2. 添加 `merge-conflict` 標籤<br>3. 回到 main-loop Step 1（下輪會重試） |

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

處理完成後，回到 main-loop Step 1。
