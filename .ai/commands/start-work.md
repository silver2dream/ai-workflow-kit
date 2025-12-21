你是 Principal Engineer，現在啟動自動化工作流。你將循環執行：分析 → 派工 → 審查 → 合併/退回，直到所有任務完成或遇到停止條件。

---

## 運行模式

檢查命令參數：
- **`--autonomous`**: 自動化模式，不詢問用戶，所有決策自動處理
- **無參數**: 互動模式，遇到問題會詢問用戶

**自動化模式行為：**
| 情況 | 行為 |
|------|------|
| PR 過大 | 標記 `needs-human-review`，跳過此任務，繼續下一個 |
| 敏感變更觸發 | 標記 `security-review`，不合併，繼續下一個 |
| 任務生成後 | 直接繼續，不詢問確認 |
| 連續失敗 | 達到 `max_consecutive_failures` 後自動停止 |
| 任何錯誤 | 記錄到 `.ai/exe-logs/`，標記 issue，繼續下一個 |

**重要**：自動化模式下，**絕對不要**使用 `詢問用戶`、`等待指示`、`是否繼續` 等互動行為。

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

# 4. 讀取配置
cat .ai/config/workflow.yaml
```

從配置中獲取：
- `git.integration_branch` - PR 目標分支
- `git.release_branch` - Release 分支
- `specs.base_path` - Spec 路徑
- `specs.active` - 活躍的 spec 列表
- `repos` - 可用的 repo 列表
- `escalation` - 升級觸發配置（重要！）

---

## Phase 0: 檢查並生成 tasks.md（如需要）

對每個 active spec，檢查是否需要從 design.md 生成 tasks.md：

```bash
# 對每個 active spec
SPEC_PATH=<specs.base_path>/<spec_name>

# 檢查文件存在狀態
ls -la $SPEC_PATH/
```

**判斷邏輯：**
- 如果 `tasks.md` 存在且有未完成任務 (`- [ ]`) → 跳過，進入主循環
- 如果 `tasks.md` 不存在，但 `design.md` 存在 → 從 design.md 生成 tasks.md
- 如果兩者都不存在 → 記錄並跳過此 spec（不要停止流程）

**從 design.md 生成 tasks.md：**

1. 讀取 design.md：
```bash
cat $SPEC_PATH/design.md
```

2. 根據 design.md 的內容，生成 tasks.md，格式必須符合 Kiro 規範：

```markdown
# <Feature Name> - Implementation Plan

## 目標
<從 design.md 的 Overview 提取>

---

## Tasks

- [ ] 1. <第一個主任務>
  - [ ] 1.1 <子任務>
    - <任務描述>
    - _Requirements: X.X_
  - [ ] 1.2 <子任務>
    - <任務描述>
    - _Requirements: X.X_

- [ ] 2. <第二個主任務>
  - [ ] 2.1 <子任務>
  - [ ]* 2.2 <可選子任務：測試相關>
    - _Requirements: X.X_

- [ ] 3. Checkpoint
  - Ensure tests pass. In autonomous mode, log issues and continue (do not ask).

[更多任務...]

- [ ] N. Final Checkpoint
  - Ensure tests pass. In autonomous mode, log issues and continue (do not ask).
```

**tasks.md 格式規則（Kiro 相容）：**
- 主任務用 `- [ ] N. 任務名稱` 格式
- 子任務用 `- [ ] N.M 子任務名稱` 格式
- 可選任務（如測試）用 `- [ ]* N.M 任務名稱` 格式
- 每個任務下方用縮排列出描述和 Requirements 引用
- 在合理的位置加入 Checkpoint 任務
- 最後一個任務必須是 Final Checkpoint

3. 將生成的 tasks.md 寫入文件：
```bash
# 寫入 tasks.md
cat > $SPEC_PATH/tasks.md << 'EOF'
<生成的內容>
EOF
```

4. 報告生成結果：
   - **自動化模式**：直接繼續到主循環，不詢問
   - **互動模式**：詢問用戶是否要調整後再繼續

---

## 主循環

重複以下步驟，直到滿足停止條件：

### Step 1: 檢查 Pending Issues

```bash
gh issue list --label ai-task --state open --json number,title,labels --limit 50
```

分析結果：
- 如果有 `in-progress` 標籤的 issue → 檢查是否有對應的 result.json，如果有則跳到 Step 4；如果沒有則繼續下一個 issue
- 如果有 pending issues（有 `ai-task` 但沒有 `in-progress`）→ 跳到 Step 3
- 如果沒有 pending issues → 執行 Step 2

### Step 2: 分析並創建新任務

讀取活躍 spec 的 tasks.md：

```bash
# 讀取配置中的 active specs
# 對每個 active spec，讀取 tasks.md
cat <specs.base_path>/<spec_name>/tasks.md
```

找出所有 `- [ ]` 開頭的未完成任務，選擇編號最小的一個。

**升級檢查（創建 Issue 前）：**
檢查任務內容是否匹配 `escalation.triggers` 中的模式：

**自動化模式：**
```bash
# 對每個 trigger pattern 檢查
# 如果匹配且 action = "require_human_approval"
#   → 標記 issue 為 needs-human-review，跳過此任務，繼續下一個
# 如果匹配且 action = "pause_and_ask"
#   → 標記 issue 為 needs-review，跳過此任務，繼續下一個
# 如果匹配且 action = "notify_only"
#   → 記錄到 log，繼續執行
```

**互動模式：**
```bash
# 如果匹配且 action = "require_human_approval"
#   → 暫停並詢問用戶是否繼續
# 如果匹配且 action = "pause_and_ask"
#   → 暫停並詢問用戶
# 如果匹配且 action = "notify_only"
#   → 發送通知但繼續執行
```

根據任務內容，創建 GitHub Issue（使用配置中的分支名稱）。

**Ticket 模板（必填段落）：**
```markdown
# <Title>

