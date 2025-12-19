# AI Workflow

「睡前啟動，早上收割」的全自動 AI 開發工作流。

使用 Claude Code (Principal) + Codex (Worker) 實現：分析任務 → 創建 Issue → 實作代碼 → 審查 PR → 合併，完整閉環。

## Quick Start

```bash
# 1. 啟動工作流
bash .ai/scripts/kickoff.sh

# 2. 背景執行（睡前用）
bash .ai/scripts/kickoff.sh --background

# 3. 查看進度
bash .ai/scripts/stats.sh

# 4. 停止
touch .ai/state/STOP
```

## Docs

| 文件 | 說明 |
|------|------|
| `docs/getting-started.md` | Directory 型 monorepo 範例（含 CI） |
| `.ai/docs/evaluate.md` | 評分標準與 Gate 定義 |

## 架構概覽

```
┌─────────────────────────────────────────────────────────────┐
│                                                              │
│  你 ──► kickoff.sh ──► Claude Code (Principal)               │
│                              │                               │
│                              ├─► 分析 tasks.md               │
│                              ├─► 創建 GitHub Issue           │
│                              ├─► 派工給 Codex (Worker)       │
│                              ├─► 審查 PR                     │
│                              ├─► 合併 or 退回                │
│                              └─► Loop                        │
│                                                              │
│  早上 ──► gh pr list ──► 收割成果 🎉                         │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## 常用命令

| 命令 | 說明 |
|------|------|
| `bash .ai/scripts/kickoff.sh` | 啟動工作流 |
| `bash .ai/scripts/kickoff.sh --dry-run` | 只做前置檢查 |
| `bash .ai/scripts/kickoff.sh --background` | 背景執行 |
| `bash .ai/scripts/stats.sh` | 查看統計報告 |
| `bash .ai/scripts/stats.sh --json` | JSON 格式輸出 |
| `bash .ai/scripts/rollback.sh <PR>` | 回滾已合併的 PR |
| `bash .ai/scripts/rollback.sh <PR> --dry-run` | 預覽回滾操作 |
| `bash .ai/scripts/cleanup.sh` | 清理已完成的 worktrees 和分支 |
| `bash .ai/scripts/cleanup.sh --dry-run` | 預覽清理操作 |
| `bash .ai/scripts/cleanup.sh --days 7` | 只清理 7 天前的 |
| `bash .ai/scripts/stats.sh --html` | 生成 HTML 報告 |
| `python3 .ai/scripts/parse_tasks.py <file>` | 解析任務依賴圖 |
| `python3 .ai/scripts/parse_tasks.py <file> --next` | 顯示下一個可執行任務 |
| `bash .ai/scripts/notify.sh --summary` | 發送統計通知 |
| `bash .ai/scripts/generate.sh` | 重新生成 CLAUDE.md 和 AGENTS.md |
| `touch .ai/state/STOP` | 停止工作流 |

## 環境需求

- `claude` CLI (Claude Code Pro)
- `codex` CLI (Codex Business)
- `gh` CLI (GitHub CLI)
- `git`
- `python3` + `pyyaml` + `jinja2`
- `bash` (Windows: Git Bash / WSL)

### GitHub CLI 認證

`gh` CLI 需要認證才能操作 Issues 和 PRs：

```bash
# 方法 1：互動式登入（推薦）
gh auth login

# 方法 2：環境變數
export GH_TOKEN="ghp_xxxxxxxxxxxx"

# 方法 3：CI/CD 環境
# GitHub Actions 自動提供 GITHUB_TOKEN
```

驗證認證狀態：
```bash
gh auth status
```

### Windows 額外設置

本工具使用符號連結（symlinks）讓 `.claude/` 指向 `.ai/`。Windows 需要額外設置：

**方法 1：開啟開發人員模式（推薦）**
1. 設定 → 更新與安全性 → 開發人員專用
2. 開啟「開發人員模式」

**方法 2：以管理員身份執行**
- 以管理員身份執行 Git Bash 或終端機

如果無法創建符號連結，腳本會自動回退到複製文件。但這樣修改 `.ai/rules/` 後需要手動執行 `bash .ai/scripts/generate.sh` 來同步。

**跨平台 Python 腳本**

關鍵腳本提供 Python 跨平台版本，在沒有 bash 的環境下使用：

```bash
# 掃描 repo 結構
python3 .ai/scripts/scan_repo.py --json

# 專案審計
python3 .ai/scripts/audit_project.py --json

# 解析任務依賴
python3 .ai/scripts/parse_tasks.py tasks.md --next
```

## Spec 工作流

Spec 是功能規格文件，用於定義和追蹤開發任務。支援與 Kiro 相容的格式。

### Spec 文件結構

```
.ai/specs/<feature-name>/
├── requirements.md   # (可選) 需求文檔
├── design.md         # (可選) 設計文檔
└── tasks.md          # (必要) 任務清單
```

### 自動生成 tasks.md

如果你有 `design.md` 但沒有 `tasks.md`，Principal 會自動從設計文檔生成任務清單：

1. 用其他 AI 工具（如 Cursor、Kiro）生成 `design.md`
2. 將 spec 加入 `workflow.yaml` 的 `specs.active` 列表
3. 執行 `bash .ai/scripts/kickoff.sh`
4. Principal 會讀取 `design.md` 並生成 Kiro 相容格式的 `tasks.md`

### tasks.md 格式（Kiro 相容）

```markdown
# Feature Name - Implementation Plan

