# Analyze Next Command

分析下一個待處理的任務並返回建議行動。

**用途：**
- 在 start-work.md 的主循環中自動調用
- 可獨立執行：`/analyze-next`

**輸出：**
- 導出 `NEXT_ACTION` 環境變數（create_task, dispatch_worker, check_result, review_pr, all_complete, none）
- 導出相關參數（ISSUE_NUMBER, PR_NUMBER, SPEC_NAME 等）
- 返回 0 表示成功，非 0 表示失敗

---

## Step 0: 初始化 Session

```bash
# 檢查是否已有 Principal session (Req 4.7)
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PRINCIPAL_SESSION_ID 未設置，嘗試獲取..."
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
  
  if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 無 active session，這是正常的（可能是獨立執行）"
  else
    export PRINCIPAL_SESSION_ID
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Session ID: $PRINCIPAL_SESSION_ID"
  fi
fi
```

---

## Step 1: 讀取配置

```bash
# 確保環境變數已設置
if [[ -z "$SPEC_BASE_PATH" ]]; then
  SPEC_BASE_PATH=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('specs',{}).get('base_path', '.ai/specs'))" 2>/dev/null || echo ".ai/specs")
fi

if [[ -z "$ACTIVE_SPECS" ]]; then
  ACTIVE_SPECS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(','.join(c.get('specs',{}).get('active', [])))" 2>/dev/null || echo "")
fi

if [[ -z "$LABEL_TASK" ]]; then
  LABEL_TASK=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('task', 'ai-task'))" 2>/dev/null || echo "ai-task")
fi

if [[ -z "$LABEL_IN_PROGRESS" ]]; then
  LABEL_IN_PROGRESS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('in_progress', 'in-progress'))" 2>/dev/null || echo "in-progress")
fi

if [[ -z "$LABEL_PR_READY" ]]; then
  LABEL_PR_READY=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('pr_ready', 'pr-ready'))" 2>/dev/null || echo "pr-ready")
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 配置已載入"
```

---

## Step 2: 檢查 in-progress Issues (Req 4.1, 4.2)

```bash
# 檢查是否有正在執行的 Issue（有 in-progress 標籤）
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 in-progress issues..."

IN_PROGRESS_ISSUES=$(gh issue list --label "$LABEL_IN_PROGRESS" --state open --json number --jq '.[].number' 2>/dev/null || echo "")

if [[ -n "$IN_PROGRESS_ISSUES" ]]; then
  # 取第一個 in-progress issue
  ISSUE_NUMBER=$(echo "$IN_PROGRESS_ISSUES" | head -1)
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 發現 in-progress issue: #$ISSUE_NUMBER"
  
  # 返回 check_result action
  export NEXT_ACTION="check_result"
  export ISSUE_NUMBER
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | NEXT_ACTION=check_result, ISSUE_NUMBER=$ISSUE_NUMBER"
  exit 0
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 無 in-progress issues"
```

---

## Step 3: 檢查 pr-ready Issues (Req 4.2, 4.3)

