# Testing Guide

本文件說明 AI Workflow Kit 的測試架構與執行方式。

> **注意：** AWK 主要測試已遷移至 Go。原有的 Python 測試已棄用。

---

## 測試環境設定

### 安裝依賴

```bash
# Go 1.22+ 已包含測試框架
go version

# (選用) 安裝 gotestsum 以獲得更好的輸出
go install gotest.tools/gotestsum@latest
```

---

## 測試架構

### 目錄結構

```
# Go 測試 (主要)
cmd/awkit/
├── main_test.go              # 主程式測試
├── kickoff_test.go           # kickoff 命令測試
├── session_integration_test.go  # Session 整合測試
└── reviewer_integration_test.go # PR 審查整合測試

internal/
├── errors/errors_test.go     # 錯誤處理測試
├── audit/auditor_test.go     # 專案審計測試
├── evaluate/evaluate_test.go # 評估測試
├── generate/
│   ├── generator_test.go     # 生成器測試
│   └── repo_type_test.go     # Repo 類型測試
├── git/
│   ├── branch_test.go        # 分支操作測試
│   ├── operations_test.go    # Git 操作測試
│   └── worktree_test.go      # Worktree 測試
├── install/install_test.go   # 安裝測試
├── kickoff/
│   ├── config_test.go        # 配置測試
│   ├── fanin_test.go         # Fan-in 測試
│   ├── integration_test.go   # 整合測試
│   └── lock_test.go          # 鎖定測試
└── ...

# 測試資料
.ai/tests/fixtures/
├── valid_workflow.yaml       # 有效配置範例
├── invalid_workflow.yaml     # 無效配置範例
└── sample_tasks.md           # 範例任務清單

# 後端測試
backend/health/health_test.go # 後端健康檢查測試
```

---

## 執行測試

### 基本執行

```bash
# 從專案根目錄執行
cd /path/to/ai-workflow-kit

# 執行所有測試
go test ./...

# 執行特定套件測試
go test ./internal/errors/... -v

# 執行特定測試函數
go test ./internal/errors -run TestConfigError -v

# 執行符合模式的測試
go test ./... -run "Test.*Config" -v
```

### 測試輸出選項

```bash
# Verbose 輸出
go test ./... -v

# 使用 gotestsum (更好的輸出格式)
gotestsum ./...

# 短輸出
go test ./... -short
```

### 測試覆蓋率

```bash
# 基本覆蓋率報告
go test ./... -cover

# 詳細覆蓋率 (顯示每個套件)
go test ./... -cover -coverprofile=coverage.out
go tool cover -func=coverage.out

# 產生 HTML 報告
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

---

## 共用 Fixtures

### 定義位置

所有共用 fixtures 定義於 `.ai/tests/conftest.py`。

### 可用 Fixtures

| Fixture | 說明 | 使用場景 |
|---------|------|----------|
| `ai_root` | `.ai` 目錄路徑 | 存取配置和腳本 |
| `project_root` | 專案根目錄 | 存取專案檔案 |
| `scripts_dir` | `.ai/scripts` 目錄 | 存取腳本檔案 |
| `fixtures_dir` | 測試 fixtures 目錄 | 載入測試資料 |
| `temp_dir` | 暫時目錄 (自動清理) | 寫入測試檔案 |
| `temp_git_repo` | 暫時 git repo | 測試 git 相關功能 |
| `mock_repo_structure` | 完整 .ai 結構 | 整合測試 |
| `sample_workflow_yaml` | 範例 workflow 內容 | 配置測試 |

### 使用範例

```python
def test_with_fixtures(ai_root, temp_dir, fixtures_dir):
    """使用多個 fixtures."""
    # ai_root 是 Path 物件指向 .ai 目錄
    assert ai_root.exists()

    # temp_dir 是暫時目錄，測試後自動刪除
    test_file = temp_dir / "test.txt"
    test_file.write_text("hello")
    assert test_file.exists()

    # fixtures_dir 包含測試資料
    sample_file = fixtures_dir / "valid_workflow.yaml"
```

---

## 測試模組說明

### test_errors.py

測試統一錯誤處理框架。

```python
class TestAWKError:
    """AWKError 基礎類別測試."""

    def test_to_dict(self):
        """測試錯誤轉換為字典."""

    def test_default_values(self):
        """測試預設值."""


class TestConfigError:
    """ConfigError 測試."""

    def test_exit_code(self):
        """測試 exit code 為 2."""


class TestValidationError:
    """ValidationError 測試."""

    def test_exit_code(self):
        """測試 exit code 為 3."""


class TestPrintError:
    """print_error 函數測試."""

    def test_json_output(self):
        """測試 JSON 格式輸出."""


class TestHandleUnexpectedError:
    """handle_unexpected_error 函數測試."""

    def test_wraps_exception(self):
        """測試包裝一般例外."""
```

### test_scan_repo.py

測試專案掃描功能。

```python
class TestGetRepoRoot:
    """get_repo_root 函數測試."""

class TestIsClean:
    """is_clean 函數測試."""

class TestGetSubmodulePaths:
    """get_submodule_paths 函數測試."""

class TestScanRepo:
    """scan_repo 函數測試."""
```

### test_parse_tasks.py

測試任務解析功能。

```python
class TestTask:
    """Task 類別測試."""

class TestParseTasks:
    """parse_tasks 函數測試."""

class TestGetExecutableTasks:
    """get_executable_tasks 函數測試."""

class TestTopologicalSort:
    """topological_sort 函數測試."""
