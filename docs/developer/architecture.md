# Architecture Guide

本文件說明 AI Workflow Kit 的內部架構，適用於 Kit 開發者與貢獻者。

---

## 總覽

AWK 採用 **Sequential Chain** 模式，由 Claude Code (Principal) 協調 Codex (Worker) 執行任務，並使用 GitHub 作為狀態機。

```
┌─────────────────────────────────────────────────────────────┐
│                    LOCAL MACHINE                             │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Claude Code (Principal)                    │ │
│  │                                                         │ │
│  │  [Analyzer] ──► [Dispatcher] ──► [Reviewer]            │ │
│  │       │              │               │                  │ │
│  │       └──────────────┼───────────────┘                  │ │
│  │                      │                                  │ │
│  │               Event Router                              │ │
│  │            (Sequential Chain)                           │ │
│  └──────────────────────┼──────────────────────────────────┘ │
│                         │                                    │
│  ┌──────────────────────┼──────────────────────────────────┐ │
│  │              Codex (Worker)                              │ │
│  │                      │                                   │ │
│  │  codex exec ──► implement ──► create PR ──► result.json │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │                    .ai/ (State Store)                    │ │
│  │                                                          │ │
│  │  state/     results/     runs/     logs/     traces/    │ │
│  └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
                         │ gh CLI
                         ▼
┌──────────────────────────────────────────────────────────────┐
│                  GITHUB (State Machine)                       │
│                                                               │
│  Issues ──────────────────► PRs                               │
│  [ai-task] [in-progress]    [open] [merged]                  │
└───────────────────────────────────────────────────────────────┘
```

---

## 目錄結構

```
.ai/
├── config/
│   ├── workflow.yaml              # 主配置檔
│   ├── workflow.schema.json       # 配置 Schema
│   ├── repo_scan.schema.json      # scan_repo 輸出 Schema
│   ├── audit.schema.json          # audit_project 輸出 Schema
│   ├── execution_trace.schema.json # 執行追蹤 Schema
│   └── failure_patterns.json      # 錯誤模式定義
│
├── skills/                        # Agent Skills (Principal/Worker 技能)
│   ├── principal-workflow/        # Principal 工作流程技能
│   │   ├── SKILL.md               # 技能入口
│   │   ├── phases/                # 流程階段
│   │   │   └── main-loop.md       # 主迴圈邏輯
│   │   ├── references/            # 參考文件
│   │   └── tasks/                 # 任務範本
│   ├── create-issues/             # Issue 建立技能
│   │   ├── SKILL.md
│   │   └── phases/                # 分析、分解、建立階段
│   └── run-issues/                # Issue 執行技能
│       ├── SKILL.md
│       └── phases/                # 分析、並行、驗證階段
│
├── templates/                     # 模板檔案
│   ├── design.md.example          # 設計文件範例
│   └── _deprecated/               # 已棄用的 Jinja2 模板 (legacy)
│
├── rules/
│   ├── _kit/                      # Kit 核心規則
│   │   └── git-workflow.md        # Git 工作流程規則
│   └── _examples/                 # 範例規則 (可選啟用)
│
├── specs/                         # Spec 規格目錄
│   └── example/                   # 範例 spec
│
├── docs/                          # Kit 內部文件
│   └── evaluate.md                # 評估指南
│
├── state/                         # 狀態檔案
│   ├── principal/                 # Principal 會話狀態
│   │   ├── session.json           # 當前會話
│   │   └── sessions/              # 歷史會話記錄
│   ├── traces/                    # 執行追蹤
│   ├── consecutive_failures       # 連續失敗計數
│   └── loop_count                 # 迴圈計數
│
├── results/                       # 執行結果
│   └── issue-*.json
│
├── runs/                          # 執行記錄 (Worker runs)
│
├── logs/                          # 結構化日誌
│   └── <command>-<date>.log
│
├── exe-logs/                      # 執行日誌
│   └── principal.log              # Principal 執行日誌
│
├── analysis/                      # 分析結果 (掃描/審計輸出)
│
└── tests/                         # 測試 fixtures
    └── fixtures/                  # 測試資料
```

> **Note:** 原有的 Python 腳本 (`scan_repo.py`, `audit_project.py`, `parse_tasks.py` 等)
> 已整合至 `awkit` CLI。請使用 `awkit <command>` 取代直接呼叫腳本。

---

## Exit Codes 規範

所有 Python 腳本使用統一的 exit codes：

| Code | 常數 | 說明 |
|------|------|------|
| 0 | `EXIT_SUCCESS` | 執行成功 |
| 1 | `EXIT_ERROR` | 一般執行錯誤 |
| 2 | `EXIT_CONFIG_ERROR` | 配置或依賴錯誤 |
| 3 | `EXIT_VALIDATION_ERROR` | 驗證失敗 |

定義於 `.ai/scripts/lib/errors.py`。

---

## 錯誤處理框架

### 錯誤類別

```python
from lib.errors import AWKError, ConfigError, ValidationError, ExecutionError

# 使用範例
raise ConfigError(
    message="Config file not found",
    suggestion="Run generate.sh first"
)

raise ValidationError(
    message="Invalid repo type",
    details={"type": "foo", "allowed": ["root", "directory", "submodule"]}
)
```

### 錯誤輸出格式

錯誤以 JSON 格式輸出到 stderr：

```json
{
  "error": {
    "type": "config_error",
    "code": 2,
    "message": "Config file not found",
    "reason": "Required configuration or dependency is missing.",
    "impact": "The workflow cannot continue with the current setup.",
    "suggestion": "Run generate.sh first",
    "details": {}
  }
}
```

---

## 結構化日誌系統

### Logger 使用

