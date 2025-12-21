派工給 Worker (Codex) 執行指定的 Issue。

用法：`/dispatch-worker <ISSUE_NUMBER>` 或 `/dispatch-worker`（會詢問 Issue 編號）

---

## Step 1: 獲取 Issue 信息

```bash
gh issue view <ISSUE_NUMBER> --json number,title,body,labels,state
```

確認：
- Issue 存在且是 open 狀態
- Issue 有 `ai-task` 標籤
- Issue 沒有 `in-progress` 標籤（避免重複執行）

## Step 2: 準備 Ticket

```bash
# 確保 temp 目錄存在
mkdir -p .ai/temp

# 保存 issue body 為 ticket 文件（使用 .ai/temp/ 而非 /tmp/）
gh issue view <ISSUE_NUMBER> --json body -q .body > .ai/temp/ticket-<ISSUE_NUMBER>.md

# 讀取 Repo 欄位
REPO=$(grep -oP '(?<=- Repo: )\w+' .ai/temp/ticket-<ISSUE_NUMBER>.md || echo "root")
echo "Repo: $REPO"
```

## Step 3: 標記進行中

```bash
gh issue edit <ISSUE_NUMBER> --add-label "in-progress"
gh issue comment <ISSUE_NUMBER> --body "🤖 Worker 開始執行..."
```

## Step 4: 執行 Worker

```bash
bash .ai/scripts/run_issue_codex.sh <ISSUE_NUMBER> .ai/temp/ticket-<ISSUE_NUMBER>.md $REPO
```

等待執行完成（阻塞）。

## Step 5: 檢查結果

```bash
cat .ai/results/issue-<ISSUE_NUMBER>.json
```

### 如果成功：

```bash
gh issue edit <ISSUE_NUMBER> --remove-label "in-progress" --add-label "pr-ready"
gh issue comment <ISSUE_NUMBER> --body "✅ Worker 完成！PR: <PR_URL>"
```

報告：
- PR URL
- 變更摘要
- 建議：執行 `/review-pr <PR_NUMBER>` 進行審查

### 如果失敗：

```bash
gh issue edit <ISSUE_NUMBER> --remove-label "in-progress"
```

讀取失敗詳情：
```bash
cat .ai/runs/issue-<ISSUE_NUMBER>/summary.txt
```

報告：
- 失敗原因
- 建議的修正方式
- 是否要重試