## 目標
功能描述

---

## Tasks

- [ ] 1. 主任務名稱
  - [ ] 1.1 子任務
    - 任務描述
    - _Requirements: 1.1_
  - [ ]* 1.2 可選子任務（測試）
    - _Requirements: 1.2_

- [ ] 2. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.
```

格式說明：
- `- [ ]` 未完成任務
- `- [x]` 已完成任務
- `- [ ]*` 可選任務（通常是測試）
- `_Requirements: X.X_` 引用需求編號
- `_depends_on: 1, 2_` 任務依賴（Task DAG）

### 任務依賴（Task DAG）

支援任務間的依賴關係，用於識別可並行執行的任務：

```markdown
- [ ] 1. 建立資料庫 schema
- [ ] 2. 實作 API endpoint
  - _depends_on: 1_
- [ ] 3. 實作前端 UI
  - _depends_on: 2_
- [ ] 4. 寫測試
  - _depends_on: 1_  # 只依賴 1，可與 2, 3 並行
```

使用 `parse_tasks.py` 分析依賴：
```bash
python3 .ai/scripts/parse_tasks.py tasks.md --next      # 下一個可執行任務
python3 .ai/scripts/parse_tasks.py tasks.md --parallel  # 可並行的任務群組
```

### Multi-Repo Tickets

支援同時修改多個 repo 的任務：

```markdown
- Repo: backend, frontend
- Coordination: sequential  # sequential | parallel
- Sync: required           # required | independent
```

- `sequential`: 依序執行，前一個成功才執行下一個
- `parallel`: 並行執行（需要多 Worker）
- `Sync: required`: 所有 repo 的 PR 必須同時合併

## 專案結構

```
.
├── .ai/                        # AI Workflow Kit
│   ├── config/
│   │   └── workflow.yaml       # 主配置文件
│   ├── scripts/                # 所有腳本
│   │   ├── kickoff.sh
│   │   ├── run_issue_codex.sh
│   │   ├── stats.sh
│   │   ├── notify.sh
│   │   ├── generate.sh
│   │   └── ...
│   ├── templates/              # 模板
│   │   ├── CLAUDE.md.j2
│   │   └── AGENTS.md.j2
│   ├── rules/                  # 架構規則
│   │   ├── git-workflow.md
│   │   ├── backend-go.md
│   │   └── frontend-unity.md
│   ├── commands/               # Claude Code 指令
│   │   ├── start-work.md
│   │   └── ...
│   ├── specs/                  # 功能規格和任務
│   │   └── <feature>/
│   │       ├── design.md       # 設計文檔（可選）
│   │       └── tasks.md        # 任務清單
│   ├── state/                  # 執行狀態
│   ├── results/                # 執行結果
│   ├── runs/                   # 執行記錄
│   └── exe-logs/               # 日誌
│
├── CLAUDE.md                   # Principal 指南（從模板生成）
├── AGENTS.md                   # Worker 指南（從模板生成）
├── .claude/
│   ├── commands/ → .ai/commands/
│   └── rules/ → .ai/rules/
│
├── backend/                    # 後端 (submodule)
└── frontend/                   # 前端 (submodule)
```

## 配置

編輯 `.ai/config/workflow.yaml` 來配置：

- 專案資訊
- Repo 列表和驗證命令
- Git 分支策略
- Spec 位置
- 審計規則
- 通知設定

修改配置後執行 `bash .ai/scripts/generate.sh` 重新生成：
- `CLAUDE.md` 和 `AGENTS.md`
- `.github/workflows/ci.yml`（每個 repo）
- `.github/workflows/validate-submodules.yml`（monorepo 時）

### 支援的語言和框架

| 類別 | `language` 值 | CI 模板 | 版本設定 |
|------|---------------|---------|----------|
| **Go** | `go`, `golang` | ci-go.yml.j2 | `go_version: "1.22.x"` |
| **Node.js** | `node`, `nodejs`, `typescript`, `javascript` | ci-node.yml.j2 | `node_version: "20"`, `package_manager: "npm"` |
| **Web 框架** | `react`, `vue`, `angular`, `nextjs`, `nuxt`, `svelte` | ci-node.yml.j2 | 同上 |
| **Node 後端** | `express`, `nestjs` | ci-node.yml.j2 | 同上 |
| **Python** | `python`, `django`, `flask`, `fastapi` | ci-python.yml.j2 | `python_version: "3.11"` |
| **Rust** | `rust` | ci-rust.yml.j2 | `rust_version: "stable"` |
| **.NET/C#** | `dotnet`, `csharp`, `aspnet`, `blazor` | ci-dotnet.yml.j2 | `dotnet_version: "8.0.x"` |
| **Unity** | `unity` | ci-unity.yml.j2 | - |
| **Unreal** | `unreal`, `ue4`, `ue5` | ci-unreal.yml.j2 | `ue_version: "5.3"`, `project_name: "MyGame"` |
| **Godot** | `godot` | ci-godot.yml.j2 | `godot_version: "4.2.2"`, `use_dotnet: "false"` |
| **其他** | `generic` 或任意值 | ci-generic.yml.j2 | - |

**遊戲引擎說明：**
- **Unity**: 檢查專案結構、manifest.json、.meta 文件等
- **Unreal**: 
  - 預設：.uproject 驗證 + UE C++ 編碼規範檢查（UCLASS/GENERATED_BODY/常見錯誤）
  - 可選：完整編譯（需要設定 Epic Games 帳號的 secrets，使用官方 Docker image）
- **Godot**: 使用 headless 模式執行專案導入和 GDScript 檢查

**Unreal 完整編譯設定：**
1. 在 GitHub repo 設定 secrets：`RUNUAT_USER` 和 `RUNUAT_TOKEN`（Epic Games 帳號）
2. 取消 CI 模板中 `full-build` job 的註解
3. 或使用 self-hosted runner 預裝 UE

框架（如 React、Vue）會自動使用對應語言的 CI 模板，差異在於你配置的 `verify.build` 和 `verify.test` 命令。

範例：
```yaml
# React 前端
- name: frontend
  language: react
  node_version: "20"
  package_manager: "pnpm"
  verify:
    build: "pnpm build"
    test: "pnpm test"

