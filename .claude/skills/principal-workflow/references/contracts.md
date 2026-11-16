# Command Output

主迴圈用到的 `awkit` 指令各印出什麼。定義處是程式本身（`awkit <指令> --help`）；這裡是查閱用的摘要，兩者不一致時以程式為準並回修這份文件。

## awkit analyze-next --json

```json
{"next_action": "dispatch_worker", "issue_number": 27, "pr_number": 30, "exit_reason": "", "merge_issue": "conflict"}
```

| 欄位 | 說明 |
|---|---|
| `next_action` | `dispatch_worker`、`check_result`、`review_pr`、`all_complete`、`none` |
| `issue_number` | Issue 編號；沒有時是 `0` |
| `pr_number` | PR 編號；沒有時是 `0` |
| `merge_issue` | `conflict` 或 `rebase`：這次派工是要 Worker 處理合併問題。其他時候是空字串 |
| `exit_reason` | `next_action` 是 `none` 時的停止原因；`issue_number` 這時是讓流程停下的那張票 |

| `next_action` | 一定有 | 可能有 |
|---|---|---|
| `dispatch_worker` | `issue_number` | `pr_number`、`merge_issue` |
| `check_result` | `issue_number` | |
| `review_pr` | `pr_number` | `issue_number` |
| `all_complete` | | |
| `none` | `exit_reason` | `issue_number`、`pr_number` |

### exit_reason

每一個值都可以原樣交給 `awkit stop-workflow`。

| 值 | 意思 |
|---|---|
| `user_stopped` | 有人要求停止（`awkit stop`）；派出去的工作不再增加 |
| `needs_human_review` | 有 issue 帶 `needs-human-review` 標籤，或衝突處理超過次數上限 |
| `worker_failed` | 有 issue 的 Worker 失敗到重試上限（`worker-failed` 標籤） |
| `review_max_retries` | 同一個 PR 的審查被擋下太多次 |
| `invalid_ticket` | 票的 `awk-ticket` 區塊讀不懂；原因已留言在那個 issue |
| `blocked_by_dependencies` | 剩下的票都在等還沒完成的 issue |
| `no_actionable_tasks` | 還有開著的票，但沒有一張現在可以做 |
| `max_loop_reached`、`max_consecutive_failures` | 迴圈次數或連續失敗次數到上限 |
| `config_not_found`、`config_invalid` | `.ai/config.yaml` 不存在或不能用；原因印在 stderr，`awkit validate` 會再說一次 |
| `github_api_error`、`loop_count_error` | 讀不到 GitHub 或本機的狀態檔 |

## awkit dispatch-worker

印出 `WORKER_STATUS=<值>`，有時另有 `PR_URL=`、`WORKER_ERROR=`。標籤、重試計數與留言已由指令處理。

| `WORKER_STATUS` | 意思 |
|---|---|
| `success` | Worker 完成，PR 已建立，issue 已標成 `pr-ready` |
| `success_no_changes` | Worker 判斷不需要改程式；issue 已留言並關閉 |
| `failed` | Worker 失敗。未到重試上限時下一輪會再派；到上限時 issue 已標成 `worker-failed` |
| `in_progress` | 這張票已經有 Worker 在做，這次沒有再派 |
| `needs_conflict_resolution` | rebase 停在衝突上。另印出 `WORKTREE_PATH=`、`ISSUE_NUMBER=`、`PR_NUMBER=`；issue 仍帶 `in-progress`，照 `conflict-resolution.md` 處理 |

## awkit check-result --issue N

印出 `CHECK_RESULT_STATUS=<值>`（`WORKER_STATUS=` 是同一個值）與 `PR_NUMBER=`。標籤已由指令處理，不論哪個值都回到主迴圈。

| `CHECK_RESULT_STATUS` | 意思 |
|---|---|
| `success` | Worker 完成，issue 已標成 `pr-ready` |
| `success_no_changes` | 不需要改程式；issue 已關閉 |
| `not_found` | 結果還沒出來；指令已等了一段時間才返回 |
| `failed_will_retry` | 失敗，未到上限；已移除 `in-progress`，下一輪會再派 |
| `failed_max_retries` | 失敗到上限；已標成 `worker-failed` |
| `crashed`、`timeout` | Worker 的行程不見了或跑太久；已移除 `in-progress` |
| `failed`、`error` | 結果檔說成功卻沒有 PR，或結果檔讀不了 |

## awkit stop-workflow \<reason\>

寫下報告、建立 `.ai/state/STOP`（`awkit run` 看到它就不再開新的 session，並依原因告訴使用者接下來做什麼；下一次 `awkit run` 會自己清掉它）、清掉迴圈計數。`reason` 除了上面的 `exit_reason`，還接受 `all_tasks_complete`、`error_exit`、`interrupted`、`max_failures`、`escalation_triggered`、`contract_violation`、`none`。

## pr-reviewer subagent

回報 `awkit submit-review` 的結果，四者之一：

| 結果 | 意思 |
|---|---|
| `merged` | 審查通過，PR 已合併，issue 已關閉 |
| `changes_requested` | 審查不通過、CI 沒過，或有維護者的意見還沒處理；issue 已退回給 Worker，或排進下一次審查 |
| `review_blocked` | 審查附的測試證據對不上實際輸出；下一輪會換一個新的審查 |
| `merge_failed` | 審查通過但合併不了（衝突、落後 base 等）；issue 已標上對應的標籤 |
