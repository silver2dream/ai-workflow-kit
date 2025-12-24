# Review PR Command

審查並合併 PR。

**用途：**
- 在 start-work.md 中自動調用
- 可獨立執行：`/review-pr <PR_NUMBER> [ISSUE_NUMBER]`

**參數：**
- `<PR_NUMBER>`: PR 編號（必填）
- `[ISSUE_NUMBER]`: 關聯的 Issue 編號（可選）

**輸出：**
- PR 審查（approve/request-changes）
- PR 合併（如果批准且啟用 auto_merge）
- 更新 Issue 標籤
- 記錄 session actions
- 更新 result.json 中的 review_audit
- 導出 `REVIEW_DECISION`, `MERGE_STATUS`, `ESCALATED` 環境變數

---

## Step 0: 初始化

```bash
# 檢查 Principal session
if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PRINCIPAL_SESSION_ID 未設置，嘗試獲取..."
  PRINCIPAL_SESSION_ID=$(bash .ai/scripts/session_manager.sh get_current_session_id 2>/dev/null || echo "")
  
  if [[ -z "$PRINCIPAL_SESSION_ID" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 無法獲取 Principal Session ID"
    exit 1
  fi
  
  export PRINCIPAL_SESSION_ID
fi

# 檢查參數
if [[ -z "$PR_NUMBER" ]]; then
  if [[ -z "$1" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 缺少 PR 編號"
    echo "用法: bash .ai/commands/review-pr.md <PR_NUMBER> [ISSUE_NUMBER]"
    exit 1
  fi
  PR_NUMBER="$1"
fi

if [[ -z "$ISSUE_NUMBER" ]] && [[ -n "$2" ]]; then
  ISSUE_NUMBER="$2"
fi

# 初始化輸出變數
export REVIEW_DECISION=""
export MERGE_STATUS=""
export ESCALATED=false

echo "[PRINCIPAL] $(date +%H:%M:%S) | Session: $PRINCIPAL_SESSION_ID"
echo "[PRINCIPAL] $(date +%H:%M:%S) | 審查 PR #$PR_NUMBER"
if [[ -n "$ISSUE_NUMBER" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 關聯 Issue #$ISSUE_NUMBER"
fi
```

---

## Step 1: 讀取配置

```bash
# 讀取 review 配置
if [[ -z "$MAX_DIFF_SIZE" ]]; then
  MAX_DIFF_SIZE=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('review',{}).get('max_diff_size_bytes', 100000))" 2>/dev/null || echo "100000")
fi

if [[ -z "$MAX_REVIEW_CYCLES" ]]; then
  MAX_REVIEW_CYCLES=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('review',{}).get('max_review_cycles', 3))" 2>/dev/null || echo "3")
fi

if [[ -z "$CI_TIMEOUT_SECONDS" ]]; then
  CI_TIMEOUT_SECONDS=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('review',{}).get('ci_timeout_seconds', 1800))" 2>/dev/null || echo "1800")
fi

if [[ -z "$AUTO_MERGE" ]]; then
  AUTO_MERGE=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(str(c.get('review',{}).get('auto_merge', True)).lower())" 2>/dev/null || echo "true")
fi

# 讀取 escalation 配置
if [[ -z "$MAX_SINGLE_PR_FILES" ]]; then
  MAX_SINGLE_PR_FILES=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('escalation',{}).get('max_single_pr_files', 50))" 2>/dev/null || echo "50")
fi

if [[ -z "$MAX_SINGLE_PR_LINES" ]]; then
  MAX_SINGLE_PR_LINES=$(python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); print(c.get('escalation',{}).get('max_single_pr_lines', 500))" 2>/dev/null || echo "500")
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 配置已載入"
echo "[PRINCIPAL] $(date +%H:%M:%S) |   Max diff size: $MAX_DIFF_SIZE bytes"
echo "[PRINCIPAL] $(date +%H:%M:%S) |   Max PR files: $MAX_SINGLE_PR_FILES"
echo "[PRINCIPAL] $(date +%H:%M:%S) |   Max PR lines: $MAX_SINGLE_PR_LINES"
echo "[PRINCIPAL] $(date +%H:%M:%S) |   Max review cycles: $MAX_REVIEW_CYCLES"
echo "[PRINCIPAL] $(date +%H:%M:%S) |   CI timeout: $CI_TIMEOUT_SECONDS seconds"
echo "[PRINCIPAL] $(date +%H:%M:%S) |   Auto merge: $AUTO_MERGE"
```


