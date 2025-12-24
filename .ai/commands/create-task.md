# Create Task Command

從 tasks.md 創建 GitHub Issue。

**用途：**
- 在 start-work.md 的 Step 2 中自動調用
- 可獨立執行：`/create-task` 或 `/create-task <spec_name>` 或 `/create-task --autonomous`

**參數：**
- `<spec_name>`: 指定要處理的 spec（可選，默認處理第一個有未完成任務的 spec）
- `--autonomous`: 自動化模式，不詢問用戶確認

**輸出：**
- 創建 GitHub Issue
- 更新 tasks.md 添加 Issue 引用
- 導出 `ISSUE_NUMBER` 環境變數
- 導出 `ESCALATED` 環境變數（如果觸發升級）
- 返回 0 表示成功，非 0 表示失敗

---

## Step 0: 初始化

```bash
# 檢查環境變數
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PRINCIPAL_SESSION_ID 未設置，嘗試獲取..."
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
  
  if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 無法獲取 Principal Session ID"
    exit 1
  fi
  
  export PRINCIPAL_SESSION_ID
fi

if [[ -z "$SPEC_BASE_PATH" ]]; then
  SPEC_BASE_PATH=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('specs',{}).get('base_path', '.ai/specs'))" 2>/dev/null || echo ".ai/specs")
fi

if [[ -z "$ACTIVE_SPECS" ]]; then
  ACTIVE_SPECS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(','.join(c.get('specs',{}).get('active', [])))" 2>/dev/null || echo "")
fi

if [[ -z "$INTEGRATION_BRANCH" ]]; then
  INTEGRATION_BRANCH=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('git',{}).get('integration_branch', 'main'))" 2>/dev/null || echo "main")
fi

# 檢查參數
AUTONOMOUS_MODE=false
TARGET_SPEC=""

for arg in "$@"; do
  if [[ "$arg" == "--autonomous" ]]; then
    AUTONOMOUS_MODE=true
  elif [[ -z "$TARGET_SPEC" ]]; then
    TARGET_SPEC="$arg"
  fi
done

# 如果通過環境變數傳入 SPEC_NAME，使用它
if [[ -n "$SPEC_NAME" ]]; then
  TARGET_SPEC="$SPEC_NAME"
fi

# 初始化 ESCALATED 標記
export ESCALATED=false
export ESCALATION_STOP=false

echo "[PRINCIPAL] $(date +%H:%M:%S) | Session: $PRINCIPAL_SESSION_ID"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Target spec: ${TARGET_SPEC:-auto}"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Autonomous: $AUTONOMOUS_MODE"
```

---

## Step 1: 找到要處理的 Spec 和任務 (Req 5.1)

```bash
# 如果沒有指定 spec，從 active specs 中找第一個有未完成任務的
if [[ -z "$TARGET_SPEC" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 尋找有未完成任務的 spec..."
  
  IFS=',' read -ra SPEC_LIST <<< "$ACTIVE_SPECS"
  
  for SPEC_NAME in "${SPEC_LIST[@]}"; do
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
    UNCOMPLETED_TASK=$(grep -n '^\- \[ \] [0-9]' "$TASKS_FILE" | grep -v '<!-- Issue #' | head -1 || echo "")
    
    if [[ -n "$UNCOMPLETED_TASK" ]]; then
      TARGET_SPEC="$SPEC_NAME"
      break
    fi
  done
  
  if [[ -z "$TARGET_SPEC" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 沒有找到未完成的任務"
    export ISSUE_NUMBER=""
    exit 0
  fi
fi

SPEC_PATH="${SPEC_BASE_PATH}/${TARGET_SPEC}"
TASKS_FILE="$SPEC_PATH/tasks.md"

echo "[PRINCIPAL] $(date +%H:%M:%S) | 處理 spec: $TARGET_SPEC"

# 檢查 tasks.md 是否存在
if [[ ! -f "$TASKS_FILE" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ tasks.md 不存在：$TASKS_FILE"
  export ISSUE_NUMBER=""
  exit 0
fi

# 讀取第一個未完成且沒有 Issue 引用的任務（選擇編號最小的）
TASK_LINE=$(grep -n '^\- \[ \] [0-9]' "$TASKS_FILE" | grep -v '<!-- Issue #' | head -1 || echo "")

if [[ -z "$TASK_LINE" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 沒有未完成的任務"
  export ISSUE_NUMBER=""
  exit 0
fi

TASK_LINE_NUM=$(echo "$TASK_LINE" | cut -d':' -f1)
TASK_CONTENT=$(echo "$TASK_LINE" | cut -d':' -f2-)

echo "[PRINCIPAL] $(date +%H:%M:%S) | 找到任務 (line $TASK_LINE_NUM)："
echo "[PRINCIPAL] $(date +%H:%M:%S) |   $TASK_CONTENT"
```