# FastAPI 後端
- name: api
  language: fastapi
  python_version: "3.12"
  verify:
    build: "pip install -e ."
    test: "pytest"
```

### Repo 類型

| 類型 | `type` 值 | 說明 | CI 位置 |
|------|-----------|------|---------|
| 單一 Repo | `root` | 整個專案是一個 repo | `.github/workflows/ci.yml` |
| 子目錄 | `directory` | Monorepo 內的資料夾 | `.github/workflows/ci-{name}.yml` |
| Submodule | `submodule` | 獨立的 Git repo | `{path}/.github/workflows/ci.yml` |

範例配置（monorepo with directories）：
```yaml
project:
  type: "monorepo"

repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    go_version: "1.22.x"
    verify:
      build: "cd backend && go build ./..."
      test: "cd backend && go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: typescript
    node_version: "20"
    package_manager: "pnpm"
    verify:
      build: "cd frontend && pnpm build"
      test: "cd frontend && pnpm test"
```

## 文件

### 架構設計
- [完整架構設計](docs/ai-workflow-architecture.md)

### 規則文件
- [Git 工作流](.ai/rules/git-workflow.md)
- [Backend (Go)](.ai/rules/backend-go.md)
- [Frontend (Unity)](.ai/rules/frontend-unity.md)

### Agent 指南
- [CLAUDE.md](CLAUDE.md) - Principal (Claude Code) 指南
- [AGENTS.md](AGENTS.md) - Worker (Codex) 指南

## 通知配置（可選）

```bash
# Slack
export AI_SLACK_WEBHOOK="https://hooks.slack.com/services/xxx/yyy/zzz"

# Discord
export AI_DISCORD_WEBHOOK="https://discord.com/api/webhooks/xxx/yyy"

# 禁用系統通知
export AI_SYSTEM_NOTIFY=false
```

## 測試

```bash
# 執行所有測試
bash .ai/tests/run_all_tests.sh

# 詳細輸出
bash .ai/tests/run_all_tests.sh --verbose

# 只驗證配置
python3 .ai/scripts/validate_config.py
```

測試套件會檢查：
- 配置文件語法和 schema 驗證
- 必要文件是否存在
- 模板語法是否正確
- 生成的文件是否有效
- 錯誤分析功能
- 歷史趨勢追蹤
- 任務依賴解析
- 跨平台腳本

## v2 新功能

### 智能錯誤恢復
- 自動分析失敗原因（編譯錯誤、測試失敗、網路問題等）
- 根據錯誤類型決定是否重試
- 失敗歷史記錄在 `.ai/state/failure_history.jsonl`

### 歷史趨勢追蹤
- 每次執行 `stats.sh` 自動記錄到 `stats_history.jsonl`
- 計算 7 天趨勢：日均完成數、成功率
- `--json` 輸出包含趨勢數據

### 成本追蹤
- 記錄每個任務的執行時間
- 結果文件包含 `metrics.duration_seconds`
- `stats.sh` 彙總總執行時間和平均時間

### 人工升級觸發
配置敏感操作的人工審查：
```yaml
escalation:
  triggers:
    - pattern: "security|credential"
      action: "pause_and_ask"
    - pattern: "delete|drop"
      action: "require_human_approval"
  max_consecutive_failures: 3
  max_single_pr_files: 50
  max_single_pr_lines: 500
```

## License

Private