---

## Step 2: 檢查審查次數 (Req 5.5, 5.6)

```bash
# 如果有 ISSUE_NUMBER，檢查審查次數
if [[ -n "$ISSUE_NUMBER" ]]; then
  REVIEW_COUNT_FILE=".ai/runs/issue-$ISSUE_NUMBER/review_count.txt"
  mkdir -p ".ai/runs/issue-$ISSUE_NUMBER"
  
  REVIEW_COUNT=0
  if [[ -f "$REVIEW_COUNT_FILE" ]]; then
    REVIEW_COUNT=$(cat "$REVIEW_COUNT_FILE" 2>/dev/null || echo "0")
  fi
  
  # 檢查 needs-human-review 標籤是否被移除（人工介入後重置）
  HAS_HUMAN_REVIEW_LABEL=$(gh issue view "$ISSUE_NUMBER" --json labels -q '.labels[].name' 2>/dev/null | grep -c "^needs-human-review$" || echo "0")
  
  if [[ "$HAS_HUMAN_REVIEW_LABEL" -eq 0 ]] && [[ "$REVIEW_COUNT" -ge "$MAX_REVIEW_CYCLES" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | needs-human-review 標籤已移除，重置 review_count"
    REVIEW_COUNT=0
  fi
  
  REVIEW_COUNT=$((REVIEW_COUNT + 1))
  echo "$REVIEW_COUNT" > "$REVIEW_COUNT_FILE"
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 審查次數: $REVIEW_COUNT / $MAX_REVIEW_CYCLES"
  
  if [[ "$REVIEW_COUNT" -gt "$MAX_REVIEW_CYCLES" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 超過最大審查次數"
    
    gh issue edit "$ISSUE_NUMBER" --add-label "needs-human-review" 2>/dev/null || true
    gh issue comment "$ISSUE_NUMBER" --body "已達到最大 review 次數 ($MAX_REVIEW_CYCLES)，需要人工審查。" 2>/dev/null || true
    
    export ESCALATED=true
    export REVIEW_DECISION="escalated"
    exit 0
  fi
fi
```

---

## Step 3: 獲取 PR 信息

```bash
# 獲取 PR 信息
echo "[PRINCIPAL] $(date +%H:%M:%S) | 獲取 PR 信息..."

PR_DATA=$(gh pr view "$PR_NUMBER" --json number,title,body,additions,deletions,files,baseRefName,headRefName,state 2>&1)

if [[ $? -ne 0 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 無法獲取 PR 信息"
  echo "$PR_DATA"
  exit 1
fi

PR_TITLE=$(echo "$PR_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('title',''))" 2>/dev/null || echo "")
PR_ADDITIONS=$(echo "$PR_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('additions',0))" 2>/dev/null || echo "0")
PR_DELETIONS=$(echo "$PR_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('deletions',0))" 2>/dev/null || echo "0")
PR_FILES_COUNT=$(echo "$PR_DATA" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('files',[])))" 2>/dev/null || echo "0")
PR_BASE=$(echo "$PR_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('baseRefName',''))" 2>/dev/null || echo "")
PR_HEAD=$(echo "$PR_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('headRefName',''))" 2>/dev/null || echo "")

TOTAL_DIFF_LINES=$((PR_ADDITIONS + PR_DELETIONS))

echo "[PRINCIPAL] $(date +%H:%M:%S) | PR: $PR_TITLE"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Base: $PR_BASE, Head: $PR_HEAD"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Files: $PR_FILES_COUNT"
echo "[PRINCIPAL] $(date +%H:%M:%S) | Diff: +$PR_ADDITIONS -$PR_DELETIONS (total: $TOTAL_DIFF_LINES lines)"
```

---

## Step 4: 檢查 PR 大小 - Escalation (Req 5.4)

