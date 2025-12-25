# Dispatch Worker Command

派工給 Worker (Codex) 執行 Issue。

**用途：**
- 在 start-work.md 的 Step 3 中自動調用
- 可獨立執行：`/dispatch-worker <ISSUE_NUMBER>`

**參數：**
- `<ISSUE_NUMBER>`: Issue 編號（必填）

**輸出：**
- 更新 Issue 標籤狀態
- 執行 Worker 並等待完成
- 導出 `WORKER_STATUS` 環境變數
- 返回 0 表示成功，非 0 表示失敗

---

## Step 0: 初始化 Session

```bash
# 檢查是否已有 Principal session (Req 6.9)
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PRINCIPAL_SESSION_ID 未設置，嘗試獲取..."
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
  
  if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 無法獲取 Principal Session ID"
    exit 1
  fi
  
  export PRINCIPAL_SESSION_ID
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | Session ID: $PRINCIPAL_SESSION_ID"

# 檢查參數
if [[ -z "$ISSUE_NUMBER" ]]; then
  if [[ -z "$1" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 缺少 Issue 編號"
    echo "用法: bash .ai/commands/dispatch-worker.md <ISSUE_NUMBER>"
    exit 1
  fi
  ISSUE_NUMBER="$1"
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 派工 Issue #$ISSUE_NUMBER"
```

---

## Step 1: 獲取並驗證 Issue 信息 (Req 6.1)

```bash
# 獲取 Issue 信息
echo "[PRINCIPAL] $(date +%H:%M:%S) | 獲取 Issue 信息..."

ISSUE_DATA=$(gh issue view "$ISSUE_NUMBER" --json number,title,body,labels,state 2>&1)

if [[ $? -ne 0 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 無法獲取 Issue 信息"
  echo "$ISSUE_DATA"
  exit 1
fi

# 驗證 Issue 狀態
ISSUE_STATE=$(echo "$ISSUE_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('state',''))" 2>/dev/null || echo "")
ISSUE_LABELS=$(echo "$ISSUE_DATA" | python3 -c "import json,sys; print(','.join([l['name'] for l in json.load(sys.stdin).get('labels',[])]))" 2>/dev/null || echo "")

if [[ "$ISSUE_STATE" != "OPEN" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ Issue 不是 open 狀態：$ISSUE_STATE"
  exit 1
fi

if [[ ! "$ISSUE_LABELS" =~ "ai-task" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ Issue 沒有 ai-task 標籤"
  exit 1
fi

if [[ "$ISSUE_LABELS" =~ "in-progress" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Issue 已經在執行中"
  export WORKER_STATUS="in_progress"
  exit 0
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Issue 驗證通過"
```

---

## Step 2: 準備 Ticket 文件 (Req 6.2, 6.3)

```bash
# 提取 Issue body 並保存到臨時文件
echo "[PRINCIPAL] $(date +%H:%M:%S) | 準備 ticket 文件..."

ISSUE_BODY=$(echo "$ISSUE_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('body',''))" 2>/dev/null || echo "")

TICKET_FILE=".ai/temp/ticket-$ISSUE_NUMBER.md"
mkdir -p .ai/temp

echo "$ISSUE_BODY" > "$TICKET_FILE"

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Ticket 文件已保存：$TICKET_FILE"

# 解析 ticket metadata（支援兩種格式）
# 格式 1: **Repo**: xxx (新版)
# 格式 2: - Repo: xxx (舊版)
REPO=$(echo "$ISSUE_BODY" | grep -oP '(?<=\*\*Repo\*\*: )[^\n]+' | head -1 || echo "")
if [[ -z "$REPO" ]]; then
  REPO=$(echo "$ISSUE_BODY" | grep -oP '(?<=- Repo: )[^\n]+' | head -1 || echo "root")
fi

COORDINATION=$(echo "$ISSUE_BODY" | grep -oP '(?<=- Coordination: )\w+' | head -1 || echo "sequential")
SYNC_MODE=$(echo "$ISSUE_BODY" | grep -oP '(?<=- Sync: )\w+' | head -1 || echo "independent")
PRIORITY=$(echo "$ISSUE_BODY" | grep -oP '(?<=\*\*Priority\*\*: )[^\n]+' | head -1 || echo "")
if [[ -z "$PRIORITY" ]]; then
  PRIORITY=$(echo "$ISSUE_BODY" | grep -oP '(?<=- Priority: )[^\n]+' | head -1 || echo "P2")
fi

RELEASE=$(echo "$ISSUE_BODY" | grep -oP '(?<=\*\*Release\*\*: )(true|false)' | head -1 || echo "")
if [[ -z "$RELEASE" ]]; then
  RELEASE=$(echo "$ISSUE_BODY" | grep -oP '(?<=- Release: )(true|false)' | head -1 || echo "false")
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | Repo: $REPO"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Coordination: $COORDINATION"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Sync: $SYNC_MODE"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Priority: $PRIORITY"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Release: $RELEASE"
```