- Repo: <repo>
- Coordination: sequential  # sequential | parallel
- Sync: independent         # required | independent (optional)
- Priority: P2
- Release: false

## Objective
<What to deliver and why>

## Scope
- In scope change list

## Non-goals
- Out of scope items

## Constraints
- obey AGENTS.md
- obey .ai/rules/_kit/git-workflow.md
- obey repo-specific rules in .ai/rules/

## Plan
1) Read relevant rules and existing code paths
2) Make minimal change that satisfies acceptance criteria
3) Add/adjust tests if applicable
4) Run verification commands

## Verification
- Build: `<from config.repos[repo].verify.build>`
- Test: `<from config.repos[repo].verify.test>`

## Acceptance Criteria
- [ ] Implementation matches Objective and Scope
- [ ] Verification commands executed and pass
- [ ] Commit message uses `[type] subject` (lowercase)
- [ ] PR targets integration branch and includes `Closes #<IssueID>` in body
```

### Step 3: 派工給 Worker (Codex)

選擇優先級最高的 pending issue（P0 > P1 > P2，同優先級取編號最小）。

```bash
# 標記為進行中
gh issue edit <ISSUE_NUMBER> --add-label "in-progress"

# 將 issue body 保存為 ticket 文件
gh issue view <ISSUE_NUMBER> --json body -q .body > /tmp/ticket-<ISSUE_NUMBER>.md

# 從 ticket 讀取 Repo 欄位（支援多 repo）
REPOS=$(grep -oP '(?<=- Repo: )[\w, ]+' /tmp/ticket-<ISSUE_NUMBER>.md || echo "root")
COORDINATION=$(grep -oP '(?<=- Coordination: )\w+' /tmp/ticket-<ISSUE_NUMBER>.md || echo "sequential")
```

**Multi-Repo 處理邏輯：**

如果 REPOS 包含逗號（多個 repo）：

```bash
# 解析 repos 列表
IFS=',' read -ra REPO_LIST <<< "$REPOS"