---

## Step 2: 解析任務內容 (Req 5.2)

```bash
# 提取任務編號和標題
# 格式：- [ ] N. 任務標題
TASK_NUMBER=$(echo "$TASK_CONTENT" | grep -oP '^\- \[ \] \K[0-9.]+' || echo "")
TASK_TITLE=$(echo "$TASK_CONTENT" | sed 's/^- \[ \] [0-9.]* //' || echo "")

if [[ -z "$TASK_NUMBER" ]] || [[ -z "$TASK_TITLE" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 無法解析任務編號或標題"
  exit 1
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 任務編號: $TASK_NUMBER"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 任務標題: $TASK_TITLE"

# 讀取任務的詳細描述（縮進的行）
TASK_DESCRIPTION=""
READING_DESC=false
LINE_NUM=$TASK_LINE_NUM

while IFS= read -r line; do
  LINE_NUM=$((LINE_NUM + 1))
  
  # 如果遇到下一個任務，停止
  if [[ "$line" =~ ^-\ \[.*\]\ [0-9] ]]; then
    break
  fi
  
  # 如果是縮進的行，添加到描述
  if [[ "$line" =~ ^[[:space:]]+-  ]] || [[ "$line" =~ ^[[:space:]]+[^-] ]]; then
    TASK_DESCRIPTION="${TASK_DESCRIPTION}${line}\n"
  fi
done < <(tail -n +$((TASK_LINE_NUM + 1)) "$TASKS_FILE")

# 提取 metadata（Repo, Priority, Release 等）
REPO=$(echo -e "$TASK_DESCRIPTION" | grep -oP '(?<=Repo: )[^\n]+' | head -1 || echo "root")
COORDINATION=$(echo -e "$TASK_DESCRIPTION" | grep -oP '(?<=Coordination: )[^\n]+' | head -1 || echo "sequential")
SYNC_MODE=$(echo -e "$TASK_DESCRIPTION" | grep -oP '(?<=Sync: )[^\n]+' | head -1 || echo "independent")
PRIORITY=$(echo -e "$TASK_DESCRIPTION" | grep -oP '(?<=Priority: )[^\n]+' | head -1 || echo "P2")
RELEASE=$(echo -e "$TASK_DESCRIPTION" | grep -oP '(?<=Release: )(true|false)' | head -1 || echo "false")

echo "[PRINCIPAL] $(date +%H:%M:%S) | Repo: $REPO"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Coordination: $COORDINATION"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Sync: $SYNC_MODE"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Priority: $PRIORITY"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Release: $RELEASE"
```

---

## Step 3: 執行 Escalation 檢查 (Req 5.3)

```bash
# 讀取 escalation triggers
ESCALATION_TRIGGERS=$(python3 -c "
import yaml
try:
    config = yaml.safe_load(open('.ai/config/workflow.yaml'))
    triggers = config.get('escalation', {}).get('triggers', [])
    for t in triggers:
        print(f\"{t.get('pattern', '')}|{t.get('action', '')}\")
except:
    pass
" 2>/dev/null || echo "")

if [[ -n "$ESCALATION_TRIGGERS" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 escalation triggers..."
  
  while IFS='|' read -r pattern action; do
    if [[ -z "$pattern" ]]; then
      continue
    fi
    
    # 檢查任務內容是否匹配 pattern
    if echo "$TASK_TITLE $TASK_DESCRIPTION" | grep -qiE "$pattern"; then
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 觸發 escalation: $pattern (action: $action)"
      
      if [[ "$action" == "require_human_approval" ]]; then
        if [[ "$AUTONOMOUS_MODE" == "true" ]]; then
          echo "[PRINCIPAL] $(date +%H:%M:%S) | 自動化模式：跳過此任務，標記為需要人工審查"
          export ESCALATED=true
          # 在任務行添加標記
          sed -i "${TASK_LINE_NUM}s/$/ <!-- Escalated: needs-human-review -->/" "$TASKS_FILE"
          echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已標記任務為 escalated"
          exit 0
        else
          echo "[PRINCIPAL] $(date +%H:%M:%S) | 此任務需要人工確認"
          echo "是否繼續創建 Issue？(y/n)"
          read -r CONFIRM
          
          if [[ "$CONFIRM" != "y" ]] && [[ "$CONFIRM" != "Y" ]]; then
            echo "[PRINCIPAL] $(date +%H:%M:%S) | 用戶取消"
            export ESCALATION_STOP=true
            exit 0
          fi
        fi
      elif [[ "$action" == "pause_and_ask" ]]; then
        if [[ "$AUTONOMOUS_MODE" == "true" ]]; then
          echo "[PRINCIPAL] $(date +%H:%M:%S) | 自動化模式：跳過此任務，標記為需要審查"
          export ESCALATED=true
          sed -i "${TASK_LINE_NUM}s/$/ <!-- Escalated: needs-review -->/" "$TASKS_FILE"
          echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已標記任務為 escalated"
          exit 0
        else
          echo "[PRINCIPAL] $(date +%H:%M:%S) | 此任務需要確認"
          echo "是否繼續？(y/n)"
          read -r CONFIRM
          
          if [[ "$CONFIRM" != "y" ]] && [[ "$CONFIRM" != "Y" ]]; then
            echo "[PRINCIPAL] $(date +%H:%M:%S) | 用戶取消"
            export ESCALATION_STOP=true
            exit 0
          fi
        fi
      elif [[ "$action" == "notify_only" ]]; then
        echo "[PRINCIPAL] $(date +%H:%M:%S) | 通知：匹配敏感模式，繼續執行"
        # 記錄到 log
        bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "escalation_notify" "{\"pattern\":\"$pattern\",\"task\":\"$TASK_NUMBER\"}" 2>/dev/null || true
      fi
    fi
  done <<< "$ESCALATION_TRIGGERS"
fi
```