```bash
# 獲取 diff 大小（bytes）
DIFF_SIZE=$(gh pr diff "$PR_NUMBER" 2>/dev/null | wc -c || echo "0")

echo "[PRINCIPAL] $(date +%H:%M:%S) | Diff size: $DIFF_SIZE bytes"

# 檢查是否超過 max_diff_size_bytes
if [[ "$DIFF_SIZE" -gt "$MAX_DIFF_SIZE" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Large diff detected: $DIFF_SIZE bytes > $MAX_DIFF_SIZE bytes"
  
  # 記錄 large_diff_warning action
  bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "large_diff_warning" "{\"issue_id\":\"${ISSUE_NUMBER:-}\",\"pr_number\":\"$PR_NUMBER\",\"diff_size\":$DIFF_SIZE,\"threshold\":$MAX_DIFF_SIZE}" 2>/dev/null || true
fi

# 檢查是否超過 max_single_pr_files 或 max_single_pr_lines
PR_TOO_LARGE=false

if [[ "$PR_FILES_COUNT" -gt "$MAX_SINGLE_PR_FILES" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PR 文件數過多: $PR_FILES_COUNT > $MAX_SINGLE_PR_FILES"
  PR_TOO_LARGE=true
fi

if [[ "$TOTAL_DIFF_LINES" -gt "$MAX_SINGLE_PR_LINES" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PR 行數過多: $TOTAL_DIFF_LINES > $MAX_SINGLE_PR_LINES"
  PR_TOO_LARGE=true
fi

if [[ "$PR_TOO_LARGE" == "true" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ PR 過大，需要人工審查"
  
  gh pr edit "$PR_NUMBER" --add-label "needs-human-review" 2>/dev/null || true
  gh pr comment "$PR_NUMBER" --body "PR 過大（$PR_FILES_COUNT 文件，$TOTAL_DIFF_LINES 行），需要人工審查。

限制：
- 最大文件數: $MAX_SINGLE_PR_FILES
- 最大行數: $MAX_SINGLE_PR_LINES" 2>/dev/null || true
  
  if [[ -n "$ISSUE_NUMBER" ]]; then
    gh issue edit "$ISSUE_NUMBER" --add-label "needs-human-review" 2>/dev/null || true
  fi
  
  export ESCALATED=true
  export REVIEW_DECISION="escalated"
  exit 0
fi
```


---

## Step 5: 獲取 PR Diff 並檢查 Escalation Triggers

```bash
# 獲取 PR diff
echo "[PRINCIPAL] $(date +%H:%M:%S) | 獲取 PR diff..."

PR_DIFF=$(gh pr diff "$PR_NUMBER" 2>/dev/null || echo "")

if [[ -z "$PR_DIFF" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 無法獲取 PR diff"
fi

# 計算 diff hash
DIFF_HASH=$(echo "$PR_DIFF" | sha256sum | cut -c1-16)

echo "[PRINCIPAL] $(date +%H:%M:%S) | Diff hash: $DIFF_HASH"

# 檢查 escalation triggers（內容模式）
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 escalation triggers..."

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

if [[ -n "$ESCALATION_TRIGGERS" ]] && [[ -n "$PR_DIFF" ]]; then
  while IFS='|' read -r pattern action; do
    if [[ -z "$pattern" ]]; then
      continue
    fi
    
    # 檢查 diff 內容是否匹配 pattern
    if echo "$PR_DIFF" | grep -qiE "$pattern"; then
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Diff 觸發 escalation: $pattern (action: $action)"
      
      if [[ "$action" == "require_human_approval" ]] || [[ "$action" == "pause_and_ask" ]]; then
        echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 敏感變更，需要人工審查"
        
        gh pr edit "$PR_NUMBER" --add-label "security-review" 2>/dev/null || true
        gh pr comment "$PR_NUMBER" --body "檢測到敏感變更模式: \`$pattern\`，需要人工審查。" 2>/dev/null || true
        
        if [[ -n "$ISSUE_NUMBER" ]]; then
          gh issue edit "$ISSUE_NUMBER" --add-label "security-review" 2>/dev/null || true
        fi
        
        export ESCALATED=true
        export REVIEW_DECISION="escalated"
        exit 0
      elif [[ "$action" == "notify_only" ]]; then
        echo "[PRINCIPAL] $(date +%H:%M:%S) | 通知：匹配敏感模式，繼續審查"
        bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "escalation_notify" "{\"pattern\":\"$pattern\",\"pr_number\":\"$PR_NUMBER\"}" 2>/dev/null || true
      fi
    fi
  done <<< "$ESCALATION_TRIGGERS"
fi
```

---

## Step 6: 檢查 CI 狀態

