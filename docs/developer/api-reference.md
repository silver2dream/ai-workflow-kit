# API Reference

本文件說明 AI Workflow Kit 的 API 和命令參考。

> **重要提示：** 原有的 Python 腳本 (`scan_repo.py`, `audit_project.py`, `parse_tasks.py` 等)
> 已整合至 `awkit` CLI。建議使用 `awkit <command>` 取代直接呼叫腳本。
>
> | 舊腳本 | 新命令 |
> |--------|--------|
> | `python3 .ai/scripts/scan_repo.py` | (integrated into `awkit kickoff`) |
> | `python3 .ai/scripts/audit_project.py` | (integrated into `awkit kickoff`) |
> | `bash .ai/scripts/kickoff.sh` | `awkit kickoff` |
> | `python3 .ai/scripts/validate_config.py` | `awkit validate` |

---

## awkit CLI

`awkit` 是 AWK 的主要命令列工具，整合所有工作流程功能。

### 常用命令

```bash
awkit init              # 初始化專案
awkit init --preset go  # 使用 preset 初始化
awkit kickoff           # 啟動工作流程（audit/掃描已內建於 kickoff 迴圈）
awkit kickoff --dry-run # 預覽執行
awkit kickoff --resume  # 恢復上次執行
awkit status            # 檢查狀態（離線摘要）
awkit next              # 顯示下一個動作
awkit validate          # 驗證配置
awkit evaluate --offline  # 執行評估 gate（--strict 任一失敗即失敗）
awkit lessons list      # 學習迴圈教訓（list/stats/add/distill/promote）
awkit dispatch-worker   # 調度 Worker
awkit upgrade           # 升級 kit 檔案
awkit check-update      # 檢查更新
awkit version           # 顯示版本
```

> 完整命令清單見 `awkit help`。`scan-repo` / `audit-project` 等舊子命令已移除,相關邏輯內建於 `kickoff` 與 `awkit audit-epic`。

### 詳細用法

執行 `awkit --help` 或 `awkit <command> --help` 查看完整參數說明。

---

## Skills API

Skills 是 AWK 的技能系統，用於定義 Agent 的行為。

### 結構

```
.ai/skills/<skill-name>/
├── SKILL.md           # 技能入口與說明
├── phases/            # 流程階段
│   └── *.md           # 各階段指令
├── references/        # 參考文件
└── tasks/             # 任務範本
```

### 內建 Skills

| Skill | 說明 |
|-------|------|
| `principal-workflow` | Principal Agent 主工作流程 |
| `create-issues` | Issue 建立技能 |
| `run-issues` | Issue 執行技能 |
| `post-mortem` | 失敗事後分析（唯讀）；學習迴圈的手動入口 |
| `release-checklist` | 發布前 go/no-go 驗證 |

---


## CLI Commands

### awkit kickoff

啟動工作流程入口。提供 PTY 即時輸出、Issue Monitor 顯示 Worker 進度、Spinner 動畫。

```bash
awkit kickoff [--dry-run] [--background] [--resume] [--fresh]
awkit validate  # 只驗證配置
```

### awkit run-issue

執行單一 Issue（內部命令，由 `awkit kickoff` 調度）。

```bash
awkit run-issue <issue_id> <ticket_file> <repo>
```

### awkit status

查看工作流狀態與統計。

```bash
awkit status
```

### awkit generate

生成設定檔。

```bash
awkit generate [--generate-ci]
```

### awkit evaluate

執行評估 gate（離線 O0–O10 + 線上 gate），輸出各 gate 狀態與評分。`--strict` 時任一 gate 失敗即以非零退出（用於 CI/發布前）。

```bash
awkit evaluate [--offline] [--strict]
```

### awkit lessons

學習迴圈的教訓管理（見下方子系統）。

```bash
awkit lessons list [--all]         # 列出教訓
awkit lessons stats                # 狀態分布 + 命中/落空率
awkit lessons add --title ... --content ...   # 手動新增（post-mortem 入口）
awkit lessons distill [--max N]    # 從新的 review feedback 蒸餾教訓
awkit lessons promote <L-xxx>      # 為已驗證教訓開一個人審 issue（固化成硬閘門）
```

### awkit submit-review

提交 PR 審查結果。**推薦**用 `--body-file` 提交結構化 JSON（`StructuredReview`）；schema 錯誤會以退出碼 **2**（`SUBMISSION INVALID`）返回,由同 session 修正,不會浪費一輪 review。舊的 `--body`（markdown）路徑保留相容。

```bash
awkit submit-review --pr N --issue M --ci-status passed --body-file review.json
```

---

## 主要子系統（internal/）

| 套件 | 職責 |
|------|------|
| `internal/lessons` | 學習迴圈:Store（`.ai/state/lessons.json`）、Select（相關性/注入預算）、Settle（命中/落空狀態機）、Distill（fail-closed LLM 蒸餾） |
| `internal/reviewer` | 審查管線:`structured.go`（JSON 契約）、`severity.go`（severity/verdict 閘門）、`multi.go`/`secondary.go`/`consensus.go`（multi-model 共識,接進 `submit.go`）、`evidence.go`（證據驗證）、`feedback.go` |
| `internal/worker` | Worker 執行:`knowledgegraph.go`（Understand-Anything 地圖注入）、`usage.go`（token/成本掃描）、`lessons_inject.go`（教訓注入 + 歸因） |
| `internal/kickoff` | Principal 啟動器:PTY 執行（Windows ConPTY）、stream-json 解析、`session_usage` 事件 |
| `internal/trace` | 統一事件流（`.ai/state/events/`），含 `session_usage`（token/成本）|

---

## 更多資源

- [架構說明](architecture.md) - 系統架構總覽
- [測試說明](testing.md) - 測試架構與執行
