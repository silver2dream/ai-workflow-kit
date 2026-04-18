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

## 可選：Multi-Model 交叉審查

當 `workflow.yaml` 中設定 `review.multi_model: true` 時，在 pr-reviewer 完成後，額外調用 architecture-reviewer 進行架構層面審查。

**執行條件**：讀取 `.ai/config/workflow.yaml`，檢查 `review.multi_model` 是否為 `true`。如果未設定或為 `false`，跳過此步驟。

**執行步驟**：

1. 在 pr-reviewer 返回結果後（且結果不是 `review_blocked`），調用 architecture-reviewer：
   - `subagent_type`: `"architecture-reviewer"`
   - `description`: `"Architecture review PR #<pr_number>"`
   - `prompt`: `"Architecture review PR #<pr_number> for Issue #<issue_number>"`

2. 合併分數：
   - 最終分數 = `pr-reviewer score × 0.7 + architecture-reviewer score × 0.3`（四捨五入取整）
   - 如果 architecture-reviewer 發現 severity=error 的問題，最終分數上限為 6

3. 合併 review body：將 architecture-reviewer 的 findings 附加到 pr-reviewer 的 review body 末尾

4. 如果合併後分數低於 `score_threshold`，使用合併後的 body 執行 `awkit submit-review` 提交 `changes_requested`

**注意**：architecture-reviewer 失敗或超時時，忽略其結果，僅使用 pr-reviewer 的結果（graceful degradation）。
