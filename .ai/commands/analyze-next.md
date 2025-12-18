分析專案並決定下一個要執行的任務，但不執行，只創建 GitHub Issue。

---

## Step 1: 檢查現有 Pending Issues

```bash
gh issue list --label ai-task --state open --json number,title,labels
```

如果已經有 pending issues，列出它們並詢問：
- 是否要處理現有的 issue？
- 還是創建新的 issue？

## Step 2: 分析 tasks.md

```bash
cat .kiro/specs/cultivation-mvp/tasks.md
```

找出所有 `- [ ]` 開頭的未完成任務，列出：
- 任務編號
- 任務標題
- 對應的 Repo（backend/frontend/root）

## Step 3: 建議下一個任務

根據以下優先級建議：
1. P0/P1 audit 問題（如果有 `.ai/state/audit.json`）
2. 編號最小的未完成任務
3. 有依賴關係時，先處理前置任務

## Step 4: 創建 Issue

確認後，創建 GitHub Issue：

```bash
gh issue create \
  --title "[type] task N: 標題" \
  --body "<ticket 內容>" \
  --label "ai-task"
```

輸出創建的 Issue URL。

---

注意：這個指令只做分析和創建 issue，不會執行 Worker。如果要執行完整流程，請使用 `/start-work`。
