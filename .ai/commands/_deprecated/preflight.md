# Preflight Command

工作流前置檢查，確保環境準備就緒。

**用途：**
- 在 start-work.md 中自動調用
- 可獨立執行：`/preflight` 或 `/preflight --force`（忽略停止標記）

**輸出：**
- 導出 `PRINCIPAL_SESSION_ID` 環境變數
- 導出所有配置變數
- 返回 0 表示成功，非 0 表示失敗

---

## Step 0: 初始化 Principal Session (Req 1.1, 1.2, 1.3)

```bash
# 初始化 Principal Session
# 會檢查是否有其他 Principal 正在運行，如果有則報錯。
# 如果舊 Principal 已死亡，會標記為 interrupted
PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh init_principal_session)
EXIT_CODE=$?

if [[ $EXIT_CODE -ne 0 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ Session 初始化失敗"
  exit 1
fi

export PRINCIPAL_SESSION_ID
echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ Session 已開始：$PRINCIPAL_SESSION_ID"
```

---

## Step 1: 確認 gh CLI 已認證

```bash
# 檢查 gh CLI 是否已認證
if ! gh auth status &>/dev/null; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ gh CLI 未認證"
  echo ""
  echo "請執行以下命令進行認證："
  echo "  gh auth login"
  echo ""
  exit 1
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ gh 已認證"
```

---

## Step 2: 確認工作目錄乾淨

```bash
# 檢查工作目錄是否乾淨
DIRTY_FILES=$(git status --porcelain)

if [[ -n "$DIRTY_FILES" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 工作目錄不乾淨"
  echo ""
  echo "未提交的變更："
  echo "$DIRTY_FILES"
  echo ""
  echo "請先提交或 stash 變更後再啟動工作流。"
  exit 1
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 工作目錄乾淨"
```

---

## Step 3: 確認沒有停止標記

```bash
# 檢查是否有停止標記
if [[ -f ".ai/state/STOP" ]]; then
  # 檢查是否有 --force 參數
  if [[ "$1" == "--force" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 發現停止標記，但使用 --force 忽略"
    rm -f .ai/state/STOP
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 發現停止標記"
    echo ""
    echo "停止標記文件存在：.ai/state/STOP"
    echo "這表示上次工作流被手動停止。"
    echo ""
    echo "如果要繼續執行，請："
    echo "  1. 刪除停止標記：rm .ai/state/STOP"
    echo "  2. 或使用 --force 參數：preflight --force"
    echo ""
    exit 1
  fi
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 無停止標記"
```

---

## Step 4: 讀取配置

```bash
# 檢查配置文件是否存在
if [[ ! -f ".ai/config/workflow.yaml" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 配置文件不存在"
  echo ""
  echo "找不到配置文件：.ai/config/workflow.yaml"
  echo "請確保已正確初始化 AWK 專案。"
  echo ""
  exit 1
fi

# 讀取配置
cat .ai/config/workflow.yaml > /dev/null

if [[ $? -ne 0 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 配置文件讀取失敗"
  exit 1
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 配置已讀取"
```

---

## Step 5: 解析並導出配置值

```bash
# 使用 Python 解析 YAML 配置並導出環境變數
# 這些變數會被 start-work.md 和其他命令使用

# Git 配置
export INTEGRATION_BRANCH=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('git',{}).get('integration_branch', 'main'))" 2>/dev/null || echo "main")
export RELEASE_BRANCH=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('git',{}).get('release_branch', 'main'))" 2>/dev/null || echo "main")
export COMMIT_FORMAT=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('git',{}).get('commit_format', '[type] subject'))" 2>/dev/null || echo "[type] subject")

# Spec 配置
export SPEC_BASE_PATH=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('specs',{}).get('base_path', '.ai/specs'))" 2>/dev/null || echo ".ai/specs")
export ACTIVE_SPECS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(','.join(c.get('specs',{}).get('active', [])))" 2>/dev/null || echo "")
export AUTO_GENERATE_TASKS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(str(c.get('specs',{}).get('auto_generate_tasks', True)).lower())" 2>/dev/null || echo "true")

# Escalation 配置
export MAX_CONSECUTIVE_FAILURES=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('escalation',{}).get('max_consecutive_failures', 3))" 2>/dev/null || echo "3")
export MAX_SINGLE_PR_FILES=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('escalation',{}).get('max_single_pr_files', 50))" 2>/dev/null || echo "50")
export MAX_SINGLE_PR_LINES=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('escalation',{}).get('max_single_pr_lines', 500))" 2>/dev/null || echo "500")
export RETRY_COUNT=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('escalation',{}).get('retry_count', 2))" 2>/dev/null || echo "2")

# Review 配置
export MAX_DIFF_SIZE=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('review',{}).get('max_diff_size_bytes', 100000))" 2>/dev/null || echo "100000")
export WARN_LARGE_DIFF=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(str(c.get('review',{}).get('warn_on_large_diff', True)).lower())" 2>/dev/null || echo "true")
export MAX_REVIEW_CYCLES=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('review',{}).get('max_review_cycles', 3))" 2>/dev/null || echo "3")
export CI_TIMEOUT_SECONDS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('review',{}).get('ci_timeout_seconds', 1800))" 2>/dev/null || echo "1800")
export AUTO_MERGE=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(str(c.get('review',{}).get('auto_merge', True)).lower())" 2>/dev/null || echo "true")

# GitHub 標籤配置
export LABEL_TASK=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('task', 'ai-task'))" 2>/dev/null || echo "ai-task")
export LABEL_IN_PROGRESS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('in_progress', 'in-progress'))" 2>/dev/null || echo "in-progress")
export LABEL_PR_READY=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('pr_ready', 'pr-ready'))" 2>/dev/null || echo "pr-ready")
export LABEL_REVIEW_PASS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('review_pass', 'review-pass'))" 2>/dev/null || echo "review-pass")
export LABEL_REVIEW_FAIL=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('review_fail', 'review-fail'))" 2>/dev/null || echo "review-fail")
export LABEL_WORKER_FAILED=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('worker_failed', 'worker-failed'))" 2>/dev/null || echo "worker-failed")
export LABEL_NEEDS_REVIEW=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('needs_human_review', 'needs-human-review'))" 2>/dev/null || echo "needs-human-review")
export LABEL_SECURITY_REVIEW=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('security_review', 'security-review'))" 2>/dev/null || echo "security-review")
export LABEL_CI_FAILED=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('ci_failed', 'ci-failed'))" 2>/dev/null || echo "ci-failed")
export LABEL_CI_TIMEOUT=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('github',{}).get('labels',{}).get('ci_timeout', 'ci-timeout'))" 2>/dev/null || echo "ci-timeout")

# Escalation Triggers (JSON array)
export ESCALATION_TRIGGERS=$(python3 -c "
import yaml, json
try:
    config = yaml.safe_load(open('.ai/config/workflow.yaml'))
    triggers = config.get('escalation', {}).get('triggers', [])
    print(json.dumps(triggers))
except:
    print('[]')
" 2>/dev/null || echo "[]")

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 配置變數已導出"
```