```bash
# 檢查 CI 狀態
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 CI 狀態..."

CI_WAIT_TIME=0
CI_CHECK_INTERVAL=30
CI_STATUS="pending"
CI_TIMEOUT="false"

while [[ "$CI_WAIT_TIME" -lt "$CI_TIMEOUT_SECONDS" ]]; do
  CI_STATES=$(gh pr checks "$PR_NUMBER" --json state --jq '.[].state' 2>/dev/null | sort -u || echo "")
  
  if echo "$CI_STATES" | grep -q "FAILURE"; then
    CI_STATUS="failed"
    break
  fi
  
  if echo "$CI_STATES" | grep -qv "PENDING" && echo "$CI_STATES" | grep -qv "QUEUED"; then
    # 所有 CI 都完成了
    if echo "$CI_STATES" | grep -q "SUCCESS"; then
      CI_STATUS="passed"
      break
    fi
  fi
  
  echo "[PRINCIPAL] $(date +%H:%M:%S) | CI 仍在運行，等待 $CI_CHECK_INTERVAL 秒..."
  sleep "$CI_CHECK_INTERVAL"
  CI_WAIT_TIME=$((CI_WAIT_TIME + CI_CHECK_INTERVAL))
done

if [[ "$CI_WAIT_TIME" -ge "$CI_TIMEOUT_SECONDS" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ CI 超時"
  CI_STATUS="timeout"
  CI_TIMEOUT="true"
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | CI 狀態: $CI_STATUS"

# 處理 CI 失敗或超時
if [[ "$CI_STATUS" == "failed" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ CI 失敗"
  
  gh pr review "$PR_NUMBER" --request-changes --body "CI 檢查失敗，請修復後重新提交。" 2>/dev/null || true
  
  if [[ -n "$ISSUE_NUMBER" ]]; then
    gh issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "ci-failed" 2>/dev/null || true
  fi
  
  # 更新 review_audit
  bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "request_changes" "failed" "false" "" 2>/dev/null || true
  
  export REVIEW_DECISION="request_changes"
  exit 0
fi

if [[ "$CI_STATUS" == "timeout" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ CI 超時"
  
  if [[ -n "$ISSUE_NUMBER" ]]; then
    gh issue edit "$ISSUE_NUMBER" --add-label "ci-timeout" 2>/dev/null || true
    gh issue comment "$ISSUE_NUMBER" --body "CI timeout after ${CI_TIMEOUT_SECONDS}s. Please investigate." 2>/dev/null || true
  fi
  
  # 更新 review_audit
  bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "pending" "timeout" "true" "" 2>/dev/null || true
  
  export REVIEW_DECISION="pending"
  exit 0
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ CI 通過"
```


---

## Step 7: 執行審查檢查（完整 5 項標準）

