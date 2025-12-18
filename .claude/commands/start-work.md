你是 Principal Engineer，現在啟動自動化工作流。你將循環執行：分析 → 派工 → 審查 → 合併/退回，直到所有任務完成或遇到停止條件。

---

## 前置檢查

先執行這些檢查，任何一項失敗就停止並報告：

```bash
# 1. 確認 gh 已認證
gh auth status

# 2. 確認工作目錄乾淨
git status --porcelain

# 3. 確認沒有停止標記
test ! -f .ai/state/STOP
```

---

## 主循環

重複以下步驟，直到滿足停止條件：

### Step 1: 檢查 Pending Issues

```bash
gh issue list --label ai-task --state open --json number,title,labels --limit 50
```

分析結果：
- 如果有 `in-progress` 標籤的 issue → 檢查是否有對應的 result.json，如果有則跳到 Step 4
- 如果有 pending issues（有 `ai-task` 但沒有 `in-progress`）→ 跳到 Step 3
- 如果沒有 pending issues → 執行 Step 2

### Step 2: 分析並創建新任務

讀取以下文件理解專案狀態：

```bash
# 讀取未完成任務
cat .kiro/specs/cultivation-mvp/tasks.md

# 讀取設計規格（如果需要上下文）
cat .kiro/specs/cultivation-mvp/design.md
```

找出所有 `- [ ]` 開頭的未完成任務，選擇編號最小的一個。

根據任務內容，創建 GitHub Issue：

```bash
gh issue create \
  --title "[feat] task N: 任務標題小寫" \
  --body "$(cat <<'EOF'
# [feat] task N: 任務標題

- Repo: backend | frontend | root（根據任務內容判斷）
- Severity: P2
- Source: tasks.md #N
- Release: false

## Objective
實作任務 N：任務描述。

## Scope
- 根據任務內容列出具體要改的範圍
- 保持變更最小化

## Non-goals
- 不做任務範圍外的重構
- 不改變不相關的代碼

## Constraints
- obey AGENTS.md
- obey `.claude/rules/git-workflow.md`
- backend: obey `.claude/rules/backend-nakama-architecture-and-patterns.md`
- frontend: obey `.claude/rules/unity-architecture-and-patterns.md`

## Plan
1. 閱讀相關規則和現有代碼
2. 實作符合驗收標準的最小變更
3. 添加/調整測試（如果適用）
4. 執行驗證命令

## Verification
- backend: `go build ./...` 和 `go test ./...`
- frontend: Unity 編譯無錯誤
- root: `git status --porcelain` 乾淨

## Acceptance Criteria
- [ ] 實作符合目標和範圍
- [ ] 驗證命令執行通過
- [ ] Commit message 使用 `[type] subject` 格式
- [ ] PR 創建到 `feat/aether`，body 包含 `Closes #N`
EOF
)" \
  --label "ai-task"
```

記錄創建的 Issue 編號，繼續到 Step 3。

### Step 3: 派工給 Worker (Codex)

選擇優先級最高的 pending issue（P0 > P1 > P2，同優先級取編號最小）。

```bash
# 標記為進行中
gh issue edit <ISSUE_NUMBER> --add-label "in-progress"

# 將 issue body 保存為 ticket 文件
gh issue view <ISSUE_NUMBER> --json body -q .body > /tmp/ticket-<ISSUE_NUMBER>.md

# 從 ticket 讀取 Repo 欄位
REPO=$(grep -oP '(?<=- Repo: )\w+' /tmp/ticket-<ISSUE_NUMBER>.md || echo "root")

# 執行 Worker
bash scripts/ai/run_issue_codex.sh <ISSUE_NUMBER> /tmp/ticket-<ISSUE_NUMBER>.md $REPO
```

等待命令完成（這是阻塞執行），然後繼續到 Step 4。

### Step 4: 檢查執行結果

```bash
cat .ai/results/issue-<ISSUE_NUMBER>.json
```

分析 `status` 欄位：

**如果 status = "success" 且有 pr_url**：
- 更新 issue 標籤：`gh issue edit <ISSUE_NUMBER> --remove-label "in-progress" --add-label "pr-ready"`
- 繼續到 Step 5

**如果 status = "failed"**：
- 讀取失敗次數：`cat .ai/runs/issue-<ISSUE_NUMBER>/fail_count.txt`
- 如果 < 3 次：移除 in-progress 標籤，下一輪重試
- 如果 >= 3 次：標記 `gh issue edit <ISSUE_NUMBER> --remove-label "in-progress" --add-label "worker-failed"`，跳過此 issue
- 回到 Step 1

### Step 5: 審查 PR

從 result.json 獲取 PR URL，提取 PR 編號。

```bash
# 獲取 PR diff
gh pr diff <PR_NUMBER>

# 讀取架構規則
cat .claude/rules/git-workflow.md
cat .claude/rules/backend-nakama-architecture-and-patterns.md  # 如果是 backend
cat .claude/rules/unity-architecture-and-patterns.md  # 如果是 frontend
```

根據以下標準審查：

1. **Commit 格式**：是否使用 `[type] subject` 格式？
2. **範圍限制**：變更是否在 ticket scope 內？
3. **架構合規**：是否符合對應的架構規則？
4. **無明顯 bug**：代碼邏輯是否合理？

### Step 6: 處理審查結果

**如果審查通過**：

```bash
# Approve PR
gh pr review <PR_NUMBER> --approve --body "✅ AI Review 通過：符合架構規則，變更在範圍內。"

# Merge PR
gh pr merge <PR_NUMBER> --squash --delete-branch

# 關閉 Issue
gh issue close <ISSUE_NUMBER> --comment "🎉 已合併！PR #<PR_NUMBER>"

# 更新標籤
gh issue edit <ISSUE_NUMBER> --add-label "review-pass"
```

回到 Step 1 處理下一個任務。

**如果審查不通過**：

```bash
# Request changes
gh pr review <PR_NUMBER> --request-changes --body "❌ 需要修正：
<列出具體問題>
"

# 更新原 issue 標籤
gh issue edit <ISSUE_NUMBER> --remove-label "pr-ready" --add-label "review-fail"

# 創建 fix issue
gh issue create \
  --title "[fix] review feedback for #<ISSUE_NUMBER>" \
  --body "## 審查反饋

<列出具體需要修正的問題>

## 原 Issue
#<ISSUE_NUMBER>

## 原 PR
#<PR_NUMBER>

---
- Repo: <same as original>
- Severity: P1
- Source: review:#<ISSUE_NUMBER>
- Release: false

## Objective
修正 PR #<PR_NUMBER> 的審查問題。

## Scope
- 只修正審查指出的問題
- 不做額外變更

## Verification
- 同原 ticket
" \
  --label "ai-task,fix,priority-P1"
```

回到 Step 1。

---

## 停止條件

遇到以下任一情況時停止循環並報告：

1. **所有任務完成**：tasks.md 中沒有 `- [ ]` 且沒有 pending issues
2. **停止標記存在**：`.ai/state/STOP` 文件存在
3. **連續失敗**：連續 3 個不同的 issue 都失敗
4. **人工中斷**：用戶說「停止」或「stop」

---

## 輸出報告

每完成一輪循環，簡要報告：
- 處理了哪個 issue
- 結果（merged / rejected / failed）
- 下一步計劃

結束時輸出總結：
- 總共處理了多少 issues
- 成功合併了多少 PRs
- 有多少需要人工處理
- 建議的後續行動

---

## 開始執行

現在開始執行前置檢查，然後進入主循環。遇到任何問題時報告並等待指示。