```python
from lib.logger import Logger, normalize_level

logger = Logger("my_script", ai_root / "logs", level="info")

logger.debug("Detailed info", {"key": "value"})
logger.info("Operation completed", {"count": 10})
logger.warn("Something unusual", {"warning": "msg"})
logger.error("Operation failed", {"error": "details"})
```

### 日誌格式

日誌以 JSON 格式寫入 `.ai/logs/<script>-<date>.log`：

```json
{"timestamp": "2025-01-04T10:30:00Z", "level": "info", "source": "scan_repo", "message": "Scan completed", "context": {"files": 100}}
```

### 命令列參數

所有腳本支援 `--log-level` 參數：

```bash
python3 .ai/scripts/scan_repo.py --log-level debug
```

---

## 執行追蹤系統

### Trace Schema

每次 `run_issue_codex.sh` 執行都會產生追蹤記錄：

```json
{
  "trace_id": "uuid",
  "issue_id": "123",
  "repo": "backend",
  "branch": "feat/ai-issue-123",
  "status": "success",
  "started_at": "2025-01-04T10:00:00Z",
  "ended_at": "2025-01-04T10:05:00Z",
  "duration_seconds": 300,
  "error": null,
  "steps": [
    {
      "name": "checkout",
      "status": "success",
      "started_at": "...",
      "ended_at": "...",
      "duration_seconds": 5
    }
  ]
}
```

### 查詢追蹤

```bash
# 查詢失敗的執行
python3 .ai/scripts/query_traces.py --status failed

# 查詢特定 issue
python3 .ai/scripts/query_traces.py --issue-id 123 --json
```

---

## 資料流程

```
┌─────────────────────────────────────────────────────────────┐
│                                                              │
│  [awkit kickoff] 或 [kickoff.sh]                             │
│       │                                                      │
│       ├─► run_script scan_repo ──► state/repo_scan.json     │
│       │                                                      │
│       ├─► run_script audit_project ──► state/audit.json     │
│       │                                                      │
│       └─► Claude Code session                                │
│                 │                                            │
│                 ├─► parse_tasks.py ──► 讀取 tasks.md         │
│                 │                                            │
│                 ├─► gh issue create ──► GitHub Issue         │
│                 │                                            │
│                 ├─► run_issue_codex.sh                       │
│                 │         │                                  │
│                 │         ├─► codex exec                     │
│                 │         ├─► write_result.sh                │
│                 │         │         │                        │
│                 │         │         └─► results/issue-N.json │
│                 │         │                                  │
│                 │         └─► traces/issue-N.json            │
│                 │                                            │
│                 ├─► gh pr diff ──► Review                    │
│                 │                                            │
│                 └─► gh pr merge / request-changes            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## 重試機制

### 配置

```yaml
# workflow.yaml
escalation:
  retry_count: 2
  retry_delay_seconds: 5
  max_consecutive_failures: 3
```

### 流程

```
執行失敗
    │
    ▼
檢查 retry_count
    │
    ├─► count < max ──► 等待 delay ──► 重試
    │
    └─► count >= max
            │
            ▼
      標記 [worker-failed]
      記錄到 result.json
      停止此 issue
```

### 結果記錄

```json
{
  "issue_id": "123",
  "status": "failed",
  "metrics": {
    "duration_seconds": 120,
    "retry_count": 2
  }
}
```

---

## Schema 驗證

所有輸出檔案都有對應的 JSON Schema：

| 檔案 | Schema |
|------|--------|
| workflow.yaml | workflow.schema.json |
| repo_scan.json | repo_scan.schema.json |
| audit.json | audit.schema.json |
| traces/*.json | execution_trace.schema.json |

驗證方式：

```bash
python3 .ai/scripts/validate_config.py
```

---

## 測試架構

```
.ai/tests/
├── pytest.ini          # pytest 配置
├── conftest.py         # 共用 fixtures
├── fixtures/           # 測試資料
│   ├── valid_workflow.yaml
│   ├── invalid_workflow.yaml
│   └── sample_tasks.md
│
└── unit/
    ├── test_scan_repo.py
    ├── test_audit_project.py
    ├── test_parse_tasks.py
    ├── test_validate_config.py
    ├── test_errors.py
    ├── test_query_traces.py
    └── test_write_result.py
```

執行測試：

```bash
python3 -m pytest .ai/tests/unit -v
```

---

## 元件關係圖

```
                    ┌─────────────────────┐
                    │   workflow.yaml     │
                    │   (配置中心)         │
                    └──────────┬──────────┘
                               │
           ┌───────────────────┼───────────────────┐
           │                   │                   │
           ▼                   ▼                   ▼
    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
    │ scan_repo   │     │   audit     │     │ parse_tasks │
    │             │     │  project    │     │             │
    └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
           │                   │                   │
           ▼                   ▼                   ▼
    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
    │ repo_scan   │     │   audit     │     │   tasks     │
    │   .json     │     │   .json     │     │   解析結果   │
    └─────────────┘     └─────────────┘     └─────────────┘
           │                   │                   │
           └───────────────────┼───────────────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │  awkit kickoff 或   │
                    │  kickoff.sh (legacy)│
                    │  (工作流程入口)       │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │  run_issue_codex    │
                    │  (Worker 調度)       │
                    └──────────┬──────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
              ▼                ▼                ▼
       ┌───────────┐    ┌───────────┐    ┌───────────┐
       │  result   │    │   trace   │    │   logs    │
       │   .json   │    │   .json   │    │   .log    │
       └───────────┘    └───────────┘    └───────────┘
```

---

## 更多資源

- [API 參考](api-reference.md) - 函數與模組說明
- [貢獻指南](contributing.md) - 開發規範與 PR 流程
- [測試說明](testing.md) - 測試架構與執行方式
