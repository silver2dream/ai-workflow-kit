你是 Principal Engineer，現在啟動自動化工作流。你將循環執行：分析 → 派工 → 審查 → 合併/退回，直到所有任務完成或遇到停止條件。

**此工作流使用模組化命令架構。所有具體操作都委派給獨立的命令文件。**

---

## 進度輸出規則（重要！）

**每個步驟開始時，必須立即輸出進度訊息**，讓使用者知道目前狀態：

```
[PRINCIPAL] <timestamp> | <phase> | <message>
```

範例：
```
[PRINCIPAL] 10:43:37 | PREFLIGHT | 開始前置檢查...
[PRINCIPAL] 10:43:38 | PREFLIGHT | ✓ gh 已認證
[PRINCIPAL] 10:43:38 | PREFLIGHT | ✓ 工作目錄乾淨
[PRINCIPAL] 10:43:39 | PHASE-0   | 檢查 tasks.md...
[PRINCIPAL] 10:43:40 | PHASE-0   | 找到 10 個未完成任務
[PRINCIPAL] 10:43:41 | STEP-1   | 檢查 pending issues...
[PRINCIPAL] 10:43:42 | STEP-2   | 創建新任務: implement room manager
[PRINCIPAL] 10:43:45 | STEP-3   | 派工給 Worker (issue #1)...
[PRINCIPAL] 10:44:30 | STEP-4   | Worker 完成，檢查結果...
[PRINCIPAL] 10:44:31 | STEP-5   | 審查 PR #2...
[PRINCIPAL] 10:44:35 | STEP-6   | ✓ PR 已合併
[PRINCIPAL] 10:44:36 | LOOP     | 回到 Step 1，處理下一個任務...
```

**規則：**
1. 每個 Phase/Step 開始時立即輸出，不要等到結束
2. 重要操作（創建 issue、派工、審查）要輸出詳細資訊
3. 錯誤時輸出 `✗` 和錯誤原因
4. 成功時輸出 `✓`
5. 長時間操作（如等待 Worker）每 30 秒輸出一次心跳

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

**輸出**: `[PRINCIPAL] <time> | PREFLIGHT | 開始前置檢查...`

執行 preflight.md 進行所有前置檢查：

```bash
# 調用 preflight.md
source .ai/commands/preflight.md

# preflight.md 會 export：
# - PRINCIPAL_SESSION_ID
# - INTEGRATION_BRANCH
# - RELEASE_BRANCH
# - SPEC_BASE_PATH
# - ACTIVE_SPECS
# - MAX_CONSECUTIVE_FAILURES
# - MAX_SINGLE_PR_FILES
# - MAX_SINGLE_PR_LINES
# - MAX_DIFF_SIZE
# - MAX_REVIEW_CYCLES
# - CI_TIMEOUT_SECONDS
# - AUTO_MERGE
# - ESCALATION_TRIGGERS (JSON array)
# - 其他配置變數
```

如果 preflight 失敗，停止執行並報告錯誤。

---

## Phase 0: 檢查並生成 tasks.md（如需要）

**輸出**: `[PRINCIPAL] <time> | PHASE-0 | 檢查 specs 和 tasks...`

調用 generate-tasks.md 檢查並生成 tasks：

```bash
# 調用 generate-tasks.md
source .ai/commands/generate-tasks.md
```

---

## 主循環

**輸出**: `[PRINCIPAL] <time> | LOOP | 開始主循環...`

初始化循環控制變數：

```bash
CONSECUTIVE_FAILURES=0
LOOP_COUNT=0
```

重複以下步驟，直到滿足停止條件：

### Step 1: 檢查 Pending Issues

**輸出**: `[PRINCIPAL] <time> | STEP-1 | 檢查 pending issues...`

