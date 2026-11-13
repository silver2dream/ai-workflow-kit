# Conflict Resolution

`awkit dispatch-worker` 印出 `WORKER_STATUS=needs_conflict_resolution` 時使用：rebase 停在要讀程式才解得開的衝突上，worktree 還在 rebase 中途。issue 這時仍帶 `in-progress` 標籤，要由你依 subagent 的結果改掉；不改的話下一輪只會一直回報 `check_result`。

## Steps

1. 從同一段輸出讀出 `WORKTREE_PATH`、`ISSUE_NUMBER`、`PR_NUMBER`。
2. 交給 `conflict-resolver` subagent：`subagent_type` 填 `conflict-resolver`，`prompt` 寫 `Resolve merge conflict. WORKTREE_PATH=<path> ISSUE_NUMBER=<n> PR_NUMBER=<n>`。不要自己進 worktree 解衝突。
3. 依它最後一行的結果改標籤，然後照「接著」那一欄做：

| 結果 | 標籤 | 接著 |
|---|---|---|
| `RESOLVED` | 移除 `in-progress`、`merge-conflict`，加上 `pr-ready` | 回到主迴圈 |
| `TOO_COMPLEX` | 移除 `in-progress`，加上 `needs-human-review`、`merge-conflict`；在 issue 留言寫出哪些檔案衝突、為什麼要人決定 | `awkit stop-workflow needs_human_review` |
| `FAILED` 或其他 | 移除 `in-progress`，加上 `merge-conflict` | 回到主迴圈；下一輪會再派一次 |

```bash
# RESOLVED
gh issue edit <n> --remove-label in-progress,merge-conflict --add-label pr-ready

# TOO_COMPLEX
gh issue edit <n> --remove-label in-progress --add-label needs-human-review,merge-conflict
gh issue comment <n> --body "<哪些檔案衝突、為什麼需要人決定>"

# FAILED
gh issue edit <n> --remove-label in-progress --add-label merge-conflict
```

標籤名稱是預設值；`.ai/config.yaml` 的 `github.labels` 改過的話，用設定裡的名稱。