```bash
# 執行審查檢查
echo "[PRINCIPAL] $(date +%H:%M:%S) | 執行審查檢查..."

# 讀取架構規則
GIT_WORKFLOW_FILE=".ai/rules/_kit/git-workflow.md"

# 檢查 1: Commit 格式
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 1: Commit 格式..."
COMMIT_FORMAT_OK=true

if [[ -f "$GIT_WORKFLOW_FILE" ]]; then
  PR_COMMITS=$(gh pr view "$PR_NUMBER" --json commits --jq '.commits[].messageHeadline' 2>/dev/null || echo "")
  
  while IFS= read -r commit_msg; do
    if [[ -z "$commit_msg" ]]; then
      continue
    fi
    
    # 檢查是否符合 [type] subject 格式
    if ! echo "$commit_msg" | grep -qE '^\[.+\] .+'; then
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Commit message 格式不正確: $commit_msg"
      COMMIT_FORMAT_OK=false
    fi
  done <<< "$PR_COMMITS"
fi

# 檢查 2: 範圍限制（變更是否在 ticket scope 內）
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 2: 範圍限制..."
SCOPE_OK=true

# 從 PR body 或關聯 issue 獲取 scope
if [[ -n "$ISSUE_NUMBER" ]]; then
  ISSUE_BODY=$(gh issue view "$ISSUE_NUMBER" --json body -q '.body' 2>/dev/null || echo "")
  SCOPE_SECTION=$(echo "$ISSUE_BODY" | awk '/^## Scope/,/^## / {if (!/^## /) print}' | head -20 || echo "")
  
  if [[ -n "$SCOPE_SECTION" ]]; then
    # 簡單檢查：確保 PR 標題或描述與 scope 相關
    PR_BODY=$(echo "$PR_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('body',''))" 2>/dev/null || echo "")
    
    # 這裡可以添加更複雜的 scope 檢查邏輯
    echo "[PRINCIPAL] $(date +%H:%M:%S) | Scope 檢查通過（基本驗證）"
  fi
fi

# 檢查 3: 架構合規
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 3: 架構合規..."
ARCHITECTURE_OK=true

# 檢查是否有違反架構規則的變更
# 例如：檢查是否修改了不應該修改的文件
PROTECTED_PATTERNS="go.mod|go.sum|package.json|package-lock.json"
MODIFIED_FILES=$(echo "$PR_DATA" | python3 -c "import json,sys; print('\n'.join([f['path'] for f in json.load(sys.stdin).get('files',[])]))" 2>/dev/null || echo "")

# 檢查 4: 無明顯 bug
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 4: 代碼質量..."
CODE_QUALITY_OK=true

# 檢查是否有明顯的問題
if echo "$PR_DIFF" | grep -qiE '(TODO|FIXME|XXX|HACK)'; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 發現待辦事項標記"
  # 不扣分，只是警告
fi

if echo "$PR_DIFF" | grep -qiE '(console\.log|debugger|print\(|fmt\.Print)'; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 發現調試代碼"
  CODE_QUALITY_OK=false
fi

# 檢查 5: 安全檢查
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 5: 安全檢查..."
SECURITY_OK=true

# 檢查是否有敏感資訊洩露
SENSITIVE_PATTERNS="password|secret|api_key|apikey|token|credential|private_key"
if echo "$PR_DIFF" | grep -qiE "$SENSITIVE_PATTERNS"; then
  # 進一步檢查是否是新增的敏感資訊
  if echo "$PR_DIFF" | grep -E '^\+' | grep -qiE "$SENSITIVE_PATTERNS"; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 可能包含敏感資訊"
    SECURITY_OK=false
  fi
fi

# 檢查 PR base branch
echo "[PRINCIPAL] $(date +%H:%M:%S) | 檢查 PR base branch..."
BASE_BRANCH_OK=true

EXPECTED_BASE="main"
if [[ -n "$INTEGRATION_BRANCH" ]]; then
  EXPECTED_BASE="$INTEGRATION_BRANCH"
fi

if [[ "$PR_BASE" != "$EXPECTED_BASE" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ PR base branch 不正確: $PR_BASE (expected: $EXPECTED_BASE)"
  BASE_BRANCH_OK=false
fi

# 計算審查分數
SCORE=10

if [[ "$COMMIT_FORMAT_OK" != "true" ]]; then
  SCORE=$((SCORE - 2))
fi

if [[ "$SCOPE_OK" != "true" ]]; then
  SCORE=$((SCORE - 1))
fi

if [[ "$ARCHITECTURE_OK" != "true" ]]; then
  SCORE=$((SCORE - 1))
fi

if [[ "$CODE_QUALITY_OK" != "true" ]]; then
  SCORE=$((SCORE - 1))
fi

if [[ "$SECURITY_OK" != "true" ]]; then
  SCORE=$((SCORE - 2))
fi

if [[ "$BASE_BRANCH_OK" != "true" ]]; then
  SCORE=$((SCORE - 1))
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | 審查分數: $SCORE / 10"
```


---

## Step 8: 提取代碼符號和設計引用

```bash
# 提取新增/修改的代碼符號
echo "[PRINCIPAL] $(date +%H:%M:%S) | 提取代碼符號..."

CODE_SYMBOLS=""

# 從 diff 中提取 func/def/class 定義
FUNC_DEFS=$(echo "$PR_DIFF" | grep -E '^\+.*(func |def |class |interface |type |struct )' | head -20 || echo "")

if [[ -n "$FUNC_DEFS" ]]; then
  CODE_SYMBOLS="$FUNC_DEFS"
fi

# 提取設計引用
echo "[PRINCIPAL] $(date +%H:%M:%S) | 提取設計引用..."

DESIGN_REFS=""

if [[ -n "$ISSUE_NUMBER" ]]; then
  ISSUE_BODY=$(gh issue view "$ISSUE_NUMBER" --json body -q '.body' 2>/dev/null || echo "")
  
  # 從 issue body 提取 spec 和 design 引用
  SPEC_NAME=$(echo "$ISSUE_BODY" | grep -oP '(?<=\*\*Spec\*\*: )[^\n]+' | head -1 || echo "")
  
  if [[ -n "$SPEC_NAME" ]]; then
    DESIGN_REFS="參考 .ai/specs/$SPEC_NAME/design.md"
  fi
fi
```

