# 輸出契約

## awkit analyze-next 輸出規格

### JSON 格式（使用 `--json`）

```json
{
  "next_action": "<action>",
  "issue_number": <number>,
  "pr_number": <number>,
  "spec_name": "<name>",
  "task_line": <number>,
  "epic_issue": <number>,
  "task_text": "<text>",
  "exit_reason": "<reason>"
}
```

### 欄位說明

| 欄位 | 類型 | 說明 |
|------|------|------|
| `next_action` | string | generate_tasks, create_task, dispatch_worker, check_result, review_pr, all_complete, none |
| `issue_number` | int | Issue 編號（0 表示無） |
| `pr_number` | int | PR 編號（0 表示無） |
| `spec_name` | string | Spec 名稱（空字串表示無） |
| `task_line` | int | tasks.md 行號（tasks_md mode，0 表示無） |
| `epic_issue` | int | Tracking Issue 編號（epic mode，0 表示無） |
| `task_text` | string | Epic body 中的任務文字（epic mode，空字串表示無） |
| `exit_reason` | string | 停止原因（僅當 next_action=none 時有值） |

## 必填欄位表

| next_action | 必填 | 可選 |
|-------------|------|------|
| `generate_tasks` | - | `spec_name` |
| `create_task` (epic mode) | `spec_name`, `epic_issue`, `task_text` | - |
| `create_task` (tasks_md mode) | `spec_name`, `task_line` | - |
| `dispatch_worker` | `issue_number` | - |
| `check_result` | `issue_number` | - |
| `review_pr` | `pr_number` | `issue_number` |
| `all_complete` | - | - |
| `none` | - | `exit_reason` |

## 命令列表

| 命令 | 用途 | stdout |
|------|------|--------|
| `awkit analyze-next` | 決定下一步 | 變數賦值 |
| `awkit dispatch-worker` | 派工 | 變數賦值 |
| `awkit check-result` | 檢查結果 | 變數賦值 |
| `awkit stop-workflow` | 停止流程 | - |
| `awkit prepare-review` | 準備審查 | 變數賦值 |
| `awkit submit-review` | 提交審查 | 變數賦值 |
| `awkit create-task` | 建立 Issue | - |
