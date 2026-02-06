# Contributing Guide

本文件說明如何為 AI Workflow Kit 貢獻程式碼。

---

## 開發環境設定

### 必要條件

- Go 1.25+
- Git
- GitHub CLI (`gh`)
- Bash (Windows 使用者需要 Git Bash 或 WSL)

### 安裝開發依賴

```bash
# (選用) 安裝 Python 依賴 - 僅供維護 Legacy 腳本時使用
# pip3 install pyyaml jsonschema jinja2

# (已棄用) Python 測試依賴 - 測試已遷移到 Go
# pip3 install pytest pytest-cov
```

### 專案結構

```
.ai/
├── config/           # 配置檔與 Schema
├── templates/        # Jinja2 模板
├── rules/            # 規則檔案
├── tests/            # 測試套件
└── docs/             # 文件 (你正在讀的)
```

---

## 開發流程

### 1. Fork 與 Clone

```bash
# Fork 專案後 clone
git clone https://github.com/<your-username>/ai-workflow-kit.git
cd ai-workflow-kit

# 加入 upstream remote
git remote add upstream https://github.com/silver2dream/ai-workflow-kit.git
```

### 2. 建立分支

```bash
# 從 main 建立功能分支
git checkout main
git pull upstream main
git checkout -b feat/your-feature
```

### 分支命名規範

| 類型 | 格式 | 範例 |
|------|------|------|
| 功能 | `feat/description` | `feat/add-gitlab-support` |
| 修復 | `fix/description` | `fix/yaml-parsing-error` |
| 文件 | `docs/description` | `docs/update-api-reference` |
| 重構 | `refactor/description` | `refactor/error-handling` |

### 3. 開發與測試

```bash
# 執行所有 Go 測試
go test ./...

# 執行特定套件測試
go test ./internal/errors/... -v

# 執行測試覆蓋率
go test ./... -cover

# 驗證配置
awkit validate
```

### 4. 提交變更

```bash
# 遵循 commit 格式
git add .
git commit -m "[feat] add GitLab support"
```

### Commit 格式

```
[type] subject

body (optional)
```

**Type 類型：**

| Type | 說明 |
|------|------|
| `feat` | 新功能 |
| `fix` | 修復 bug |
| `docs` | 文件變更 |
| `refactor` | 重構 (不改變功能) |
| `test` | 測試相關 |
| `chore` | 維護性工作 |

**範例：**
```
[feat] add retry mechanism for failed executions

- Add retry_count and retry_delay_seconds to config
- Implement exponential backoff
- Update write_result.sh to record retry count
```

### 5. 推送與建立 PR

```bash
# 推送分支
git push origin feat/your-feature

# 建立 PR
gh pr create --base main --title "[feat] add GitLab support" --body "..."
```

---

## 程式碼規範

### Python

> **⚠️ DEPRECATED**: 以下 Python 規範僅供參考，AWK 已遷移至 Go。新功能請遵循 Go 規範。

#### 風格指南

- 遵循 PEP 8
- 使用 4 空格縮排
- 行長度最大 100 字元
- 使用 type hints

#### 錯誤處理

使用統一的錯誤框架：

```python
from lib.errors import AWKError, ConfigError, ValidationError, ExecutionError, print_error

# 配置相關錯誤
raise ConfigError(
    message="Config file not found",
    suggestion="Run awkit generate first"
)

# 驗證相關錯誤
raise ValidationError(
    message="Invalid repo type",
    details={"type": "foo", "allowed": ["root", "directory", "submodule"]}
)

# 執行相關錯誤
raise ExecutionError(
    message="Command failed",
    reason="git push failed",
    impact="Changes not uploaded to remote"
)
```

#### 日誌記錄

使用結構化日誌：

```python
from lib.logger import Logger

logger = Logger("my_script", ai_root / "logs", level="info")

# 正確：包含結構化 context
logger.info("Processing file", {"file": "data.txt", "size": 1024})

# 錯誤：使用 f-string
logger.info(f"Processing {filename}")  # 不推薦
```

#### 主程式結構

```python
#!/usr/bin/env python3
"""Script description."""

import sys
from pathlib import Path
from lib.errors import AWKError, print_error, handle_unexpected_error
from lib.logger import Logger, split_log_level

def main(argv: list[str]) -> int:
    """Main entry point."""
    level, remaining_args, err = split_log_level(argv)
    if err:
        print(f"Warning: {err}", file=sys.stderr)

    ai_root = Path(__file__).parent.parent
    logger = Logger("script_name", ai_root / "logs", level=level)

    try:
        # 主要邏輯
        result = do_work()
        print(json.dumps(result))
        return 0

    except AWKError as e:
        print_error(e)
        return e.exit_code

    except Exception as e:
        err = handle_unexpected_error(e)
        print_error(err)
        return err.exit_code

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
```

### Shell Scripts

#### 風格指南

- 使用 `#!/usr/bin/env bash`
- 開頭加入 `set -euo pipefail`
- 使用函數組織程式碼
- 變數使用雙引號包裹

#### 範例結構

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"

# Functions
log_info() {
    echo "[INFO] $*" >&2
}

log_error() {
    echo "[ERROR] $*" >&2
}

main() {
    local arg1="${1:-}"

    if [[ -z "$arg1" ]]; then
        log_error "Missing required argument"
        exit 1
    fi

    # 主要邏輯
}