---

## Step 3: 檢測 Repo 類型 (Req 6.3, 6.4)

```bash
# 從 workflow.yaml 讀取 repo 配置
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢測 repo 類型..."

# 處理多 repo 情況
REPOS_CLEAN=$(echo "$REPO" | tr -d ' ')

REPO_TYPE=$(python3 -c "
import yaml
try:
    config = yaml.safe_load(open('.ai/config/workflow.yaml'))
    repos = config.get('repos', {})
    repo_name = '$REPOS_CLEAN'.split(',')[0]  # 取第一個 repo
    if repo_name in repos:
        print(repos[repo_name].get('type', 'root'))
    else:
        print('root')
except:
    print('root')
" 2>/dev/null || echo "root")

REPO_PATH=$(python3 -c "
import yaml
try:
    config = yaml.safe_load(open('.ai/config/workflow.yaml'))
    repos = config.get('repos', {})
    repo_name = '$REPOS_CLEAN'.split(',')[0]  # 取第一個 repo
    if repo_name in repos:
        print(repos[repo_name].get('path', './'))
    else:
        print('./')
except:
    print('./')
" 2>/dev/null || echo "./")

echo "[PRINCIPAL] $(date +%H:%M:%S) | Repo type: $REPO_TYPE"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Repo path: $REPO_PATH"
```

---

## Step 4: 標記 Issue 為 in-progress (Req 6.5, 6.6)

```bash
# 標記 Issue 為 in-progress
echo "[PRINCIPAL] $(date +%H:%M:%S) | 標記 Issue 為 in-progress..."

gh issue edit "$ISSUE_NUMBER" --add-label "in-progress" 2>/dev/null || true

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Issue 已標記為 in-progress"
```

---

## Step 5: 記錄 worker_dispatched (Req 6.8)

```bash
# 記錄 worker_dispatched action
echo "[PRINCIPAL] $(date +%H:%M:%S) | 記錄 worker_dispatched..."

bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "worker_dispatched" "{\"issue_id\":\"$ISSUE_NUMBER\",\"repo\":\"$REPO\"}"

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已記錄 worker_dispatched"
```

---

## Step 6: 執行 Worker - Multi-Repo 支援 (Req 6.6, 6.7, 17.1-17.4)