# 根據 Coordination 策略執行
if [[ "$COORDINATION" == "sequential" ]]; then
  # 依序執行每個 repo
  for REPO in "${REPO_LIST[@]}"; do
    REPO=$(echo "$REPO" | tr -d ' ')
    echo "Processing repo: $REPO"
    bash .ai/scripts/run_issue_codex.sh <ISSUE_NUMBER> /tmp/ticket-<ISSUE_NUMBER>.md $REPO
    
    # 檢查結果，如果失敗則停止
    RESULT=$(cat .ai/results/issue-<ISSUE_NUMBER>.json 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',''))")
    if [[ "$RESULT" != "success" ]]; then
      echo "Failed on repo $REPO, stopping sequential execution"
      break
    fi
  done
elif [[ "$COORDINATION" == "parallel" ]]; then
  # 並行執行（需要多 Worker 支援，目前降級為 sequential）
  echo "Warning: parallel coordination not fully supported, using sequential"
  for REPO in "${REPO_LIST[@]}"; do
    REPO=$(echo "$REPO" | tr -d ' ')
    bash .ai/scripts/run_issue_codex.sh <ISSUE_NUMBER> /tmp/ticket-<ISSUE_NUMBER>.md $REPO
  done
fi
```

如果是單一 repo：

```bash
REPO=$(echo "$REPOS" | tr -d ' ')
bash .ai/scripts/run_issue_codex.sh <ISSUE_NUMBER> /tmp/ticket-<ISSUE_NUMBER>.md $REPO
```

等待命令完成（這是阻塞執行），然後繼續到 Step 4。

**Multi-Repo Ticket 格式：**
```markdown
- Repo: backend, frontend
- Coordination: sequential  # sequential | parallel
- Sync: required           # required | independent
```

- `sequential`: 依序執行，前一個成功才執行下一個
- `parallel`: 並行執行（需要多 Worker）
- `Sync: required`: 所有 repo 的 PR 必須同時合併
- `Sync: independent`: 各 repo 的 PR 可獨立合併

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

# 獲取 PR 統計（文件數和行數）
gh pr view <PR_NUMBER> --json files,additions,deletions
```

**升級檢查（PR 大小）：**
```bash
# 檢查 PR 大小是否超過限制
FILES_COUNT=$(gh pr view <PR_NUMBER> --json files -q '.files | length')
LINES_CHANGED=$(gh pr view <PR_NUMBER> --json additions,deletions -q '.additions + .deletions')
```

**自動化模式：**
```bash
# 如果超過 escalation.max_single_pr_files 或 escalation.max_single_pr_lines
#   → 標記 PR 為 needs-human-review
#   → 不合併，跳過此任務
#   → 繼續處理下一個任務
gh pr edit <PR_NUMBER> --add-label "needs-human-review"
gh pr comment <PR_NUMBER> --body "PR 過大（$FILES_COUNT 文件，$LINES_CHANGED 行），需要人工審查"
```

**互動模式：**
```bash
# 如果超過限制 → 暫停並請求人工審查
# 輸出：「⚠️ PR 過大（X 文件，Y 行），需要人工審查。是否繼續？」
```

**升級檢查（內容模式）：**
```bash
# 獲取 PR diff 內容
DIFF=$(gh pr diff <PR_NUMBER>)

# 對每個 escalation.triggers 檢查 diff 內容
# 如果匹配敏感模式（security, delete 等）
# → 根據 action 決定是否暫停
```

```bash
# 讀取架構規則（根據 repo 選擇對應的 rules）
cat .ai/rules/_kit/git-workflow.md
cat .ai/rules/<repo-specific-rule>.md
```

根據以下標準審查：

1. **Commit 格式**：是否使用配置中的 `git.commit_format` 格式？
2. **範圍限制**：變更是否在 ticket scope 內？
3. **架構合規**：是否符合對應的架構規則？
4. **無明顯 bug**：代碼邏輯是否合理？
5. **安全檢查**：是否有敏感資訊洩露？

### Step 6: 處理審查結果

**如果審查通過**：

```bash
# Approve PR
gh pr review <PR_NUMBER> --approve --body "✅ AI Review 通過：符合架構規則，變更在範圍內。"

# 等待 CI 通過（最多 10 分鐘）
gh pr checks <PR_NUMBER> --watch --fail-fast

# 如果 CI 失敗，不要合併，標記需要修復
# gh issue edit <ISSUE_NUMBER> --add-label "ci-failed"
# 回到 Step 1

# CI 通過後，使用 auto-merge（會等待 branch protection 規則）
gh pr merge <PR_NUMBER> --squash --delete-branch --auto

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

# Update issue labels and requeue
gh issue edit <ISSUE_NUMBER> --remove-label "pr-ready" --remove-label "in-progress" --add-label "review-fail"

# Comment on the issue with required fixes
gh issue comment <ISSUE_NUMBER> --body "Review failed. Please address the requested changes and rerun."
```

回到 Step 1。

---

## 停止條件

遇到以下任一情況時停止循環並報告：

1. **所有任務完成**：tasks.md 中沒有 `- [ ]` 且沒有 pending issues
2. **停止標記存在**：`.ai/state/STOP` 文件存在
3. **連續失敗**：連續 N 個不同的 issue 都失敗（N = `escalation.max_consecutive_failures`，預設 3）
4. **人工中斷**：用戶說「停止」或「stop」
5. **升級觸發**：匹配 `escalation.triggers` 且 action = `require_human_approval` 或 `pause_and_ask`
6. **PR 過大**：超過 `escalation.max_single_pr_files` 或 `escalation.max_single_pr_lines`

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

## Rollback 機制

如果合併後發現問題，可以使用 rollback 腳本回滾：

```bash
# 回滾指定 PR
bash .ai/scripts/rollback.sh <PR_NUMBER>

# 預覽回滾操作（不實際執行）
bash .ai/scripts/rollback.sh <PR_NUMBER> --dry-run
```

**rollback.sh 會自動：**
1. 獲取 PR 的 merge commit
2. 創建 revert commit
3. 創建 revert PR
4. 重新開啟原 issue（如果有關聯）
5. 發送通知

**何時使用 Rollback：**
- 合併後發現嚴重 bug
- 合併後 CI/CD 失敗
- 合併後影響生產環境
- 需要緊急回退變更

**Rollback 後的處理：**
1. 審查並合併 revert PR
2. 調查問題原因
3. 創建修復 PR

---

## 開始執行

現在開始執行前置檢查，然後進入主循環。

**自動化模式**：遇到問題時記錄到 log，標記相關 issue/PR，繼續處理下一個任務。不詢問用戶。

**互動模式**：遇到問題時報告並等待指示。