---

## Step 4: 讀取 Verification 命令

```bash
# 從 workflow.yaml 讀取 repo 的 verify 命令
echo "[PRINCIPAL] $(date +%H:%M:%S) | 讀取 verification 命令..."

VERIFY_BUILD=$(python3 -c "
import yaml
try:
    config = yaml.safe_load(open('.ai/config/workflow.yaml'))
    repos = config.get('repos', {})
    repo_name = '$REPO'.split(',')[0].strip()
    if repo_name in repos:
        print(repos[repo_name].get('verify', {}).get('build', 'go build ./...'))
    else:
        print('go build ./...')
except:
    print('go build ./...')
" 2>/dev/null || echo "go build ./...")

VERIFY_TEST=$(python3 -c "
import yaml
try:
    config = yaml.safe_load(open('.ai/config/workflow.yaml'))
    repos = config.get('repos', {})
    repo_name = '$REPO'.split(',')[0].strip()
    if repo_name in repos:
        print(repos[repo_name].get('verify', {}).get('test', 'go test ./...'))
    else:
        print('go test ./...')
except:
    print('go test ./...')
" 2>/dev/null || echo "go test ./...")

echo "[PRINCIPAL] $(date +%H:%M:%S) | Build: $VERIFY_BUILD"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Test: $VERIFY_TEST"
```

---

## Step 5: 生成 Issue Body (Req 5.4) - 完整 Ticket 模板

```bash
# 讀取 requirements.md 和 design.md（如果存在）
REQUIREMENTS_FILE="$SPEC_PATH/requirements.md"
DESIGN_FILE="$SPEC_PATH/design.md"

REQUIREMENTS_SUMMARY=""
DESIGN_SUMMARY=""

if [[ -f "$REQUIREMENTS_FILE" ]]; then
  REQUIREMENTS_SUMMARY=$(head -50 "$REQUIREMENTS_FILE" | grep -A 5 "^## " | head -20 || echo "參考 requirements.md")
fi

if [[ -f "$DESIGN_FILE" ]]; then
  DESIGN_SUMMARY=$(head -50 "$DESIGN_FILE" | grep -A 5 "^## " | head -20 || echo "參考 design.md")
fi

# 生成完整的 Issue body（符合舊版 Ticket 模板）
ISSUE_BODY="# Task $TASK_NUMBER: $TASK_TITLE

- Repo: $REPO
- Coordination: $COORDINATION
- Sync: $SYNC_MODE
- Priority: $PRIORITY
- Release: $RELEASE

## Objective

$TASK_TITLE

## Scope

- 實現 Task $TASK_NUMBER 的功能
$(echo -e "$TASK_DESCRIPTION" | grep -E '^\s+-' | head -10 || echo "- 參考 tasks.md 中的任務描述")

## Non-goals

- 不在此任務範圍內的功能
- 不修改與此任務無關的代碼

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

- Build: \`$VERIFY_BUILD\`
- Test: \`$VERIFY_TEST\`

## Acceptance Criteria

- [ ] Implementation matches Objective and Scope
- [ ] Verification commands executed and pass
- [ ] Commit message uses \`[type] subject\` (lowercase)
- [ ] PR targets $INTEGRATION_BRANCH and includes \`Closes #<IssueID>\` in body

## References

### Spec: $TARGET_SPEC

### Requirements Reference

$REQUIREMENTS_SUMMARY

### Design Reference

$DESIGN_SUMMARY

---

**Note**: 此 Issue 由 AWK Principal 自動創建。
**Session**: $PRINCIPAL_SESSION_ID
"

echo "[PRINCIPAL] $(date +%H:%M:%S) | Issue body 已生成"
```