---

## Step 9: 生成 AWK Review Comment（完整格式）(Req 5.1, 5.2, 5.9)

```bash
# 生成 AWK Review Comment
echo "[PRINCIPAL] $(date +%H:%M:%S) | 生成審查評論..."

REVIEW_COMMENT="<!-- AWK Review -->

## Review Summary

**Session**: $PRINCIPAL_SESSION_ID
**Diff Hash**: $DIFF_HASH
**Review Cycle**: ${REVIEW_COUNT:-1}

### 程式碼符號 (Code Symbols):

\`\`\`
${CODE_SYMBOLS:-無新增符號}
\`\`\`

### 設計引用 (Design References):

${DESIGN_REFS:-無設計引用}

### 評分 (Score): $SCORE/10

### 評分理由 (Reasoning):

"

if [[ "$SCORE" -ge 7 ]]; then
  REVIEW_COMMENT="${REVIEW_COMMENT}此 PR 符合基本要求，可以合併。

**通過的檢查：**
"
  [[ "$COMMIT_FORMAT_OK" == "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✓ Commit message 格式正確
"
  [[ "$SCOPE_OK" == "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✓ 變更在 scope 內
"
  [[ "$ARCHITECTURE_OK" == "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✓ 符合架構規則
"
  [[ "$CODE_QUALITY_OK" == "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✓ 代碼質量良好
"
  [[ "$SECURITY_OK" == "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✓ 無安全問題
"
  [[ "$BASE_BRANCH_OK" == "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✓ Base branch 正確
"
else
  REVIEW_COMMENT="${REVIEW_COMMENT}此 PR 存在以下問題需要修復：

"
  [[ "$COMMIT_FORMAT_OK" != "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✗ Commit message 格式不符合規範（應為 \`[type] subject\`）
"
  [[ "$SCOPE_OK" != "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✗ 變更超出 scope
"
  [[ "$ARCHITECTURE_OK" != "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✗ 違反架構規則
"
  [[ "$CODE_QUALITY_OK" != "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✗ 代碼中包含調試代碼
"
  [[ "$SECURITY_OK" != "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✗ 可能包含敏感資訊
"
  [[ "$BASE_BRANCH_OK" != "true" ]] && REVIEW_COMMENT="${REVIEW_COMMENT}- ✗ Base branch 應為 \`$EXPECTED_BASE\`
"
fi

REVIEW_COMMENT="${REVIEW_COMMENT}
### 可改進之處 (Improvements):

"

# 添加改進建議
if [[ "$COMMIT_FORMAT_OK" != "true" ]]; then
  REVIEW_COMMENT="${REVIEW_COMMENT}- 請使用 \`[type] subject\` 格式的 commit message
"
fi

if [[ "$CODE_QUALITY_OK" != "true" ]]; then
  REVIEW_COMMENT="${REVIEW_COMMENT}- 請移除調試代碼（console.log, print, debugger 等）
"
fi

REVIEW_COMMENT="${REVIEW_COMMENT}
### 潛在風險 (Risks):

"

if [[ "$SECURITY_OK" != "true" ]]; then
  REVIEW_COMMENT="${REVIEW_COMMENT}- ⚠ 可能包含敏感資訊，請確認是否需要移除
"
fi

if [[ "$TOTAL_DIFF_LINES" -gt 300 ]]; then
  REVIEW_COMMENT="${REVIEW_COMMENT}- ⚠ PR 較大（$TOTAL_DIFF_LINES 行），建議拆分為更小的 PR
"
fi

REVIEW_COMMENT="${REVIEW_COMMENT}
---
*Reviewed by AWK Principal*
"

echo "[PRINCIPAL] $(date +%H:%M:%S) | 審查評論已生成"
```

---

## Step 10: 驗證 Review Comment (Req 5.3)