```bash
# 獲取所有 ai-task issues
ISSUES_JSON=$(gh issue list --label ai-task --state open --json number,title,labels --limit 50)

# 分析 issues 狀態
IN_PROGRESS_ISSUE=""
PR_READY_ISSUE=""
PENDING_ISSUE=""
PENDING_PRIORITY=999  # 用於優先級排序

# 檢查每個 issue 的標籤
while read -r issue; do
  ISSUE_NUMBER=$(echo "$issue" | jq -r '.number')
  LABELS=$(echo "$issue" | jq -r '.labels[].name' | tr '\n' ' ')
  
  # 提取優先級 (P0=0, P1=1, P2=2)
  ISSUE_PRIORITY=2
  if echo "$LABELS" | grep -q "P0"; then
    ISSUE_PRIORITY=0
  elif echo "$LABELS" | grep -q "P1"; then
    ISSUE_PRIORITY=1
  fi
  
  if echo "$LABELS" | grep -q "in-progress"; then
    # 檢查是否有對應的 result.json
    if [[ -f ".ai/results/issue-$ISSUE_NUMBER.json" ]]; then
      IN_PROGRESS_ISSUE="$ISSUE_NUMBER"
      break
    fi
  elif echo "$LABELS" | grep -q "pr-ready"; then
    if [[ -z "$PR_READY_ISSUE" ]] || [[ "$ISSUE_PRIORITY" -lt "$PR_READY_PRIORITY" ]]; then
      PR_READY_ISSUE="$ISSUE_NUMBER"
      PR_READY_PRIORITY="$ISSUE_PRIORITY"
    fi
  elif [[ -z "$PENDING_ISSUE" ]] || [[ "$ISSUE_PRIORITY" -lt "$PENDING_PRIORITY" ]]; then
    # 有 ai-task 但沒有 in-progress 或 pr-ready，選擇優先級最高的
    if ! echo "$LABELS" | grep -q "in-progress" && ! echo "$LABELS" | grep -q "pr-ready"; then
      PENDING_ISSUE="$ISSUE_NUMBER"
      PENDING_PRIORITY="$ISSUE_PRIORITY"
    fi
  fi
done < <(echo "$ISSUES_JSON" | jq -c '.[]')

# 輸出找到的 issues 數量
TOTAL_ISSUES=$(echo "$ISSUES_JSON" | jq '. | length')
echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-1 | 找到 $TOTAL_ISSUES 個 pending issues"

# 決定下一步
if [[ -n "$IN_PROGRESS_ISSUE" ]]; then
  # 有 in-progress issue 且有 result.json，跳到 Step 4 檢查結果
  CURRENT_ISSUE="$IN_PROGRESS_ISSUE"
  NEXT_STEP=4
elif [[ -n "$PR_READY_ISSUE" ]]; then
  # 有 pr-ready issue，跳到 Step 5 審查 PR
  CURRENT_ISSUE="$PR_READY_ISSUE"
  NEXT_STEP=5
elif [[ -n "$PENDING_ISSUE" ]]; then
  # 有 pending issue，跳到 Step 3 派工
  CURRENT_ISSUE="$PENDING_ISSUE"
  NEXT_STEP=3
else
  # 沒有 pending issues，執行 Step 2 創建新任務
  NEXT_STEP=2
fi
```

### Step 2: 分析並創建新任務

```bash
if [[ "$NEXT_STEP" -eq 2 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-2 | 分析 tasks.md，準備創建任務..."
  
  # 調用 create-task.md
  source .ai/commands/create-task.md
  
  # create-task.md 會 export ISSUE_NUMBER 和 ESCALATED
  if [[ "$ESCALATED" == "true" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-2 | ⚠ 任務觸發升級，跳過"
    NEXT_STEP=1  # 回到 Step 1 處理下一個
  elif [[ -n "$ISSUE_NUMBER" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-2 | ✓ 創建 Issue #$ISSUE_NUMBER"
    CURRENT_ISSUE="$ISSUE_NUMBER"
    NEXT_STEP=3
    CONSECUTIVE_FAILURES=0
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-2 | ✗ 創建失敗或無任務"
    CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
    NEXT_STEP=1  # 回到 Step 1
  fi
fi
```

### Step 3: 派工給 Worker (Codex)