```bash
# 檢查是否有 PR 待審查的 Issue（有 pr-ready 標籤）
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 pr-ready issues..."

PR_READY_ISSUES=$(gh issue list --label "$LABEL_PR_READY" --state open --json number,body --jq '.[] | "\(.number)|\(.body)"' 2>/dev/null || echo "")

if [[ -n "$PR_READY_ISSUES" ]]; then
  # 取第一個 pr-ready issue
  ISSUE_LINE=$(echo "$PR_READY_ISSUES" | head -1)
  ISSUE_NUMBER=$(echo "$ISSUE_LINE" | cut -d'|' -f1)
  ISSUE_BODY=$(echo "$ISSUE_LINE" | cut -d'|' -f2-)
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 發現 pr-ready issue: #$ISSUE_NUMBER"
  
  # 從 Issue body 或 result.json 提取 PR 編號
  PR_NUMBER=""
  
  # 方法 1: 從 result.json 讀取
  RESULT_FILE=".ai/results/issue-$ISSUE_NUMBER.json"
  if [[ -f "$RESULT_FILE" ]]; then
    PR_URL=$(python3 -c "import json; print(json.load(open('$RESULT_FILE')).get('pr_url',''))" 2>/dev/null || echo "")
    if [[ -n "$PR_URL" ]]; then
      PR_NUMBER=$(echo "$PR_URL" | grep -oP '(?<=pull/)\d+' || echo "")
    fi
  fi
  
  # 方法 2: 從 Issue body 提取（查找 PR #N 或 pull/N）
  if [[ -z "$PR_NUMBER" ]]; then
    PR_NUMBER=$(echo "$ISSUE_BODY" | grep -oP '(?<=#)\d+(?=\s|$)' | head -1 || echo "")
  fi
  
  if [[ -z "$PR_NUMBER" ]]; then
    PR_NUMBER=$(echo "$ISSUE_BODY" | grep -oP '(?<=pull/)\d+' | head -1 || echo "")
  fi
  
  if [[ -n "$PR_NUMBER" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 提取到 PR 編號: #$PR_NUMBER"
    
    # 返回 review_pr action
    export NEXT_ACTION="review_pr"
    export ISSUE_NUMBER
    export PR_NUMBER
    
    echo "[PRINCIPAL] $(date +%H:%M:%S) | NEXT_ACTION=review_pr, ISSUE_NUMBER=$ISSUE_NUMBER, PR_NUMBER=$PR_NUMBER"
    exit 0
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 無法提取 PR 編號，跳過此 issue"
    # 移除 pr-ready 標籤，讓它重新進入 pending 狀態
    gh issue edit "$ISSUE_NUMBER" --remove-label "$LABEL_PR_READY" 2>/dev/null || true
  fi
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 無 pr-ready issues"
```

---

## Step 4: 檢查 pending Issues (Req 4.3, 4.4)

```bash
# 檢查是否有待派工的 Issue（有 ai-task 標籤但沒有 in-progress 或 pr-ready）
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 pending issues..."

PENDING_ISSUES=$(gh issue list --label "$LABEL_TASK" --state open --json number,labels --jq '.[] | select(.labels | map(.name) | (contains(["'$LABEL_IN_PROGRESS'"]) or contains(["'$LABEL_PR_READY'"])) | not) | .number' 2>/dev/null || echo "")

if [[ -n "$PENDING_ISSUES" ]]; then
  # 取第一個 pending issue
  ISSUE_NUMBER=$(echo "$PENDING_ISSUES" | head -1)
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 發現 pending issue: #$ISSUE_NUMBER"
  
  # 返回 dispatch_worker action
  export NEXT_ACTION="dispatch_worker"
  export ISSUE_NUMBER
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | NEXT_ACTION=dispatch_worker, ISSUE_NUMBER=$ISSUE_NUMBER"
  exit 0
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 無 pending issues"
```

---

## Step 5: 檢查 tasks.md 中的未完成任務 (Req 4.4, 4.5)

```bash
# 如果沒有 pending issues，檢查 tasks.md 是否有未完成任務
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 tasks.md 中的未完成任務..."

if [[ -z "$ACTIVE_SPECS" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 沒有 active specs"
else
  # 分割 active specs 列表
  IFS=',' read -ra SPEC_LIST <<< "$ACTIVE_SPECS"
  
  for SPEC_NAME in "${SPEC_LIST[@]}"; do
    # 移除空白
    SPEC_NAME=$(echo "$SPEC_NAME" | tr -d ' ')
    
    if [[ -z "$SPEC_NAME" ]]; then
      continue
    fi
    
    SPEC_PATH="${SPEC_BASE_PATH}/${SPEC_NAME}"
    TASKS_FILE="$SPEC_PATH/tasks.md"
    
    if [[ ! -f "$TASKS_FILE" ]]; then
      continue
    fi
    
    # 查找第一個未完成且沒有 Issue 引用的任務
    # 格式：- [ ] N. 任務名稱（沒有 <!-- Issue #N --> 註釋）
    UNCOMPLETED_TASK=$(grep -n '^\- \[ \] [0-9]' "$TASKS_FILE" | grep -v '<!-- Issue #' | head -1 || echo "")
    
    if [[ -n "$UNCOMPLETED_TASK" ]]; then
      TASK_LINE_NUM=$(echo "$UNCOMPLETED_TASK" | cut -d':' -f1)
      TASK_CONTENT=$(echo "$UNCOMPLETED_TASK" | cut -d':' -f2-)
      
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 發現未完成任務：$SPEC_NAME (line $TASK_LINE_NUM)"
      echo "[PRINCIPAL] $(date +%H:%M:%S) |   $TASK_CONTENT"
      
      # 返回 create_task action
      export NEXT_ACTION="create_task"
      export SPEC_NAME
      export TASK_LINE="$TASK_LINE_NUM"
      
      echo "[PRINCIPAL] $(date +%H:%M:%S) | NEXT_ACTION=create_task, SPEC_NAME=$SPEC_NAME, TASK_LINE=$TASK_LINE"
      exit 0
    fi
  done
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 無未完成任務"
```

