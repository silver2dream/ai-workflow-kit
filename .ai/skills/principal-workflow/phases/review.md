# Review PR

## CRITICAL: review_pr 必須使用 Task Tool

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

**收到結果後，直接回到 main-loop Step 1**，不要嘗試修正或重試。