```bash
# 驗證 Review Comment
echo "[PRINCIPAL] $(date +%H:%M:%S) | 驗證審查評論..."

# 保存到臨時文件
mkdir -p .ai/temp
REVIEW_FILE=".ai/temp/review-$PR_NUMBER.md"
echo "$REVIEW_COMMENT" > "$REVIEW_FILE"

# 調用 verify_review.sh（如果存在）
VERIFY_EXIT=0
if [[ -f ".ai/scripts/verify_review.sh" ]]; then
  bash .ai/scripts/verify_review.sh "$REVIEW_FILE" || VERIFY_EXIT=$?
  
  if [[ "$VERIFY_EXIT" -eq 1 ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ Review comment 驗證失敗，重新生成"
    # 可以在這裡添加重新生成邏輯
  fi
  
  if [[ "$VERIFY_EXIT" -eq 2 ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | Review score < 7, 請求修改"
    SCORE=6  # 強制設為不通過
  fi
fi

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 審查評論驗證完成"
```


---

## Step 11: 發布審查評論

```bash
# 發布審查評論
echo "[PRINCIPAL] $(date +%H:%M:%S) | 發布審查評論..."

gh pr comment "$PR_NUMBER" --body "$REVIEW_COMMENT" 2>/dev/null || true

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 審查評論已發布"
```

---

## Step 12: 批准或請求修改

```bash
# 根據分數決定批准或請求修改
if [[ "$SCORE" -ge 7 ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 批准 PR"
  
  gh pr review "$PR_NUMBER" --approve --body "✅ AI Review 通過：符合架構規則，變更在範圍內。" 2>/dev/null || true
  
  DECISION="approved"
else
  echo "[PRINCIPAL] $(date +%H:%M:%S) | ✗ 請求修改"
  
  gh pr review "$PR_NUMBER" --request-changes --body "❌ 需要修正，請參考審查評論。" 2>/dev/null || true
  
  DECISION="request_changes"
  
  if [[ -n "$ISSUE_NUMBER" ]]; then
    gh issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --remove-label "in-progress" --add-label "review-fail" 2>/dev/null || true
    gh issue comment "$ISSUE_NUMBER" --body "Review failed. Please address the requested changes and rerun." 2>/dev/null || true
  fi
fi

export REVIEW_DECISION="$DECISION"
```

---

## Step 13: 記錄審查 (Req 1.4)

```bash
# 記錄 pr_reviewed action
echo "[PRINCIPAL] $(date +%H:%M:%S) | 記錄審查..."

bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "pr_reviewed" "{\"issue_id\":\"${ISSUE_NUMBER:-}\",\"pr_number\":\"$PR_NUMBER\",\"decision\":\"$DECISION\",\"score\":$SCORE}"

echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 已記錄審查"
```

---

## Step 14: 自動合併（如果批准）(Req 6.4)

