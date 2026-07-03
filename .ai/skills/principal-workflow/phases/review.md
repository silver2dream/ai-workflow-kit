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

當 `workflow.yaml` 中設定 `review.multi_model: true` 時，**`awkit submit-review` 會自動執行**多模型共識審查，Principal 與 pr-reviewer 不需要做任何額外操作。

系統行為（由 Go 程式碼強制執行，非 agent 計算）：

1. `submit-review` 取得 PR diff，平行調用 `review.secondary_reviewers` 設定的次要審查者（未設定時預設為一個 architecture-focused 的 opus 審查者，透過 `claude --print` 執行）。
2. 共識分數 = `primary × 0.7 + secondaries 均分 × 0.3`（四捨五入）；任一審查者回報 `[ERROR]` 發現時，共識分數上限為 6。
3. 共識報告（每位審查者的分數與 findings）自動附加到 review body，approve 與 changes_requested 的留言都會帶上。
4. 次要審查者失敗或超時不會阻塞審查：失敗會記載於報告中並排除於計分外（graceful degradation）。

**Principal 不要自行調用 architecture-reviewer、不要自行計算加權分數** —— 這些已由 `awkit submit-review` 處理。