main "$@"
```

#### JSON 處理

使用 `json_escape` 函數處理特殊字元：

```bash
json_escape() {
    local input
    input="$(cat)"

    if [[ -z "$input" ]]; then
        printf '""'
        return
    fi

    printf '%s' "$input" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()), end="")'
}

# 使用
MESSAGE=$(echo "$raw_message" | json_escape)
```

---

## 測試規範

### 測試檔案結構

```
# Go 測試 (主要測試架構)
cmd/awkit/
├── main_test.go
├── kickoff_test.go
└── *_test.go

internal/
├── errors/errors_test.go
├── audit/auditor_test.go
├── evaluate/evaluate_test.go
├── generate/generator_test.go
├── git/operations_test.go
└── ...

# Shell/Fixture 測試資料
.ai/tests/
└── fixtures/
    ├── valid_workflow.yaml
    ├── invalid_workflow.yaml
    └── sample_tasks.md
```

### 撰寫測試 (Go)

```go
package errors

import (
    "testing"
)

func TestAWKError(t *testing.T) {
    t.Run("creates error with message", func(t *testing.T) {
        err := NewError("test message")
        if err.Error() != "test message" {
            t.Errorf("expected 'test message', got %s", err.Error())
        }
    })

    t.Run("returns correct exit code", func(t *testing.T) {
        err := NewConfigError("config error")
        if err.ExitCode() != 2 {
            t.Errorf("expected exit code 2, got %d", err.ExitCode())
        }
    })
}
```

### 測試命令

```bash
# 執行所有測試
go test ./...

# 帶覆蓋率執行
go test ./... -cover

# 帶 verbose 輸出
go test ./... -v

# 執行特定測試函數
go test ./internal/errors -run TestAWKError -v
```

### 測試覆蓋率要求

- 新增的程式碼應有對應的測試
- 核心套件 (`internal/errors`, `internal/config`) 覆蓋率應 > 70%
- PR 不應降低整體覆蓋率

---

## 文件規範

### 文件類型

| 類型 | 位置 | 對象 |
|------|------|------|
| 使用者文件 | `docs/user/` | Kit 使用者 |
| 開發者文件 | `docs/developer/` | Kit 貢獻者 |
| API 文件 | 程式碼內 docstring | 開發者 |

### Markdown 風格

- 使用 ATX 風格標題 (`#`)
- 程式碼區塊標註語言
- 使用表格呈現結構化資訊
- 中英文之間加空格

### API 文件格式

> **⚠️ DEPRECATED**: 以下 Python docstring 範例僅供參考，AWK 已遷移至 Go。新程式碼請使用 Go doc 註解風格。

```python
def function_name(param1: str, param2: int = 10) -> dict:
    """簡短描述。

    詳細說明 (可選)。

    Args:
        param1: 參數 1 說明
        param2: 參數 2 說明，預設值 10

    Returns:
        回傳值說明

    Raises:
        ValidationError: 驗證失敗時
        ConfigError: 配置錯誤時

    Example:
        >>> result = function_name("test")
        >>> print(result)
        {'status': 'success'}
    """
```

---

## Pull Request 流程

### PR Checklist

提交 PR 前請確認：

- [ ] 程式碼符合風格規範
- [ ] 所有測試通過 (`go test ./...`)
- [ ] 新功能有對應測試
- [ ] 文件已更新 (如適用)
- [ ] Commit 格式正確

> ⚠️ **已棄用**: `pytest .ai/tests/unit -v` 已棄用。請使用 `go test ./...` 執行測試。

### PR 描述模板

```markdown
## Summary

簡述變更內容。

## Changes

- 變更項目 1
- 變更項目 2

## Testing

說明如何測試這些變更。

## Related Issues

Closes #123
```

### Review 流程

1. **自動檢查** - CI 執行測試
2. **Code Review** - 維護者審查
3. **修改** - 根據回饋修改
4. **合併** - 審查通過後 squash merge

---

## 發布流程

### 版本號規則

遵循 Semantic Versioning：

- **MAJOR** - 不相容的 API 變更
- **MINOR** - 向後相容的新功能
- **PATCH** - 向後相容的 bug 修復

### 發布 Checklist

1. 更新 CHANGELOG.md
2. 更新版本號
3. 建立 Release PR
4. 合併後建立 Git tag
5. 發布 GitHub Release

---

## 常見問題

### Q: 測試失敗怎麼辦？

```bash
# 查看詳細輸出 (Go)
go test ./... -v

# 執行特定套件測試
go test ./internal/errors/... -v
```

> ⚠️ **已棄用**: 以下 pytest 命令已棄用，僅供歷史參考：
> ```bash
> # python3 -m pytest .ai/tests/unit -v --tb=long
> # python3 -m pytest .ai/tests/unit/test_errors.py::TestAWKError::test_to_dict -v
> ```

### Q: 如何在本地測試 Shell 腳本？

```bash
# 使用 awkit CLI
awkit kickoff --dry-run

# 啟用 debug 輸出
awkit generate
```

### Q: Windows 上腳本執行失敗？

確保使用 Git Bash 或 WSL，並檢查行尾符號：

```bash
git config --global core.autocrlf input
```

---

## 聯繫方式

- **Issues**: [GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues)
- **Discussions**: [GitHub Discussions](https://github.com/silver2dream/ai-workflow-kit/discussions)

---

## 更多資源

- [架構說明](architecture.md) - 系統內部架構
- [API 參考](api-reference.md) - 函數與模組說明
- [測試說明](testing.md) - 測試架構與執行
