# Architecture Guide

本文件說明 AI Workflow Kit 的內部架構，適用於 Kit 開發者與貢獻者。

> **近期新增子系統（v0.14.0）**：下方部分流程圖早於這些子系統,對整體 Principal→Worker→Reviewer 骨架仍準確,但尚未畫出以下模組。各模組的權威說明見 [API Reference](api-reference.md#主要子系統internal) 與 [Configuration](../user/configuration.md):
> - **學習迴圈**（`internal/lessons`）:記錄→蒸餾→注入→驗證,教訓存於 `.ai/state/lessons.json`
> - **結構化 review**（`reviewer/structured.go`）+ **severity/verdict 閘門** + **multi-model 共識**（`reviewer/multi.go`,接進 `submit.go`）
> - **Knowledge-graph 注入**（`worker/knowledgegraph.go`）與 **token/成本觀測**（`worker/usage.go`、trace `session_usage` 事件）
> - **原生 Windows 支援**（`kickoff/pty_windows.go` ConPTY）

---

## 總覽

AWK 採用 **Sequential Chain** 模式，由 Claude Code (Principal) 協調 Worker（`codex` 預設,或 `claude-code`）執行任務，並使用 GitHub 作為狀態機。審查端已從單一 reviewer 演進為結構化提交 + 證據驗證 + 選配的 multi-model 共識。

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
│  │  state/     results/     runs/     logs/     skills/    │ │
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
│   ├── repo_scan.schema.json      # repo 掃描輸出 Schema
│   ├── audit.schema.json          # audit 輸出 Schema
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
│   └── create-issues/             # Issue 建立技能
│       ├── SKILL.md
│       └── phases/                # 分析、分解、建立階段
│
├── templates/                     # 模板檔案
│   └── design.md.example          # 設計文件範例
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
├── state/                         # 狀態檔案 (runtime, gitignored)
│   ├── principal/                 # Principal 會話狀態
│   │   ├── session.json           # 當前會話
│   │   └── sessions/              # 歷史會話記錄
│   ├── traces/                    # 執行追蹤
│   ├── events/                    # 統一事件流 (含 session_usage token/成本)
│   ├── reviews/                   # 各 PR 的審查記錄
│   ├── lessons.json               # 學習迴圈教訓 (committable)
│   ├── review_feedback.jsonl      # 審查回饋記錄
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

## awkit CLI

`awkit` 是 AWK 的主要命令列工具，整合了所有工作流程功能：

| 指令 | 說明 |
|------|------|
| `awkit init` | 初始化專案 |
| `awkit kickoff` | 啟動工作流程（audit/掃描已內建於迴圈） |
| `awkit status` / `next` | 檢查工作流程狀態 / 下一動作 |
| `awkit dispatch-worker` | 調度 Worker 執行任務 |
| `awkit submit-review` | 提交審查（`--body-file` 結構化 JSON） |
| `awkit evaluate` | 評估 gate（`--offline`、`--strict`） |
| `awkit lessons` | 學習迴圈（list/stats/add/distill/promote） |

詳細用法請參考 `awkit help` 或專案 README。（註:舊的 `scan-repo` / `audit-project` 子命令已移除,邏輯內建於 `kickoff` 與 `audit-epic`。）

---

## 執行追蹤系統

### Trace Schema

每次 Worker 執行都會產生追蹤記錄：

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

---

## 資料流程

```
┌─────────────────────────────────────────────────────────────┐
│                                                              │
│  [awkit kickoff]                                             │
│       │                                                      │
│       ├─► preflight + audit（內建）                          │
│       │                                                      │
│       └─► Claude Code session                                │
│                 │                                            │
│                 ├─► 讀取 specs/tasks.md                      │
│                 │                                            │
│                 ├─► gh issue create ──► GitHub Issue         │
│                 │                                            │
│                 ├─► awkit dispatch-worker                    │
│                 │         │                                  │
│                 │         ├─► codex exec                     │
│                 │         │                                  │
│                 │         └─► results/issue-N.json           │
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

---

## 元件關係圖

```
                    ┌─────────────────────┐
                    │   workflow.yaml     │
                    │   (配置中心)         │
                    └──────────┬──────────┘
                               │
           ┌───────────────────┴───────────────────┐
           │                                       │
           ▼                                       ▼
    ┌─────────────┐                         ┌─────────────┐
    │   rules/    │                         │   skills/   │
    │  規則定義    │                         │  技能定義    │
    └──────┬──────┘                         └──────┬──────┘
           │                                       │
           └───────────────────┬───────────────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │   awkit kickoff     │
                    │ (工作流程入口;內建   │
                    │  preflight + audit) │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ awkit dispatch-worker│
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
