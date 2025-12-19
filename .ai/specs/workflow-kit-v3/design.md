# AI Workflow Kit v3 - Bug Fixes & Consistency

## Overview

v3 專注於修復 v2 遺留的問題，確保 Kit 在各種環境下都能正確運行。

### 目標
- 修復跨平台可執行性問題
- 統一所有文件的路徑引用
- 確保配置與 repo 現實一致
- 加強測試覆蓋率

---

## P0 - 關鍵修復

### 1. 入口腳本跨平台支援

**問題：**
- `kickoff.sh` 硬呼叫 `bash .ai/scripts/scan_repo.sh`
- 在 Windows 或 CRLF 環境下會失敗
- 已有 `.py` 版本但未被使用

**解決方案：**
創建統一入口函數，自動選擇 `.sh` 或 `.py`：

```bash
# 在 kickoff.sh 中加入
run_script() {
  local script_name="$1"
  shift
  local sh_path="$MONO_ROOT/.ai/scripts/${script_name}.sh"
  local py_path="$MONO_ROOT/.ai/scripts/${script_name}.py"
  
  # 優先使用 Python（跨平台）
  if command -v python3 &>/dev/null && [[ -f "$py_path" ]]; then
    python3 "$py_path" "$@"
  elif [[ -f "$sh_path" ]]; then
    bash "$sh_path" "$@"
  else
    echo "ERROR: Script not found: $script_name" >&2
    return 1
  fi
}

# 使用
run_script scan_repo
run_script audit_project
```

### 2. 路徑引用統一

**問題：**
多個文件仍引用舊路徑 `scripts/ai/...`

**需要更新的文件：**
- `docs/ai-workflow-architecture.md`
- `.ai/commands/dispatch-worker.md`
- `.ai/commands/stop-work.md`
- `.ai/commands/review-pr.md`

**統一路徑格式：**
- 舊：`scripts/ai/xxx.sh` → 新：`.ai/scripts/xxx.sh`
- 舊：`bash scripts/ai/` → 新：`bash .ai/scripts/`

### 3. review-pr.md 硬編碼修復

**問題：**
- 引用 `.ai/rules/git-workflow.md`（應為 `.ai/rules/_kit/git-workflow.md`）
- 硬編碼 `feat/aether`（應從配置讀取）

**解決方案：**
```markdown
# 讀取配置
INTEGRATION_BRANCH=$(python3 -c "import yaml; print(yaml.safe_load(open('.ai/config/workflow.yaml'))['git']['integration_branch'])")

# 使用正確路徑
cat .ai/rules/_kit/git-workflow.md
```

### 4. workflow.yaml 與 repo 現實一致

**問題：**
- 宣告 `type: submodule` 但沒有 `.gitmodules`
- `backend/` 和 `frontend/` 不是獨立 repo

**解決方案：**
將 `type: submodule` 改為 `type: directory`：

```yaml
repos:
  - name: backend
    path: backend/
    type: directory  # 改為 directory
    
  - name: frontend
    path: frontend/
    type: directory  # 改為 directory
```

---

## P1 - 重要修復

### 5. cleanup.sh 分支命名修復

**問題：**
- `run_issue_codex.sh` 創建 `feat/ai-issue-<id>`
- `cleanup.sh` 只清理 `issue-*`

**解決方案：**
更新 cleanup.sh 的分支匹配模式：

```bash
# 遠端分支
REMOTE_BRANCHES=$(git branch -r --list 'origin/feat/ai-issue-*' 2>/dev/null || true)

# 本地分支
LOCAL_BRANCHES=$(git branch --list 'feat/ai-issue-*' 2>/dev/null | sed 's/^[* ]*//' || true)
```

### 6. 測試套件加強

**問題：**
- 沒有真正執行腳本的測試
- CRLF 問題無法被測試發現

**解決方案：**
新增可執行性測試：

```bash
# Test: scan_repo.py 可執行
python3 .ai/scripts/scan_repo.py --json > /dev/null

# Test: audit_project.py 可執行
python3 .ai/scripts/audit_project.py --json > /dev/null

# Test: kickoff.sh --dry-run 可執行（如果有 bash）
if command -v bash &>/dev/null; then
  bash .ai/scripts/kickoff.sh --dry-run > /dev/null 2>&1 || true
fi
```

### 7. validate_config.py 依賴處理

**問題：**
- 自動 `pip3 install` 在受限環境會失敗

**解決方案：**
改為報錯並提示手動安裝：

```python
try:
    import yaml
    import jsonschema
except ImportError as e:
    print(f"[validate] ERROR: Missing dependency: {e.name}")
    print(f"[validate] Please install: pip3 install pyyaml jsonschema")
    sys.exit(1)
```

---

## 測試策略

每個修復都需要：
1. 單元測試驗證
2. 跨平台測試（Windows/macOS/Linux）
3. 文檔更新
