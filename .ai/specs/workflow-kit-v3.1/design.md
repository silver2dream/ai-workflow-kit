# AI Workflow Kit v3.1 - Evaluation Consistency Fix

## Overview

v3.1 修復 evaluate.md 評估框架與實際實作之間的不一致問題。

### 目標
- 確保 `.py` 腳本與 `.sh` 行為一致（落盤）
- 創建 evaluate.sh 實作評估腳本
- 統一 Online Gate 檢查項目
- 加強配置一致性驗證

---

## P0 - 關鍵修復

### 1. Python 腳本落盤

**問題：**
- `scan_repo.py` 和 `audit_project.py` 只輸出到 stdout
- 不會產生 `.ai/state/repo_scan.json` 和 `.ai/state/audit.json`
- 導致 Offline Gate O2/O4 在 Python fallback 時會 FAIL

**解決方案：**
讓 `.py` 腳本也寫入 `.ai/state/*.json`：

```python
# scan_repo.py
def main():
    output_json = '--json' in sys.argv
    root = get_repo_root()
    result = scan_repo(root)
    
    # 總是寫入 state 文件
    state_dir = root / '.ai' / 'state'
    state_dir.mkdir(parents=True, exist_ok=True)
    with open(state_dir / 'repo_scan.json', 'w', encoding='utf-8') as f:
        json.dump(result, f, indent=2)
    
    # 輸出到 stdout
    if output_json:
        print(json.dumps(result, indent=2))
    else:
        # human-readable output
        ...
```

### 2. 創建 evaluate.sh

**問題：**
- evaluate.md 說要跑 `bash .ai/scripts/evaluate.sh`
- 但這個檔案不存在

**解決方案：**
創建 `.ai/scripts/evaluate.sh`，實作 evaluate.md 裡的完整評估腳本。

---

## P1 - 重要修復

### 3. 統一 Online Gate

**問題：**
- 表格有 N2 (rollback --dry-run)
- 但完整腳本只檢查 kickoff 和 stats

**解決方案：**
在 evaluate.sh 和 evaluate.md 都加入 rollback 檢查：

```bash
check_online "N2" "bash .ai/scripts/rollback.sh 99999 --dry-run 2>&1 | grep -qiE 'not found|usage|dry'" "rollback"
```

### 4. 加強配置一致性檢查 (R2)

**問題：**
- 目前只驗證 `repos[].path` 存在
- 無法抓出 `type: submodule` 但實際不是 submodule 的情況

**解決方案：**
在 `validate_config.py` 加入 type-specific 驗證：

```python
def validate_repo_type(repo: dict, mono_root: Path) -> list:
    errors = []
    repo_type = repo.get('type', 'directory')
    repo_path = repo.get('path', '')
    full_path = mono_root / repo_path
    
    if repo_type == 'submodule':
        # 檢查 .gitmodules 存在且包含該 path
        gitmodules = mono_root / '.gitmodules'
        if not gitmodules.exists():
            errors.append(f"type=submodule but .gitmodules not found")
        else:
            content = gitmodules.read_text()
            if repo_path.rstrip('/') not in content:
                errors.append(f"type=submodule but path not in .gitmodules")
        # 檢查該 path 下是 git repo
        if not (full_path / '.git').exists():
            errors.append(f"type=submodule but {repo_path} is not a git repo")
    
    elif repo_type == 'directory':
        # 檢查是目錄
        if not full_path.is_dir():
            errors.append(f"type=directory but {repo_path} is not a directory")
        # 不應該是獨立 git repo（有 .git 目錄）
        if (full_path / '.git').exists():
            errors.append(f"type=directory but {repo_path} has .git (should be submodule?)")
    
    elif repo_type == 'root':
        # 檢查 path 是 ./ 或空
        if repo_path not in ['./', '.', '']:
            errors.append(f"type=root but path is not './' (got: {repo_path})")
    
    return errors
```

---

## P2 - 文檔改進

### 5. 明確前置條件

**問題：**
- Offline Gate 假設 Python 依賴已安裝
- 但沒有明確列出

**解決方案：**
在 evaluate.md 加入前置條件章節：

```markdown
## 前置條件

### Offline Gate 最低需求
- Python 3.8+
- pip packages: `pyyaml`, `jsonschema`, `jinja2`
- bash (Git Bash on Windows)

### 安裝依賴
\`\`\`bash
pip3 install pyyaml jsonschema jinja2
\`\`\`
```

---

## 測試策略

1. 執行 `python3 .ai/scripts/scan_repo.py` 後檢查 `.ai/state/repo_scan.json` 存在
2. 執行 `python3 .ai/scripts/audit_project.py` 後檢查 `.ai/state/audit.json` 存在
3. 執行 `bash .ai/scripts/evaluate.sh --offline` 確認 Offline Gate 全過
4. 測試 `type: submodule` 配置在沒有 `.gitmodules` 時會報錯