---

## Step 6: 檢查是否全部完成 (Req 4.5, 4.6)

```bash
# 如果沒有 pending issues 也沒有未完成任務，檢查是否全部完成
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查是否全部完成..."

# 檢查是否還有任何 open 的 ai-task issues
OPEN_TASK_COUNT=$(gh issue list --label "$LABEL_TASK" --state open --json number --jq '. | length' 2>/dev/null || echo "0")

if [[ "$OPEN_TASK_COUNT" -eq 0 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 所有任務已完成！"
  
  # 返回 all_complete action
  export NEXT_ACTION="all_complete"
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | NEXT_ACTION=all_complete"
  exit 0
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 還有 $OPEN_TASK_COUNT 個 open issues"
```

---

## Step 7: 無待處理任務 (Req 4.6)

```bash
# 如果有 open issues 但都需要人工審查或等待中
echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 無待處理任務（可能都需要人工審查）"

# 返回 none action
export NEXT_ACTION="none"

echo "[PRINCIPAL] $(date +%H:%M:%S) | NEXT_ACTION=none"
exit 0
```

---

## 輸出變數

此命令會 export 以下變數：

- `NEXT_ACTION`: 下一步行動（必定有值）
  - `create_task` - 創建新任務
  - `dispatch_worker` - 派工給 Worker
  - `check_result` - 檢查 Worker 結果
  - `review_pr` - 審查 PR
  - `all_complete` - 所有任務完成
  - `none` - 無待處理任務

- `ISSUE_NUMBER`: Issue 編號（當 action 為 dispatch_worker, check_result, review_pr 時）
- `PR_NUMBER`: PR 編號（當 action 為 review_pr 時）
- `SPEC_NAME`: Spec 名稱（當 action 為 create_task 時）
- `TASK_LINE`: 任務行號（當 action 為 create_task 時）

---

## 使用範例

### 從 start-work.md 調用

```bash
# 在主循環中調用
source .ai/commands/analyze-next.md

# 根據 NEXT_ACTION 路由
case "$NEXT_ACTION" in
  "create_task")
    echo "創建任務：$SPEC_NAME"
    ;;
  "dispatch_worker")
    echo "派工：Issue #$ISSUE_NUMBER"
    ;;
  "check_result")
    echo "檢查結果：Issue #$ISSUE_NUMBER"
    ;;
  "review_pr")
    echo "審查 PR：#$PR_NUMBER (Issue #$ISSUE_NUMBER)"
    ;;
  "all_complete")
    echo "所有任務完成"
    ;;
  "none")
    echo "無待處理任務"
    ;;
esac
```

### 獨立執行

```bash
bash .ai/commands/analyze-next.md
echo "Next action: $NEXT_ACTION"
```

---

## 依賴項

- `gh` CLI (GitHub CLI)
- `python3` with `yaml` module
- `.ai/config/workflow.yaml`
- `.ai/specs/<spec>/tasks.md` files

---

## 決策邏輯

優先級順序（從高到低）：

1. **in-progress issues** → check_result（最高優先級，先完成正在執行的）
2. **pr-ready issues** → review_pr（其次，審查已完成的 PR）
3. **pending issues** → dispatch_worker（然後，派工給 Worker）
4. **uncompleted tasks** → create_task（接著，創建新任務）
5. **no open issues** → all_complete（最後，檢查是否全部完成）
6. **all issues need review** → none（都需要人工介入）

---

## 錯誤處理

- 如果 gh CLI 失敗：忽略錯誤，返回空結果
- 如果配置文件不存在：使用默認值
- 如果無法提取 PR 編號：移除 pr-ready 標籤，讓 issue 重新進入 pending 狀態
- 如果沒有任何待處理任務：返回 `none` action