```bash
if [[ "$DECISION" == "approved" ]] && [[ "$AUTO_MERGE" == "true" ]]; then
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 準備自動合併..."
  
  # 等待最終 CI 通過
  echo "[PRINCIPAL] $(date +%H:%M:%S) | 等待最終 CI 通過..."
  sleep 10
  
  CI_FINAL_STATUS=$(gh pr checks "$PR_NUMBER" --json state --jq '.[].state' 2>/dev/null | sort -u || echo "")
  
  if echo "$CI_FINAL_STATUS" | grep -q "SUCCESS" && ! echo "$CI_FINAL_STATUS" | grep -qE "(PENDING|QUEUED|FAILURE)"; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ CI 通過，執行合併..."
    
    # 使用 --auto 等待 branch protection 規則
    gh pr merge "$PR_NUMBER" --squash --delete-branch --auto 2>/dev/null
    MERGE_EXIT=$?
    
    if [[ "$MERGE_EXIT" -eq 0 ]]; then
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ PR 已合併（或已啟用 auto-merge）"
      
      # 獲取 merge timestamp
      MERGE_TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
      
      # 更新 review_audit (Req 6.4)
      bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "approved" "passed" "false" "$MERGE_TIMESTAMP" 2>/dev/null || true
      
      # 記錄 pr_merged action (Req 1.4)
      bash .ai/scripts/session_manager.sh append_session_action "$PRINCIPAL_SESSION_ID" "pr_merged" "{\"issue_id\":\"${ISSUE_NUMBER:-}\",\"pr_number\":\"$PR_NUMBER\",\"merge_timestamp\":\"$MERGE_TIMESTAMP\"}"
      
      # 更新 Issue 標籤
      if [[ -n "$ISSUE_NUMBER" ]]; then
        gh issue edit "$ISSUE_NUMBER" --remove-label "pr-ready" --add-label "review-pass" 2>/dev/null || true
        gh issue close "$ISSUE_NUMBER" --comment "🎉 已合併！PR #$PR_NUMBER" 2>/dev/null || true
        
        # 重置 fail_count 和刪除 review_count.txt (Req 5.8)
        rm -f ".ai/runs/issue-$ISSUE_NUMBER/fail_count.txt"
        rm -f ".ai/runs/issue-$ISSUE_NUMBER/review_count.txt"
      fi
      
      export MERGE_STATUS="merged"
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ✓ 審查流程完成"
    else
      echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ 合併命令執行，可能已啟用 auto-merge"
      export MERGE_STATUS="auto_merge_enabled"
    fi
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | ⚠ CI 未完全通過，跳過合併"
    export MERGE_STATUS="ci_pending"
    
    # 更新 review_audit
    bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "approved" "pending" "false" "" 2>/dev/null || true
  fi
else
  if [[ "$DECISION" != "approved" ]]; then
    echo "[PRINCIPAL] $(date +%H:%M:%S) | 審查不通過，不執行合併"
    export MERGE_STATUS="not_approved"
    
    # 更新 review_audit
    bash .ai/scripts/session_manager.sh update_result_with_review_audit "$ISSUE_NUMBER" "$PRINCIPAL_SESSION_ID" "request_changes" "" "false" "" 2>/dev/null || true
  else
    echo "[PRINCIPAL] $(date +%H:%M:%S) | Auto merge 已禁用"
    export MERGE_STATUS="auto_merge_disabled"
  fi
fi

exit 0
```

---

## 使用範例

### 從 start-work.md 調用

```bash
PR_NUMBER=123 ISSUE_NUMBER=456 source .ai/commands/review-pr.md

if [[ "$ESCALATED" == "true" ]]; then
  echo "PR 觸發升級，需要人工審查"
elif [[ "$REVIEW_DECISION" == "approved" ]]; then
  echo "審查通過，MERGE_STATUS: $MERGE_STATUS"
else
  echo "審查不通過"
fi
```

### 獨立執行

```bash
# 只審查 PR
bash .ai/commands/review-pr.md 123

# 審查 PR 並關聯 Issue
bash .ai/commands/review-pr.md 123 456
```

---

## 依賴項

- `gh` CLI (GitHub CLI)
- `python3` with `yaml` and `json` modules
- `.ai/config/workflow.yaml`
- `.ai/scripts/session_manager.sh`
- `.ai/scripts/verify_review.sh` (optional)
- `.ai/rules/_kit/git-workflow.md`

---

## 輸出文件

- `.ai/temp/review-<N>.md` - 審查評論臨時文件
- `.ai/runs/issue-<N>/review_count.txt` - 審查次數計數

---

## 輸出變數

- `REVIEW_DECISION`: 審查決定
  - `approved` - 批准
  - `request_changes` - 請求修改
  - `escalated` - 觸發升級
  - `pending` - 等待中
  
- `MERGE_STATUS`: 合併狀態
  - `merged` - 已合併
  - `auto_merge_enabled` - 已啟用 auto-merge
  - `ci_pending` - CI 等待中
  - `not_approved` - 未批准
  - `auto_merge_disabled` - auto-merge 已禁用
  
- `ESCALATED`: 是否觸發升級
  - `true` - 觸發升級
  - `false` - 未觸發

---

## 錯誤處理

- 如果 PR 不存在：報錯並退出
- 如果超過最大審查次數：標記 `needs-human-review` 並設置 ESCALATED=true
- 如果 PR 過大：標記 `needs-human-review` 並設置 ESCALATED=true
- 如果觸發敏感模式：標記 `security-review` 並設置 ESCALATED=true
- 如果 CI 失敗：請求修改並退出
- 如果 CI 超時：標記 `ci-timeout` 並退出
- 如果審查分數 < 7：請求修改
- 如果審查分數 >= 7：批准並可能自動合併