---

## Step 6: 驗證配置

```bash
# 驗證關鍵配置
echo "[PRINCIPAL] $(date +%H:%M:%S) | 配置摘要："
echo "  - Integration Branch: $INTEGRATION_BRANCH"
echo "  - Release Branch: $RELEASE_BRANCH"
echo "  - Spec Base Path: $SPEC_BASE_PATH"
echo "  - Active Specs: $ACTIVE_SPECS"
echo "  - Max Consecutive Failures: $MAX_CONSECUTIVE_FAILURES"
echo "  - Max Single PR Files: $MAX_SINGLE_PR_FILES"
echo "  - Max Single PR Lines: $MAX_SINGLE_PR_LINES"
echo "  - Max Diff Size: $MAX_DIFF_SIZE bytes"
echo "  - Max Review Cycles: $MAX_REVIEW_CYCLES"
echo "  - CI Timeout: $CI_TIMEOUT_SECONDS seconds"
echo "  - Auto Merge: $AUTO_MERGE"

# 檢查 active specs 是否為空
if [[ -z "$ACTIVE_SPECS" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 沒有啟用的 spec"
  echo ""
  echo "workflow.yaml 中的 specs.active 為空。"
  echo "工作流只會處理現有 pending issues，不會創建新任務。"
  echo ""
fi
```

---

## Step 7: 完成

```bash
echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 前置檢查完成"
echo ""

# 返回成功
exit 0
```

---

## 錯誤處理

如果任何步驟失敗，腳本會：
1. 輸出清晰的錯誤訊息
2. 提供修復建議
3. 返回非 0 退出碼
4. 不會初始化或破壞 Principal Session

---

## 導出的環境變數

### Git 配置
- `INTEGRATION_BRANCH` - PR 目標分支
- `RELEASE_BRANCH` - Release 分支
- `COMMIT_FORMAT` - Commit message 格式

### Spec 配置
- `SPEC_BASE_PATH` - Spec 路徑
- `ACTIVE_SPECS` - 活躍的 spec 列表（逗號分隔）
- `AUTO_GENERATE_TASKS` - 是否自動生成 tasks.md

### Escalation 配置
- `MAX_CONSECUTIVE_FAILURES` - 最大連續失敗次數
- `MAX_SINGLE_PR_FILES` - 單個 PR 最大文件數
- `MAX_SINGLE_PR_LINES` - 單個 PR 最大行數
- `RETRY_COUNT` - 重試次數
- `ESCALATION_TRIGGERS` - 升級觸發條件（JSON array）

### Review 配置
- `MAX_DIFF_SIZE` - 最大 diff 大小（bytes）
- `WARN_LARGE_DIFF` - 是否警告大 diff
- `MAX_REVIEW_CYCLES` - 最大審查次數
- `CI_TIMEOUT_SECONDS` - CI 超時時間
- `AUTO_MERGE` - 是否自動合併

### GitHub 標籤配置
- `LABEL_TASK` - ai-task 標籤
- `LABEL_IN_PROGRESS` - in-progress 標籤
- `LABEL_PR_READY` - pr-ready 標籤
- `LABEL_REVIEW_PASS` - review-pass 標籤
- `LABEL_REVIEW_FAIL` - review-fail 標籤
- `LABEL_WORKER_FAILED` - worker-failed 標籤
- `LABEL_NEEDS_REVIEW` - needs-human-review 標籤
- `LABEL_SECURITY_REVIEW` - security-review 標籤
- `LABEL_CI_FAILED` - ci-failed 標籤
- `LABEL_CI_TIMEOUT` - ci-timeout 標籤

---

## 使用範例

**在 start-work.md 中調用：**
```bash
# 執行前置檢查
source .ai/commands/preflight.md

if [[ $? -ne 0 ]]; then
  echo "前置檢查失敗，停止執行"
  exit 1
fi

# 此時 PRINCIPAL_SESSION_ID 和配置變數已導出
echo "Session ID: $PRINCIPAL_SESSION_ID"
echo "Integration Branch: $INTEGRATION_BRANCH"
```

**獨立執行：**
```bash
# 基本檢查
bash .ai/commands/preflight.md

# 忽略停止標記
bash .ai/commands/preflight.md --force
```
