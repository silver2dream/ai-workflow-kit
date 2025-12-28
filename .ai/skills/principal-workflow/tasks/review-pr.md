# Review PR

審查 PR 並決定 approve 或 request-changes。

## 輸入

- `PR_NUMBER`: PR 編號（必填）
- `ISSUE_NUMBER`: 關聯 Issue（由 analyze_next.sh 提供）

## 步驟

### 1. 獲取審查資訊

執行一次腳本獲取所有需要的資訊：

```bash
bash .ai/scripts/prepare_review.sh "$PR_NUMBER" "$ISSUE_NUMBER"
```

輸出包含：
- `PRINCIPAL_SESSION_ID`: 審查者 session
- `CI_STATUS`: CI 狀態（passed/failed）
- `DIFF_HASH`: diff 的 hash
- `DIFF_BYTES`: diff 大小（bytes）
- `REVIEW_DIR`: 審查產物目錄（包含 diff/review/evidence log）
- `WORKTREE_PATH`: worktree 路徑
- Ticket 需求（Issue 內容）
- PR diff
- PR commits

### 2. 切換到 Worktree 審查代碼

```bash
cd .worktrees/issue-$ISSUE_NUMBER
```

在 worktree 中，根據 ticket 需求審查實際代碼。你可以直接讀取任何檔案。

### 3. 執行審查

**對照 ticket 需求和實際代碼，執行以下 6 項審查：**

1. **需求符合度**：PR 是否完成了 ticket 所要求的功能？
2. **Commit 格式**：是否符合 `[type] subject`（小寫）？
3. **範圍限制**：有無超出 ticket scope 的修改？
4. **架構合規**：是否符合專案規範？
5. **代碼質量**：有無調試代碼、明顯 bug？
6. **安全檢查**：有無敏感資訊洩露？

**評分標準：**
- 9-10：完美完成需求，代碼質量優秀
- 7-8：完成需求，代碼質量良好
- 5-6：部分完成需求，或有明顯問題
- 3-4：大部分需求未完成
- 1-2：完全不符合需求，或有安全問題

**硬性規則（可證明審查）**

你的 `$REVIEW_BODY` 必須包含至少 3 行 `EVIDENCE:`（一行一條），每一條都必須是 PR diff 中「可直接搜尋到」的原始字串。

格式（擇一）：
- `EVIDENCE: <file> | <needle>`（推薦，會限定在該檔案的 diff 區段內驗證）
- `EVIDENCE: <needle>`

若缺少或無法驗證，`submit_review.sh` 會中止審查、移除 `pr-ready`、加上 `needs-human-review`，避免主迴圈無限重試。

### 4. 提交審查結果

準備審查內容（markdown 格式），然後執行：

```bash
bash .ai/scripts/submit_review.sh "$PR_NUMBER" "$ISSUE_NUMBER" "$SCORE" "$CI_STATUS" "$REVIEW_BODY"
```

其中 `$REVIEW_BODY` 是你的審查內容，格式：

```markdown
### Code Symbols (New/Modified)
- `func NewHandler()`
- `type Config struct`

### Evidence
EVIDENCE: backend/internal/rooms/room_manager.go | type RoomManager struct
EVIDENCE: backend/internal/rooms/room_manager.go | func (m *RoomManager) JoinRoom(
EVIDENCE: backend/internal/rooms/room_manager_test.go | room full

### Score Reason
完成 ticket 需求，代碼質量良好

### Suggested Improvements
- 可以加入更多錯誤處理

### Potential Risks
- 無明顯風險
```

腳本會自動：
- 發布 AWK Review Comment
- 發布 GitHub Review（approve 或 request-changes）
- 如果通過且 CI passed：合併 PR、關閉 Issue、更新 tasks.md、清理 worktree
- 如果不通過：加回 ai-task 標籤讓 Worker 重做

## 輸出

- `RESULT=merged`: PR 已合併
- `RESULT=approved_ci_failed`: 審查通過但 CI 失敗
- `RESULT=changes_requested`: 審查不通過
- `RESULT=review_blocked`: 審查被阻擋（evidence 無法驗證或缺少依賴）
- `RESULT=merge_failed`: 合併失敗

回到 main-loop。
