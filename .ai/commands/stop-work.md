停止自動化工作流並生成總結報告。

---

## Step 1: 創建停止標記

```bash
mkdir -p .ai/state
echo "Stopped by user at $(date -u +%Y-%m-%dT%H:%M:%SZ)" > .ai/state/STOP
```

## Step 2: 生成總結報告

### 2.1 統計 Issues

```bash
# 所有 AI 任務
gh issue list --label ai-task --state all --json number,title,state,labels --limit 100

# 待處理
gh issue list --label ai-task --state open --json number,title

# 失敗的
gh issue list --label worker-failed --state open --json number,title

# 需要審查的
gh issue list --label pr-ready --state open --json number,title
```

### 2.2 統計 PRs

```bash
# 待審查的 PR
gh pr list --json number,title,state

# 最近合併的
gh pr list --state merged --limit 10 --json number,title,mergedAt
```

### 2.3 本地結果

```bash
# 列出所有結果文件
ls -la .ai/results/

# 統計成功/失敗
grep -l '"status": "success"' .ai/results/*.json 2>/dev/null | wc -l
grep -l '"status": "failed"' .ai/results/*.json 2>/dev/null | wc -l
```

## Step 3: 輸出報告

```
═══════════════════════════════════════════
        AI Workflow 執行報告
═══════════════════════════════════════════

📊 統計摘要
───────────────────────────────────────────
  Issues 創建:     X
  Issues 完成:     X
  Issues 待處理:   X
  Issues 失敗:     X

  PRs 創建:        X
  PRs 合併:        X
  PRs 待審查:      X

⚠️ 需要關注
───────────────────────────────────────────
  [列出失敗的 issues]
  [列出待審查的 PRs]

📝 建議後續行動
───────────────────────────────────────────
  1. 查看失敗 issues: gh issue list --label worker-failed
  2. 審查待處理 PR: gh pr list
  3. 繼續工作: 刪除 .ai/state/STOP 後執行 /start-work

═══════════════════════════════════════════
```

## Step 4: 清理（可選）

詢問是否要清理臨時文件：

```bash
# 清理 tmp ticket 文件
rm -f /tmp/ticket-*.md

# 清理舊的 signal 文件（如果有）
rm -f .ai/signals/*
```

---

## Step 5: 發送通知

```bash
# 發送統計摘要通知
bash .ai/scripts/notify.sh --summary
```

---

## 重新啟動

要重新啟動工作流，執行：

```bash
rm .ai/state/STOP
```

然後使用 `/start-work` 或 `awkit kickoff`。
