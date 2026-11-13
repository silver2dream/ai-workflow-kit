# Getting Started

> **Looking for the English quick start?** See [Quick Start Guide](quick-start.md) for a streamlined 5-minute setup.

本指南幫助你快速上手 AI Workflow Kit (AWK)。

---

## 系統需求

### 必要條件

| 工具 | 版本 | 說明 |
|------|------|------|
| Bash | 4.0+ | Shell 腳本 (Windows 可用 Git Bash 或 WSL) |
| Git | 2.20+ | 版本控制 |

`awkit` 是單一靜態 Go binary,不需要 Python 或其他執行期依賴。

### 可選條件 (完整工作流程)

| 工具 | 說明 |
|------|------|
| [GitHub CLI (gh)](https://cli.github.com/) | Issue/PR 操作 |
| Claude Code | Principal AI |
| Codex | Worker AI |

---

## 安裝方式

### 方式一：使用 awkit CLI (推薦)

**macOS / Linux:**

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex
```

安裝到專案：

```bash
# 在當前目錄初始化 AWK
awkit init

# 或使用 preset（Go 後端 + Godot C# 前端）
awkit init --preset godot-go

# 或指定路徑
awkit init /path/to/your-project --preset godot-go
```

升級現有專案：

```bash
# 升級 kit 檔案（保留你的 workflow.yaml）
awkit upgrade

# 重新生成輔助檔案
awkit generate
```

### 方式二：手動安裝

1. 複製 `.ai/` 目錄到你的專案
2. 執行生成命令：`awkit generate`

---

## 初始化專案

### 步驟 1：配置 workflow.yaml

編輯 `.ai/config/workflow.yaml`，設定你的專案結構：

**Single Repo 範例：**

```yaml
project:
  name: "my-project"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: python
    verify:
      build: "python -m py_compile *.py"
      test: "pytest"
```

**Monorepo 範例：**

```yaml
project:
  name: "my-monorepo"
  type: "monorepo"

repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: csharp
    verify:
      build: "dotnet build"
      test: "dotnet test"
```

### 步驟 2：生成設定檔

```bash
awkit generate
```

這會產生：
- `CLAUDE.md` - Principal AI 的指令檔
- `AGENTS.md` - Worker AI 的指令檔
- `.ai/rules/_kit/git-workflow.md` - Git 工作流程規則
- `.claude/settings.local.json` - Claude Code 權限設定（自動批准 gh、git 等命令）

### 步驟 3：驗證設定

```bash
awkit validate
```

如果看到 `Configuration is valid` 表示設定正確。

---

## 第一個工作流程

較大的需求可以先交給議會規劃，由它產出文件與票券：

```bash
awkit plan "幫 API server 加上健康檢查端點，回報 build 版本與資料庫狀態"
awkit plan approve <plan>    # 讀過 docs/plans/<plan>/tickets.yaml 之後，建立 Issue
```

`.ai/roster.yaml` 裡的角色會辯論這個需求，寫出 `docs/` 下各自擁有的文件，把決策記入 `docs/decisions.md`，並提出票券；你核可之前，GitHub 上不會有任何東西。加上 `--ask`，議會會在辯論前先把問題拿來問你；不加的話，它會寫明自己做了哪些假設。核可之後，直接跳到「3. 啟動前檢查」。

只有一件事要做時，照下面的步驟自己寫一張票。

### 1. 寫一張票券

工作就是一串票券：帶有 task 標籤（`ai-task`）的 GitHub Issue。內文說明要做什麼，並用一個小區塊寫明這張票屬於哪個 repo。

把內文寫進檔案，例如 `ticket.md`：

````markdown
## Summary
新增 health endpoint。

## Scope
- API server 的 GET /healthz

## Acceptance Criteria
- [ ] GET /healthz 回 200，並帶上 build 版本
- [ ] Unit tests added for new functionality

## Testing Requirements
- handler 要有單元測試，涵蓋正常與錯誤情況

```yaml awk-ticket
schema: 1
repo: root
```
````

`yaml awk-ticket` 區塊的欄位：`schema` 固定為 `1`；`repo` 是 `workflow.yaml` 裡的 repo 名稱或 `root`；需要等其他 Issue 先完成時，加上 `depends_on: [12, 15]`。完整說明見 `.ai/skills/principal-workflow/tasks/create-task.md`。

### 2. 建立 Issue

```bash
awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md
```

內文不合格（缺少 section、沒有區塊、repo 不在設定檔裡）時不會建立 Issue，並說明原因。

### 3. 啟動前檢查

```bash
awkit run --dry-run
```

`--dry-run` 只做啟動前檢查，不會開始工作。

### 4. 開始工作流程

確認無誤後，執行完整流程：

```bash
# 先登入 GitHub CLI
gh auth login

# 啟動工作流程：處理所有未完成的票券
awkit run

# 或只做一張票
awkit run --issue 42
```

票券依 Issue 編號由小到大處理，`depends_on` 裡的 Issue 都完成後才會開始。沒有票券可做、或有事情需要人處理時，執行就會停下；`awkit status` 會顯示目前的狀況。

### 5. 停止工作流程

```bash
touch .ai/state/STOP
```

---

## 常見安裝問題

### Bash 版本過舊 (macOS)

**問題：** macOS 內建 bash 版本為 3.x

**解決：**
```bash
brew install bash
```

### Windows 路徑問題

**問題：** 腳本執行時路徑錯誤

**解決：** 使用 Git Bash 或 WSL，避免使用 cmd.exe

### YAML 解析錯誤

**問題：** `yaml.scanner.ScannerError`

**解決：** 檢查 `workflow.yaml` 的縮排是否使用空格 (不要用 Tab)

---

## 下一步

- [配置說明](configuration.md) - 完整的 workflow.yaml 設定
- [故障排除](troubleshooting.md) - 常見錯誤與解決方案
- [FAQ](faq.md) - 常見問題

---

## awkit CLI 命令參考

| 命令 | 說明 |
|------|------|
| `awkit init` | 初始化新專案 |
| `awkit init --preset <name>` | 使用指定 preset 初始化 |
| `awkit upgrade` | 升級 kit 檔案，保留 workflow.yaml |
| `awkit uninstall` | 移除 AWK |
| `awkit list-presets` | 列出可用 preset |
| `awkit check-update` | 檢查 CLI 更新 |
| `awkit version` | 顯示版本 |
| `awkit help <command>` | 顯示命令說明 |
| `awkit plan "<需求>"` | 讓議會規劃：產出文件、決策與提案的票券（`--ask` 會先問你問題） |
| `awkit plan approve <plan>` | 核可工作清單，建立 Issue |
| `awkit create-task` | 從寫好的內文建立一張票券 |
| `awkit run` | 啟動工作流程 |
| `awkit run --dry-run` | 預覽工作流程 |
| `awkit validate` | 驗證配置 |
| `awkit doctor` | 檢查專案健康狀態，報告問題 |
| `awkit reset` | 重設專案狀態（支援 `--all`、`--dry-run` 等 flags） |
| `awkit generate` | 生成輔助文件與 scaffolding |
| `awkit evaluate` | 執行品質評估（離線/線上 gate 檢查與評分；`--offline`、`--strict`） |
| `awkit lessons` | 學習迴圈：`list` / `stats` / `add` / `distill` / `promote` |

### init 選項

```bash
awkit init [path] [options]

Options:
  --preset <name>     使用指定 preset (generic 或 godot-go)
  --scaffold          建立專案骨架（僅 godot-go）
  --force             覆蓋所有現有檔案
  --force-config      只覆蓋 workflow.yaml
  --dry-run           預覽操作，不實際執行
  --no-generate       跳過執行 awkit generate
  --project-name      覆蓋專案名稱
```

### upgrade 選項

```bash
awkit upgrade [path] [options]

Options:
  --scaffold          補充 scaffold 檔案 (需要 --preset)
  --preset <name>     scaffold 使用的 preset
  --force             覆蓋 scaffold 檔案
  --dry-run           預覽操作，不實際執行
  --no-generate       跳過執行 awkit generate
  --no-commit         跳過自動 commit
```

> **注意：** CI workflow 會在 `init` 時自動建立，`upgrade` 時會自動遷移（移除舊版的 awk job）。
