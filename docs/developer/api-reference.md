# API Reference

本文件說明 AI Workflow Kit 的 Python 腳本和模組 API。

---

## lib/errors.py

統一錯誤處理框架。

### Constants

```python
EXIT_SUCCESS = 0          # 執行成功
EXIT_ERROR = 1            # 一般錯誤
EXIT_CONFIG_ERROR = 2     # 配置錯誤
EXIT_VALIDATION_ERROR = 3 # 驗證錯誤
```

### Classes

#### AWKError

基礎錯誤類別。

```python
@dataclass
class AWKError(Exception):
    message: str                    # 錯誤訊息
    error_type: str = "execution_error"
    exit_code: int = EXIT_ERROR
    reason: Optional[str] = None    # 錯誤原因
    impact: Optional[str] = None    # 影響範圍
    suggestion: Optional[str] = None # 建議解決方案
    details: Dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> Dict[str, Any]:
        """轉換為字典格式"""
```

#### ConfigError

配置相關錯誤，exit_code = 2。

```python
class ConfigError(AWKError):
    def __init__(self, message: str, **kwargs) -> None
```

#### ValidationError

驗證相關錯誤，exit_code = 3。

```python
class ValidationError(AWKError):
    def __init__(self, message: str, **kwargs) -> None
```

#### ExecutionError

執行相關錯誤，exit_code = 1。

```python
class ExecutionError(AWKError):
    def __init__(self, message: str, **kwargs) -> None
```

### Functions

#### print_error

輸出錯誤到 stderr (JSON 格式)。

```python
def print_error(err: AWKError, stream: Any = None) -> None
```

**參數：**
- `err` - AWKError 實例
- `stream` - 輸出串流，預設為 sys.stderr

#### handle_unexpected_error

將一般 Exception 轉換為 AWKError。

```python
def handle_unexpected_error(exc: Exception) -> AWKError
```

**範例：**
```python
try:
    do_something()
except AWKError as err:
    print_error(err)
    sys.exit(err.exit_code)
except Exception as exc:
    err = handle_unexpected_error(exc)
    print_error(err)
    sys.exit(err.exit_code)
```

---

## lib/logger.py

結構化 JSON 日誌系統。

### Constants

```python
LEVELS = {
    "debug": 10,
    "info": 20,
    "warn": 30,
    "error": 40,
}
```

### Functions

#### normalize_level

正規化日誌等級。

```python
def normalize_level(level: Optional[str], default: str = "info") -> str
```

**參數：**
- `level` - 日誌等級字串
- `default` - 預設等級

**回傳：** 正規化後的等級字串

#### split_log_level

從命令列參數提取 --log-level。

```python
def split_log_level(argv: List[str], default: str = "info") -> Tuple[str, List[str], Optional[str]]
```

**參數：**
- `argv` - 命令列參數列表
- `default` - 預設等級

**回傳：** (level, remaining_args, error)

### Classes

#### Logger

結構化日誌記錄器。

```python
class Logger:
    def __init__(self, source: str, log_dir: Path, level: str = "info") -> None
```

**參數：**
- `source` - 日誌來源名稱 (通常是腳本名稱)
- `log_dir` - 日誌目錄
- `level` - 最低記錄等級

**方法：**

```python
def debug(self, message: str, context: Optional[Dict[str, Any]] = None) -> None
def info(self, message: str, context: Optional[Dict[str, Any]] = None) -> None
def warn(self, message: str, context: Optional[Dict[str, Any]] = None) -> None
def error(self, message: str, context: Optional[Dict[str, Any]] = None) -> None
```

**範例：**
```python
from lib.logger import Logger

logger = Logger("my_script", Path(".ai/logs"), level="debug")
logger.info("Processing started", {"file": "data.txt"})
logger.error("Failed to process", {"error": str(e)})
```

---

## scan_repo.py

掃描專案結構。

### CLI

```bash
python3 .ai/scripts/scan_repo.py [--json] [--log-level LEVEL]
```

**參數：**
- `--json` - 輸出 JSON 格式到 stdout
- `--log-level` - 日誌等級 (debug/info/warn/error)

**輸出：** 寫入 `.ai/state/repo_scan.json`

### Functions

#### get_repo_root

取得 Git 專案根目錄。

```python
def get_repo_root() -> Path
```

#### is_clean

檢查工作目錄是否乾淨。

```python
def is_clean(cwd: Path) -> bool
```

#### get_submodule_paths

解析 .gitmodules 取得 submodule 路徑。

```python
def get_submodule_paths(root: Path) -> List[str]
```

#### scan_repo

執行完整的專案掃描。

```python
def scan_repo(root: Path) -> dict
```

