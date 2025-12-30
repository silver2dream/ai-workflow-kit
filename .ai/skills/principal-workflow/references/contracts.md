# 變數契約

## awkit analyze-next 輸出規格

### stdout（只能是可 eval 的變數賦值）

```bash
NEXT_ACTION=<action>
ISSUE_NUMBER=<number or empty>
PR_NUMBER=<number or empty>
SPEC_NAME=<name or empty>
TASK_LINE=<number or empty>
EXIT_REASON=<reason or empty>
```

### stderr

所有 log 輸出到 stderr。

## 變數契約表

| NEXT_ACTION | 必填 | 可選 |
|-------------|------|------|
| `generate_tasks` | - | `SPEC_NAME` |
| `create_task` | `SPEC_NAME`, `TASK_LINE` | - |
| `dispatch_worker` | `ISSUE_NUMBER` | - |
| `check_result` | `ISSUE_NUMBER` | - |
| `review_pr` | `PR_NUMBER` | `ISSUE_NUMBER` |
| `all_complete` | - | - |
| `none` | - | `EXIT_REASON` |

## 契約違反

如果必填欄位為空，awkit analyze-next 應輸出：

```bash
NEXT_ACTION=none
EXIT_REASON=contract_violation
```

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