```bash
if [[ "$NEXT_STEP" -eq 3 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-3 | 派工給 Worker (issue #$CURRENT_ISSUE)..."
  
  # 調用 dispatch-worker.md
  ISSUE_NUMBER="$CURRENT_ISSUE" source .ai/commands/dispatch-worker.md
  
  # dispatch-worker.md 會 export WORKER_STATUS
  if [[ "$WORKER_STATUS" == "success" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-3 | ✓ Worker 成功"
    NEXT_STEP=4
    CONSECUTIVE_FAILURES=0
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-3 | ✗ Worker 失敗"
    CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
    NEXT_STEP=1  # 回到 Step 1
  fi
fi
```

### Step 4: 檢查執行結果

```bash
if [[ "$NEXT_STEP" -eq 4 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-4 | Worker 完成，檢查結果..."
  
  # 調用 check-result.md
  ISSUE_NUMBER="$CURRENT_ISSUE" source .ai/commands/check-result.md
  
  # check-result.md 會 export CHECK_RESULT_STATUS 和 PR_NUMBER
  if [[ "$CHECK_RESULT_STATUS" == "success" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-4 | ✓ Worker 成功，PR #$PR_NUMBER"
    NEXT_STEP=5
    CONSECUTIVE_FAILURES=0
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-4 | ✗ Worker 失敗或結果未就緒"
    CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
    NEXT_STEP=1  # 回到 Step 1
  fi
fi
```

### Step 5: 審查 PR

```bash
if [[ "$NEXT_STEP" -eq 5 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-5 | 審查 PR #$PR_NUMBER..."
  
  # 調用 review-pr.md
  PR_NUMBER="$PR_NUMBER" ISSUE_NUMBER="$CURRENT_ISSUE" source .ai/commands/review-pr.md
  
  # review-pr.md 會 export REVIEW_DECISION, MERGE_STATUS, ESCALATED
  if [[ "$ESCALATED" == "true" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-5 | ⚠ PR 觸發升級，需要人工審查"
    NEXT_STEP=1  # 回到 Step 1
  elif [[ "$REVIEW_DECISION" == "approved" ]]; then
    NEXT_STEP=6
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-5 | ✗ 審查不通過"
    CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
    NEXT_STEP=1  # 回到 Step 1
  fi
fi
```

### Step 6: 處理審查結果

```bash
if [[ "$NEXT_STEP" -eq 6 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-6 | 處理審查結果..."
  
  # review-pr.md 已經處理了 approve 和 merge
  # 檢查是否成功合併
  if [[ "$MERGE_STATUS" == "merged" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-6 | ✓ PR #$PR_NUMBER 已合併，issue #$CURRENT_ISSUE 已關閉"
    CONSECUTIVE_FAILURES=0
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STEP-6 | ✗ 合併失敗"
    CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
  fi
  
  NEXT_STEP=1  # 回到 Step 1
fi
```

### 檢查停止條件

```bash
LOOP_COUNT=$((LOOP_COUNT + 1))
echo "[PRINCIPAL] $(date +%H:%M:%S) | LOOP | 回到 Step 1，處理下一個任務..."

# 停止條件 1: 停止標記存在
if [[ -f ".ai/state/STOP" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STOP | 檢測到停止標記"
  EXIT_REASON="user_stopped"
  source .ai/commands/stop-work.md
  bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON"
  exit 0
fi

# 停止條件 2: 連續失敗次數超過限制
if [[ "$CONSECUTIVE_FAILURES" -ge "$MAX_CONSECUTIVE_FAILURES" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STOP | 連續失敗 $CONSECUTIVE_FAILURES 次，超過限制 $MAX_CONSECUTIVE_FAILURES"
  EXIT_REASON="error_exit"
  source .ai/commands/stop-work.md
  bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON"
  exit 1
fi

# 停止條件 3: 所有任務完成
if [[ "$NEXT_STEP" -eq 2 ]] && [[ -z "$ISSUE_NUMBER" ]] && [[ "$ESCALATED" != "true" ]]; then
  # Step 2 沒有創建新任務，且沒有 pending issues
  echo "[PRINCIPAL] $(date +%H:%M:%S) | COMPLETE | ✓ 所有任務完成！"
  EXIT_REASON="all_tasks_complete"
  source .ai/commands/stop-work.md
  bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON"
  exit 0
fi

# 停止條件 4: 人工中斷（互動模式）
# 在互動模式下，檢查用戶輸入
if [[ "$AUTONOMOUS_MODE" != "true" ]]; then
  # 檢查是否有用戶輸入「停止」或「stop」
  read -t 1 -r USER_INPUT 2>/dev/null || true
  if [[ "$USER_INPUT" == "停止" ]] || [[ "$USER_INPUT" == "stop" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | STOP | 用戶請求停止"
    EXIT_REASON="user_stopped"
    source .ai/commands/stop-work.md
    bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON"
    exit 0
  fi
fi

# 停止條件 5: 升級觸發（在 create-task.md 或 review-pr.md 中設置）
if [[ "$ESCALATION_STOP" == "true" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STOP | 升級觸發，需要人工介入"
  EXIT_REASON="escalation_triggered"
  source .ai/commands/stop-work.md
  bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON"
  exit 0
fi

# 停止條件 6: 循環次數過多
if [[ "$LOOP_COUNT" -ge 1000 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | STOP | 循環次數過多 ($LOOP_COUNT)，可能陷入無限循環"
  EXIT_REASON="error_exit"
  source .ai/commands/stop-work.md
  bash .ai/scripts/session_manager.sh end_principal_session "$PRINCIPAL_SESSION_ID" "$EXIT_REASON"
  exit 1
fi

# 繼續下一輪循環
sleep 2
```