```bash
# 執行 Worker
echo "[PRINCIPAL] $(date +%H:%M:%S) | 執行 Worker..."
echo ""

# 檢查是否為多 repo
if echo "$REPOS_CLEAN" | grep -q ","; then
  # Multi-Repo 處理
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢測到多 repo 任務"
  
  IFS=',' read -ra REPO_LIST <<< "$REPOS_CLEAN"
  
  if [[ "$COORDINATION" == "sequential" ]]; then
    # 依序執行每個 repo (Req 17.1-17.4)
    echo "[PRINCIPAL] $(date +%H:%M:%S) | 使用 sequential 協調模式"
    
    for CURRENT_REPO in "${REPO_LIST[@]}"; do
      CURRENT_REPO=$(echo "$CURRENT_REPO" | tr -d ' ')
      echo "[PRINCIPAL] $(date +%H:%M:%S) | 處理 repo: $CURRENT_REPO"
      
      # 獲取 repo type 以決定處理方式
      CURRENT_REPO_TYPE=$(python3 -c "
import yaml
try:
    config = yaml.safe_load(open('.ai/config/workflow.yaml'))
    repos = config.get('repos', {})
    if '$CURRENT_REPO' in repos:
        print(repos['$CURRENT_REPO'].get('type', 'directory'))
    else:
        print('directory')
except:
    print('directory')
" 2>/dev/null || echo "directory")
      
      echo "[PRINCIPAL] $(date +%H:%M:%S) | Repo type: $CURRENT_REPO_TYPE"
      
      # 調用 run_issue_codex.sh
      bash .ai/scripts/run_issue_codex.sh "$ISSUE_NUMBER" "$TICKET_FILE" "$CURRENT_REPO"
      WORKER_EXIT_CODE=$?
      
      # 檢查結果，如果失敗則停止 (Req 17.3)
      RESULT_STATUS=$(cat ".ai/results/issue-$ISSUE_NUMBER.json" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
      
      if [[ "$RESULT_STATUS" != "success" ]]; then
        echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 在 repo $CURRENT_REPO (type: $CURRENT_REPO_TYPE) 失敗，停止 sequential 執行"
        
        # 對於 submodule type，檢查一致性狀態 (Req 17.4)
        if [[ "$CURRENT_REPO_TYPE" == "submodule" ]]; then
          CONSISTENCY=$(cat ".ai/results/issue-$ISSUE_NUMBER.json" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('consistency_status',''))" 2>/dev/null || echo "")
          
          if [[ "$CONSISTENCY" != "consistent" ]] && [[ -n "$CONSISTENCY" ]]; then
            echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ WARNING: Submodule 處於不一致狀態: $CONSISTENCY"
            
            RECOVERY=$(cat ".ai/results/issue-$ISSUE_NUMBER.json" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('recovery_command',''))" 2>/dev/null || echo "")
            
            if [[ -n "$RECOVERY" ]]; then
              echo "[PRINCIPAL] $(date +%H:%M:%S) | Recovery command: $RECOVERY"
            fi
          fi
        fi
        
        break
      fi
      
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Repo $CURRENT_REPO 完成"
    done
    
  elif [[ "$COORDINATION" == "parallel" ]]; then
    # 並行執行（目前降級為 sequential，因為需要多 Worker 支援）
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Warning: parallel coordination 尚未完全支援，使用 sequential"
    
    for CURRENT_REPO in "${REPO_LIST[@]}"; do
      CURRENT_REPO=$(echo "$CURRENT_REPO" | tr -d ' ')
      echo "[PRINCIPAL] $(date +%H:%M:%S) | 處理 repo: $CURRENT_REPO"
      
      bash .ai/scripts/run_issue_codex.sh "$ISSUE_NUMBER" "$TICKET_FILE" "$CURRENT_REPO"
    done
  fi
  
else
  # 單一 repo
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 單一 repo 任務: $REPOS_CLEAN"
  
  bash .ai/scripts/run_issue_codex.sh "$ISSUE_NUMBER" "$TICKET_FILE" "$REPOS_CLEAN"
  WORKER_EXIT_CODE=$?
fi

echo ""
echo "[PRINCIPAL] $(date +%H:%M:%S) | Worker 執行完成"
```

---

## Step 7: 檢查執行結果 (Req 6.9, 6.10)

```bash
# 檢查 result.json
RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"

if [[ ! -f "$RESULT_FILE" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Result 文件不存在：$RESULT_FILE"
  WORKER_STATUS="failed"
else
  WORKER_STATUS=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('status','failed'))" 2>/dev/null || echo "failed")
  PR_URL=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('pr_url',''))" 2>/dev/null || echo "")
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | Worker status: $WORKER_STATUS"
  
  if [[ -n "$PR_URL" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | PR URL: $PR_URL"
  fi
fi
```