---

## Step 6: 創建 GitHub Issue (Req 5.5)

```bash
# 創建 Issue
echo "[PRINCIPAL] $(date +%H:%M:%S) | 創建 GitHub Issue..."

# 確定標籤
LABELS="ai-task,$PRIORITY"

# 創建 Issue
ISSUE_URL=$(echo "$ISSUE_BODY" | gh issue create \
  --title "[$TARGET_SPEC] Task $TASK_NUMBER: $TASK_TITLE" \
  --body-file - \
  --label "$LABELS" \
  2>&1)

if [[ $? -ne 0 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 創建 Issue 失敗"
  echo "$ISSUE_URL"
  exit 1
fi

# 提取 Issue 編號
ISSUE_NUMBER=$(echo "$ISSUE_URL" | grep -oP '(?<=/issues/)\d+' || echo "")

if [[ -z "$ISSUE_NUMBER" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 無法提取 Issue 編號"
  exit 1
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 創建 Issue #$ISSUE_NUMBER"
echo "[PRINCIPAL] $(date +%H:%M:%S) | URL: $ISSUE_URL"

export ISSUE_NUMBER
```

---

## Step 7: 添加 AWK 追蹤註釋 (Req 5.6, 4.1)

```bash
# 添加 AWK 追蹤註釋
echo "[PRINCIPAL] $(date +%H:%M:%S) | 添加 AWK 追蹤註釋..."

# 使用 github_comment.sh 添加追蹤註釋
if [[ -f ".ai/scripts/github_comment.sh" ]]; then
  source .ai/scripts/github_comment.sh
  add_issue_comment "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "principal" "issue_created" "{}" 2>/dev/null || true
else
  # 備用方案：直接添加註釋
  gh issue comment "$ISSUE_NUMBER" --body "<!-- AWK Tracking -->
**AWK Session**: $PRINCIPAL_SESSION_ID
**Action**: issue_created
**Timestamp**: $(date -u +%Y-%m-%dT%H:%M:%SZ)
" 2>/dev/null || true
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 追蹤註釋已添加"
```

---

## Step 8: 記錄到 Session (Req 5.7, 1.4)

```bash
# 記錄 issue_created action
echo "[PRINCIPAL] $(date +%H:%M:%S) | 記錄到 session..."

bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "issue_created" "{\"issue_id\":\"$ISSUE_NUMBER\",\"title\":\"$TASK_TITLE\",\"spec\":\"$TARGET_SPEC\",\"task\":\"$TASK_NUMBER\"}"

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已記錄到 session"
```

---

## Step 9: 更新 tasks.md (Req 5.8)

```bash
# 在任務行添加 Issue 引用
echo "[PRINCIPAL] $(date +%H:%M:%S) | 更新 tasks.md..."

sed -i "${TASK_LINE_NUM}s/$/ <!-- Issue #$ISSUE_NUMBER -->/" "$TASKS_FILE"

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ tasks.md 已更新"
```

---

## Step 10: 完成

```bash
echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 任務創建完成"
echo ""
echo "Issue #$ISSUE_NUMBER: $TASK_TITLE"
echo "URL: $ISSUE_URL"
echo ""

exit 0
```

---

## 使用範例

### 從 start-work.md 調用

```bash
# 使用環境變數傳入 spec name
SPEC_NAME="command-modularization" source .ai/commands/create-task.md

if [[ "$ESCALATED" == "true" ]]; then
  echo "任務觸發升級，跳過"
elif [[ $? -eq 0 ]] && [[ -n "$ISSUE_NUMBER" ]]; then
  echo "創建成功：Issue #$ISSUE_NUMBER"
fi
```

### 獨立執行

```bash
# 自動選擇第一個有未完成任務的 spec
bash .ai/commands/create-task.md

# 指定 spec
bash .ai/commands/create-task.md command-modularization

# 自動化模式
bash .ai/commands/create-task.md --autonomous
```

---

## 依賴項

- `gh` CLI (GitHub CLI)
- `python3` with `yaml` module
- `.ai/config/workflow.yaml`
- `.ai/scripts/session_manager.sh`
- `.ai/scripts/github_comment.sh`
- `.ai/specs/<spec>/tasks.md`
- `.ai/specs/<spec>/requirements.md` (optional)
- `.ai/specs/<spec>/design.md` (optional)

---

## 錯誤處理

- 如果 PRINCIPAL_SESSION_ID 未設置：嘗試獲取，失敗則退出
- 如果沒有未完成任務：設置 ISSUE_NUMBER 為空並退出
- 如果 gh CLI 失敗：報錯並退出
- 如果觸發 escalation 且為自動化模式：標記任務並設置 ESCALATED=true
- 如果觸發 escalation 且為互動模式：詢問用戶確認