重複主循環直到滿足停止條件。

---

## 錯誤處理

如果任何命令執行失敗：

1. **記錄錯誤**：寫入 `.ai/exe-logs/principal-<session_id>.log`
2. **增加失敗計數**：`CONSECUTIVE_FAILURES++`
3. **檢查是否超過限制**：如果 `CONSECUTIVE_FAILURES >= MAX_CONSECUTIVE_FAILURES`，停止工作流
4. **繼續下一個任務**：在自動化模式下，不要停止整個流程

---

## 命令依賴關係

此工作流依賴以下命令文件：

1. **preflight.md** - 前置檢查和配置載入
2. **generate-tasks.md** - 從 design.md 生成 tasks.md
3. **create-task.md** - 創建 GitHub Issue (Step 2)
4. **dispatch-worker.md** - 派工給 Worker (Step 3)
5. **check-result.md** - 檢查 Worker 執行結果 (Step 4)
6. **review-pr.md** - 審查 PR (Step 5 & 6)
7. **stop-work.md** - 停止工作流並生成報告

所有命令文件都位於 `.ai/commands/` 目錄。

---

## 使用範例

```bash
# 啟動工作流
awkit kickoff

# 直接調用
claude --print text -p .ai/commands/start-work.md
```

---

## 輸出報告

每完成一輪循環，簡要報告：
- 處理了哪個 issue
- 結果（merged / rejected / failed）
- 下一步計劃

結束時會調用 stop-work.md 生成詳細總結。

---

## 重新啟動

如果工作流被停止，要重新啟動：

```bash
# 刪除停止標記
rm .ai/state/STOP

# 重新執行
awkit kickoff
```

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

## 架構說明

**模組化設計原則：**

1. **保持原有流程**：Step 1-6 的線性流程完全保留
2. **模組化實現**：每個 Step 內部調用獨立命令文件
3. **輸出格式一致**：使用與 backup 相同的輸出格式
4. **行為完全相同**：執行邏輯與 backup 版本一致

**與 backup 的差異：**

- ❌ backup：所有邏輯嵌入在一個文件中（~1000 行）
- ✅ 重構：邏輯分散到獨立命令文件，start-work.md 負責編排

**優勢：**

- 更容易理解和維護
- 可以單獨測試每個命令
- 可以手動執行特定步驟進行調試
- 更容易擴展新功能
- 減少重複代碼

---

## 注意事項

1. **環境變數**：命令之間通過 `export` 的環境變數傳遞數據
2. **錯誤處理**：每個命令負責自己的錯誤處理，start-work.md 只檢查返回狀態
3. **Session 管理**：所有 session 操作都通過 session_manager.sh 進行
4. **日誌記錄**：重要操作都會記錄到 session log
5. **冪等性**：命令應該設計為可重複執行（如果可能）