---

## Step 8: 處理成功情況 (Req 6.9)

```bash
if [[ "$WORKER_STATUS" == "success" ]] && [[ -n "$PR_URL" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Worker 成功"
  
  # 更新 Issue 標籤：移除 in-progress，添加 pr-ready
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" --add-label "pr-ready" 2>/dev/null || true
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Issue 標籤已更新 (in-progress → pr-ready)"
  
  # 記錄 worker_completed
  bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "worker_completed" "{\"issue_id\":\"$ISSUE_NUMBER\",\"status\":\"success\",\"pr_url\":\"$PR_URL\"}"
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已記錄 worker_completed"
  
  # 導出狀態
  export WORKER_STATUS="success"
  
  exit 0
fi
```

---

## Step 9: 處理失敗情況 (Req 6.7, 6.8)

```bash
echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ Worker 失敗"

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
  
  # 記錄最終失敗
  bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "worker_failed" "{\"issue_id\":\"$ISSUE_NUMBER\",\"attempts\":$FAIL_COUNT}"
  
  export WORKER_STATUS="failed"
  exit 1
else
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 將在下一輪重試 (attempt $FAIL_COUNT/3)"
  
  # 移除 in-progress 標籤，下一輪可以重新派工
  gh issue edit "$ISSUE_NUMBER" --remove-label "in-progress" 2>/dev/null || true
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已移除 in-progress 標籤"
  
  export WORKER_STATUS="failed"
  exit 1
fi
```

---

## 輸出變數

此命令會 export 以下變數：

- `WORKER_STATUS`: Worker 執行狀態
  - `success` - 成功
  - `failed` - 失敗
  - `in_progress` - 已在執行中

---

## Multi-Repo Ticket 格式

```markdown
- Repo: backend, frontend
- Coordination: sequential  # sequential | parallel
- Sync: required           # required | independent
```

- `sequential`: 依序執行，前一個成功才執行下一個
- `parallel`: 並行執行（需要多 Worker）
- `Sync: required`: 所有 repo 的 PR 必須同時合併
- `Sync: independent`: 各 repo 的 PR 可獨立合併

---

## 使用範例

### 從 start-work.md 調用

```bash
ISSUE_NUMBER=123
source .ai/commands/dispatch-worker.md

if [[ "$WORKER_STATUS" == "success" ]]; then
  echo "Worker 成功"
  CONSECUTIVE_FAILURES=0
else
  echo "Worker 失敗"
  CONSECUTIVE_FAILURES=$((CONSECUTIVE_FAILURES + 1))
fi
```

### 獨立執行

```bash
bash .ai/commands/dispatch-worker.md 123
```

---

## 依賴項

- `gh` CLI (GitHub CLI)
- `python3` with `yaml` and `json` modules
- `.ai/config/workflow.yaml`
- `.ai/scripts/session_manager.sh`
- `.ai/scripts/run_issue_codex.sh`

---

## 輸出文件

- `.ai/temp/ticket-<N>.md` - Issue body 臨時文件
- `.ai/results/issue-<N>.json` - Worker 執行結果
- `.ai/runs/issue-<N>/fail_count.txt` - 失敗次數計數

---

## 錯誤處理

- 如果 PRINCIPAL_SESSION_ID 未設置：嘗試獲取，失敗則退出
- 如果 ISSUE_NUMBER 未提供：報錯並退出
- 如果 Issue 不存在或狀態不正確：報錯並退出
- 如果 Worker 失敗：增加 fail_count，達到 3 次後標記 `worker-failed`
- 如果 gh CLI 失敗：忽略錯誤繼續執行（使用 `|| true`）
- 對於 submodule 類型：檢查一致性狀態並提供恢復命令
