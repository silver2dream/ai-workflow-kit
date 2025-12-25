# Check Result Command

檢查 Worker 執行結果，更新 Issue 狀態。

**用途：**
- 在 start-work.md 的 Step 4 中自動調用
- 可獨立執行：`ISSUE_NUMBER=<N> bash .ai/commands/check-result.md`

**參數：**
- `ISSUE_NUMBER`: Issue 編號（必填，通過環境變數傳入）

**輸出：**
- 更新 Issue 標籤
- 記錄 session actions
- 導出 `CHECK_RESULT_STATUS`, `WORKER_STATUS`, `PR_NUMBER` 環境變數
- 返回 0

---

## Step 0: 初始化 Session

```bash
# 檢查是否已有 Principal session
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
fi

# 如果沒有 session，嘗試獲取
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PRINCIPAL_SESSION_ID 未設置"
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh init_principal_session 2>/dev/null || echo "")
fi

export PRINCIPAL_SESSION_ID
echo "[PRINCIPAL] $(date +%H:%M:%S) | Session: $PRINCIPAL_SESSION_ID"
```

---

## Step 1: 驗證參數

```bash
if [[ -z "$ISSUE_NUMBER" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ Error: ISSUE_NUMBER not provided"
  exit 1
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 Issue #$ISSUE_NUMBER 的執行結果..."
```

---

## Step 2: 讀取 result.json

```bash
RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"

if [[ ! -f "$RESULT_FILE" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Result 文件不存在: $RESULT_FILE"
  export CHECK_RESULT_STATUS="not_found"
  export WORKER_STATUS="not_found"
  exit 0
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 讀取 result 文件: $RESULT_FILE"
```

---

## Step 3: 提取結果信息

```bash
# 提取 Worker session ID
WORKER_SESSION_ID=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('session',{}).get('worker_session_id',''))" 2>/dev/null || echo "")
if [[ -z "$WORKER_SESSION_ID" ]]; then
  WORKER_SESSION_ID=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('worker_session_id',''))" 2>/dev/null || echo "")
fi

# 提取 status
WORKER_STATUS=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('status',''))" 2>/dev/null || echo "")

# 提取 PR URL
PR_URL=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('pr_url',''))" 2>/dev/null || echo "")

# 提取 consistency_status（用於 submodule）
CONSISTENCY_STATUS=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('consistency_status',''))" 2>/dev/null || echo "")

echo "[PRINCIPAL] $(date +%H:%M:%S) | Worker Session: $WORKER_SESSION_ID"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Status: $WORKER_STATUS"
echo "[PRINCIPAL] $(date +%H:%M:%S) | PR URL: $PR_URL"
if [[ -n "$CONSISTENCY_STATUS" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | Consistency: $CONSISTENCY_STATUS"
fi
```

---

## Step 4: 記錄 Worker 完成 (Req 1.5)

```bash
if [[ -n "$WORKER_SESSION_ID" ]] && [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  bash .ai/scripts/session_manager.sh update_worker_completion "$PRINCIPAL_SESSION_ID" "$ISSUE_NUMBER" "$WORKER_SESSION_ID" "$WORKER_STATUS" "$PR_URL" 2>/dev/null || true
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已記錄 worker completion"
fi
```

---

## Step 5: 更新 Principal Session ID (Req 6.3)

```bash
if [[ -n "$PRINCIPAL_SESSION_ID" ]]; then
  bash .ai/scripts/session_manager.sh update_result_with_principal_session "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" 2>/dev/null || true
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已更新 result.json 的 principal_session_id"
fi
```

---

## Step 6: 處理成功情況

```bash
if [[ "$WORKER_STATUS" == "success" ]] && [[ -n "$PR_URL" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Worker 成功，PR 已創建: $PR_URL"
  
  # 提取 PR 編號
  PR_NUMBER=$(echo "$PR_URL" | grep -oP '(?<=pull/)\d+' || echo "")
  
  if [[ -n "$PR_NUMBER" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | PR Number: #$PR_NUMBER"
    export PR_NUMBER
  fi
  
  # 更新 issue 標籤：移除 in-progress，添加 pr-ready
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" --add-label "pr-ready" 2>/dev/null || true
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Issue 標籤已更新 (in-progress → pr-ready)"
  
  # 導出狀態
  export CHECK_RESULT_STATUS="success"
  export WORKER_STATUS="success"
  
  exit 0
fi
```

