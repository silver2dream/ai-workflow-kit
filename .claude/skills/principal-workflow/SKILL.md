---
name: principal-workflow
description: 執行 AWK 的主迴圈：問 awkit 下一步 → 派工、檢查結果或交付審查 → 再問，直到沒有工作或需要人處理。由 awkit run 啟動的 Principal 使用；要補一張票券時，讀它的 references/ticket.md。
allowed-tools: Read, Grep, Glob, Bash
---

# Principal Workflow

下一步由 `awkit analyze-next` 決定，寫程式的是 Worker，審查與解衝突的是 subagent；你照結果把它們接起來。進度記在 GitHub 的標籤與 `.ai/state/`，不靠你的對話記得。

## Loop

1. 執行 `awkit analyze-next --json`，讀出 `next_action`、`issue_number`、`pr_number`、`merge_issue`、`exit_reason`。
2. 依 `next_action` 做下表的一件事。
3. 回到 1。只有執行了 `awkit stop-workflow` 才結束。

| `next_action` | 做什麼 |
|---|---|
| `dispatch_worker` | 見 Dispatch |
| `check_result` | `awkit check-result --issue <issue_number>`；不論印出什麼狀態都回到 1 |
| `review_pr` | 見 Review |
| `all_complete` | `awkit stop-workflow all_tasks_complete`，結束 |
| `none` | `awkit stop-workflow <exit_reason>`，結束 |

## Dispatch

- 指令是 `awkit dispatch-worker --issue <issue_number>`。`merge_issue` 有值（`conflict` 或 `rebase`）時加上 `--merge-issue <merge_issue> --pr <pr_number>`；少了它，Worker 不知道要處理合併問題，同一張票會一再被派回來。
- 指令等 Worker 做完才返回，可能要幾十分鐘，結果在它印出的 `WORKER_STATUS`。等的時候不讀 log、不查進度、不描述進度。
- 工具回報指令被移到背景時，等它完成的通知再讀輸出；不要停掉它，也不要再派一次。
- `WORKER_STATUS=needs_conflict_resolution`：讀 `references/conflict-resolution.md` 照做。其他狀態都回到 1；失敗的票要不要重試，由 `awkit analyze-next` 決定。

## Review

- 把審查交給 `pr-reviewer` subagent：`subagent_type` 填 `pr-reviewer`，`prompt` 寫 `Review PR #<pr_number> for Issue #<issue_number>`。
- 不要自己執行 `awkit prepare-review`、`awkit submit-review`，也不要自己讀 diff 或寫審查意見：票是你派的，自己審等於自己驗收自己。
- subagent 回報 `merged`、`changes_requested`、`review_blocked` 或 `merge_failed`。不論哪一個都回到 1，不重試、不自己修；後續由 `awkit analyze-next` 安排。

## Guardrails

- 這次執行沒有人在看。需要人決定的事不要提問：執行 `awkit stop-workflow needs_human_review` 結束。指令出錯，重試一次仍然失敗，執行 `awkit stop-workflow error_exit` 結束。
- 結束前一定要執行 `awkit stop-workflow <reason>`：它寫下報告，並讓 `awkit run` 停下來。沒執行就結束，`awkit run` 會再開一個新的 session 重做。
- 不寫程式、不動 Worker 的分支、不自己合併 PR、不自己開票。票的內容有問題就在 issue 留言說明並停下，不替 Worker 補做。
- 迴圈次數與連續失敗的上限由 `awkit analyze-next` 計數，到了它會回報 `none`；不必自己數。

## References

需要時才讀，讀過而且內容還在對話裡就不必重讀。

- `references/conflict-resolution.md`：`needs_conflict_resolution` 時交給 `conflict-resolver` subagent，以及之後要改的標籤。
- `references/ticket.md`：票券的格式與 `awkit create-task`。有人要求補一張票時才讀。
- `references/contracts.md`：各指令印出的欄位、狀態值與停止原因。輸出看不懂時查。
- `references/after-compaction.md`：對話被壓縮之後怎麼接回迴圈。