**回傳值 Schema：** 參考 `repo_scan.schema.json`

---

## audit_project.py

審計專案狀態。

### CLI

```bash
python3 .ai/scripts/audit_project.py [--json] [--log-level LEVEL]
```

**輸出：** 寫入 `.ai/state/audit.json`

### Functions

#### audit_project

執行專案審計。

```python
def audit_project(root: Path) -> dict
```

**回傳值包含：**
- `findings` - 發現的問題列表
- `summary` - 統計摘要 (P0/P1/P2 數量)

---

## parse_tasks.py

解析 tasks.md 任務清單。

### CLI

```bash
python3 .ai/scripts/parse_tasks.py <tasks_file> [--json] [--next] [--parallel]
```

**參數：**
- `tasks_file` - tasks.md 檔案路徑
- `--json` - 輸出 JSON 格式
- `--next` - 顯示下一個可執行的任務
- `--parallel` - 顯示可並行執行的任務群組

### Classes

#### Task

任務資料類別。

```python
class Task:
    id: str                    # 任務 ID
    title: str                 # 任務標題
    completed: bool            # 是否已完成
    depends_on: List[str]      # 依賴的任務 ID
    subtasks: List[Task]       # 子任務

    def to_dict(self) -> dict
```

### Functions

#### parse_tasks

解析 tasks.md 內容。

```python
def parse_tasks(content: str) -> List[Task]
```

#### get_executable_tasks

取得可執行的任務 (依賴已滿足且未完成)。

```python
def get_executable_tasks(tasks: List[Task]) -> List[Task]
```

#### get_parallel_tasks

取得可並行執行的任務群組。

```python
def get_parallel_tasks(tasks: List[Task]) -> List[List[Task]]
```

#### topological_sort

拓撲排序任務 (依賴優先)。

```python
def topological_sort(tasks: List[Task]) -> List[Task]
```

---

## validate_config.py

驗證 workflow.yaml 配置。

### CLI

```bash
python3 .ai/scripts/validate_config.py [config_path] [--log-level LEVEL]
```

**參數：**
- `config_path` - 配置檔路徑，預設為 `.ai/config/workflow.yaml`

**Exit Codes：**
- 0 - 驗證通過
- 2 - 配置檔或 schema 不存在
- 3 - 驗證失敗

### 驗證項目

1. **Schema 驗證** - 檢查必要欄位和格式
2. **語意驗證**：
   - `submodule` type 需要 `.gitmodules` 存在
   - `directory` type 若有 `.git` 會警告
   - `root` type 的 path 必須是 `./`
   - `custom` rules 的檔案必須存在

---

## query_traces.py

查詢執行追蹤記錄。

### CLI

```bash
python3 .ai/scripts/query_traces.py [--issue-id ID] [--status STATUS] [--json]
```

**參數：**
- `--issue-id` - 篩選特定 issue
- `--status` - 篩選狀態 (success/failed)
- `--json` - 輸出 JSON 格式

### Functions

#### load_traces

載入所有追蹤記錄。

```python
def load_traces(trace_dir: Path) -> List[Dict[str, Any]]
```

#### summarize_trace

產生追蹤摘要。

```python
def summarize_trace(trace: Dict[str, Any]) -> Dict[str, Any]
```

**回傳值：**
```python
{
    "trace_id": str,
    "issue_id": str,
    "repo": str,
    "status": str,
    "duration_seconds": int,
    "started_at": str,
    "ended_at": str,
    "error": str,
    "failed_steps": List[str],
}
```

---

## Shell Scripts

### kickoff.sh

啟動工作流程入口。

```bash
bash .ai/scripts/kickoff.sh [--dry-run] [--background] [--help]
```

### run_issue_codex.sh

執行單一 Issue。

```bash
bash .ai/scripts/run_issue_codex.sh <issue_id> <ticket_file> <repo>
```

### write_result.sh

寫入執行結果。

```bash
bash .ai/scripts/write_result.sh <issue_id> <status> <pr_url> <summary_file>
```

**環境變數：**
- `AI_STATE_ROOT` - 狀態目錄根路徑
- `AI_RESULTS_ROOT` - 結果目錄根路徑
- `AI_EXEC_DURATION` - 執行時間 (秒)
- `AI_RETRY_COUNT` - 重試次數

### generate.sh

生成設定檔。

```bash
bash .ai/scripts/generate.sh [--generate-ci]
```

### evaluate.sh

評估腳本。

```bash
bash .ai/scripts/evaluate.sh [--offline] [--strict]
```

---

## 更多資源

- [架構說明](architecture.md) - 系統架構總覽
- [測試說明](testing.md) - 測試架構與執行