---

## Step 7: 處理失敗情況

```bash
echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ Worker 失敗或無 PR 創建"

# 讀取失敗次數
FAIL_COUNT_FILE=".ai/runs/issue-$ISSUE_NUMBER/fail_count.txt"
mkdir -p ".ai/runs/issue-$ISSUE_NUMBER"

FAIL_COUNT=0
if [[ -f "$FAIL_COUNT_FILE" ]]; then
  FAIL_COUNT=$(cat "$FAIL_COUNT_FILE" 2>/dev/null || echo "0")
fi

FAIL_COUNT=$((FAIL_COUNT + 1))
echo "$FAIL_COUNT" > "$FAIL_COUNT_FILE"

echo "[PRINCIPAL] $(date +%H:%M:%S) | 失敗次數: $FAIL_COUNT / 3"

# 檢查是否達到最大重試次數
if [[ "$FAIL_COUNT" -ge 3 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 達到最大重試次數 (3)"
  
  # 標記為 worker-failed
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" --add-label "worker-failed" 2>/dev/null || true
  
  # 添加評論
  gh issue comment "$ISSUE_NUMBER" --body "Worker 已失敗 3 次，需要人工介入。

請檢查：
1. 任務描述是否清晰
2. 是否有技術難點
3. 是否需要調整任務範圍

執行日誌位置：\`.ai/runs/issue-$ISSUE_NUMBER/\`" 2>/dev/null || true
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已標記為 worker-failed"
  
  export CHECK_RESULT_STATUS="failed_max_retries"
else
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 將在下一輪重試 (attempt $FAIL_COUNT/3)"
  
  # 移除 in-progress 標籤，下一輪可以重新派工
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" 2>/dev/null || true
  
  export CHECK_RESULT_STATUS="failed_will_retry"
fi

export WORKER_STATUS="failed"
exit 0
```

---

## 輸出變數

此命令會 export 以下變數：

- `CHECK_RESULT_STATUS`: 檢查結果狀態
  - `success` - Worker 成功，PR 已創建
  - `failed_will_retry` - Worker 失敗，會重試
  - `failed_max_retries` - Worker 失敗，已達最大重試次數
  - `not_found` - 結果文件不存在
  
- `WORKER_STATUS`: Worker 執行狀態
  - `success` - 成功
  - `failed` - 失敗
  - `not_found` - 結果不存在
  
- `PR_NUMBER`: PR 編號（如果有創建）

---

## 使用範例

### 從 start-work.md 調用

```bash
ISSUE_NUMBER=123
source .ai/commands/check-result.md

# 檢查結果
if [[ "$CHECK_RESULT_STATUS" == "success" ]]; then
  echo "Worker 成功，PR #$PR_NUMBER"
  CONSECUTIVE_FAILURES=0
else
  echo "Worker 失敗"
  CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
fi
```

### 獨立執行

```bash
ISSUE_NUMBER=123 bash .ai/commands/check-result.md
```

---

## 依賴項

- `gh` CLI (GitHub CLI)
- `python3` with `json` module
- `.ai/scripts/session_manager.sh`
- `.ai/results/issue-<N>.json` (result file)

---

## 輸出文件

- `.ai/runs/issue-<N>/fail_count.txt` - 失敗次數計數

---

## 錯誤處理

- 如果 ISSUE_NUMBER 未提供：報錯並退出
- 如果 result.json 不存在：設置 status 為 `not_found` 並退出
- 如果 Worker 失敗：增加 fail_count，達到 3 次後標記 `worker-failed`
- 如果 gh CLI 失敗：忽略錯誤繼續執行（使用 `|| true`）