```

### test_validate_config.py

測試配置驗證功能。

```python
class TestValidateConfig:
    """validate_config 相關測試."""

    def test_valid_config(self, fixtures_dir):
        """測試有效配置通過驗證."""

    def test_invalid_yaml(self):
        """測試無效 YAML 格式."""

    def test_missing_required_field(self):
        """測試缺少必要欄位."""
```

### test_query_traces.py

測試追蹤查詢功能。

```python
class TestLoadTraces:
    """load_traces 函數測試."""

class TestSummarizeTrace:
    """summarize_trace 函數測試."""

class TestFilterTraces:
    """篩選功能測試."""
```

### test_write_result.py

測試結果寫入功能 (Shell 腳本測試)。

```python
class TestWriteResult:
    """write_result.sh 測試."""

    def test_creates_result_file(self, temp_dir):
        """測試建立結果檔."""

    def test_includes_retry_count(self, temp_dir):
        """測試包含重試次數."""

    def test_json_escape(self):
        """測試特殊字元處理."""
```

---

## 撰寫測試指南

### 測試結構

```python
import pytest
from pathlib import Path

# 導入被測試的模組
import sys
sys.path.insert(0, str(Path(__file__).parent.parent.parent / "scripts"))
from lib.errors import AWKError, ConfigError


class TestFeatureName:
    """功能說明."""

    def test_success_case(self):
        """正常情況測試."""
        # Arrange
        input_data = {...}

        # Act
        result = function(input_data)

        # Assert
        assert result == expected

    def test_edge_case(self):
        """邊界情況測試."""

    def test_error_case(self):
        """錯誤處理測試."""
        with pytest.raises(ValidationError):
            function(invalid_input)
```

### 命名慣例

| 項目 | 格式 | 範例 |
|------|------|------|
| 測試檔案 | `test_<module>.py` | `test_errors.py` |
| 測試類別 | `Test<Feature>` | `TestAWKError` |
| 測試方法 | `test_<behavior>` | `test_to_dict` |

### 使用 Fixtures

```python
@pytest.fixture
def sample_error():
    """建立測試用的錯誤物件."""
    return AWKError(
        message="Test error",
        reason="Testing",
        suggestion="Fix it"
    )


def test_with_custom_fixture(sample_error):
    """使用自訂 fixture."""
    assert sample_error.message == "Test error"
```

### 測試例外

```python
def test_raises_validation_error():
    """測試拋出驗證錯誤."""
    with pytest.raises(ValidationError) as exc_info:
        validate(invalid_data)

    # 檢查錯誤訊息
    assert "required" in str(exc_info.value)
    assert exc_info.value.exit_code == 3
```

### 測試 Shell 腳本

```python
import subprocess

def test_shell_script(temp_dir, scripts_dir):
    """測試 shell 腳本."""
    result = subprocess.run(
        ["bash", str(scripts_dir / "script.sh"), "arg1"],
        capture_output=True,
        text=True,
        cwd=temp_dir
    )

    assert result.returncode == 0
    assert "expected output" in result.stdout
```

### 參數化測試

```python
@pytest.mark.parametrize("input_val,expected", [
    ("debug", "debug"),
    ("DEBUG", "debug"),
    ("info", "info"),
    ("invalid", "info"),  # fallback to default
])
def test_normalize_level(input_val, expected):
    """測試日誌等級正規化."""
    result = normalize_level(input_val)
    assert result == expected
```

---

## 測試資料 (Fixtures)

### 位置

測試資料存放於 `.ai/tests/fixtures/` 目錄。

### 可用資料

```
fixtures/
├── valid_workflow.yaml      # 有效的 workflow 配置
├── invalid_workflow.yaml    # 無效的 workflow 配置
├── sample_tasks.md          # 範例任務清單
├── sample_traces/           # 範例追蹤記錄
│   ├── success.json
│   └── failed.json
└── ...
```

### 新增測試資料

1. 在 `fixtures/` 目錄建立檔案
2. 使用 `fixtures_dir` fixture 存取

```python
def test_with_fixture_file(fixtures_dir):
    """使用 fixture 檔案."""
    config_file = fixtures_dir / "valid_workflow.yaml"
    content = config_file.read_text()
    config = yaml.safe_load(content)
    assert config["version"] == "1.0"
```

---

## CI 整合

### GitHub Actions

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: |
          pip install pytest pytest-cov pyyaml jsonschema jinja2

      - name: Run tests
        run: |
          python -m pytest .ai/tests/unit -v --tb=short

      - name: Coverage report
        run: |
          python -m pytest .ai/tests/unit --cov=.ai/scripts --cov-report=term-missing
```

### 本地 CI 模擬

```bash
# 執行與 CI 相同的命令
python3 -m pytest .ai/tests/unit -v --tb=short
```

---

## 常見問題

### Q: 測試找不到模組？

確保從專案根目錄執行：

```bash
cd /path/to/ai-workflow-kit
python3 -m pytest .ai/tests/unit -v
```

### Q: Fixture 未定義？

確認 `conftest.py` 在正確位置，pytest 會自動載入。

### Q: Windows 上測試失敗？

部分 Shell 腳本測試需要 Git Bash：

```bash
# 使用 Git Bash
"C:\Program Files\Git\bin\bash.exe" -c "python -m pytest .ai/tests/unit -v"
```

### Q: 如何跳過特定測試？

```python
@pytest.mark.skip(reason="Not implemented yet")
def test_future_feature():
    pass


@pytest.mark.skipif(
    sys.platform == "win32",
    reason="Shell script not supported on Windows"
)
def test_shell_script():
    pass
```

---

## 更多資源

- [pytest 文件](https://docs.pytest.org/)
- [貢獻指南](contributing.md) - 測試撰寫規範
- [API 參考](api-reference.md) - 被測試的模組說明
