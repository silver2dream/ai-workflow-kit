# Getting Started

本指南幫助你快速上手 AI Workflow Kit (AWK)。

---

## 系統需求

### 必要條件

| 工具 | 版本 | 說明 |
|------|------|------|
| Bash | 4.0+ | Shell 腳本 (Windows 可用 Git Bash 或 WSL) |
| Git | 2.20+ | 版本控制 |

### 可選條件 (legacy 腳本)

| 工具 | 版本 | 說明 |
|------|------|------|
| Python | 3.8+ | 僅用於 legacy 腳本（生成功能已內建於 `awkit`） |

```bash
# 僅在需要執行 legacy Python 腳本時安裝
pip3 install pyyaml jsonschema jinja2
```

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

# 或使用 preset
awkit init --preset react-go

# 或指定路徑
awkit init /path/to/your-project --preset react-go
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
    language: node
    verify:
      build: "npm run build"
      test: "npm test"
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

### 1. 建立 Spec

在 `.ai/specs/` 下建立你的第一個 spec：

```bash
mkdir -p .ai/specs/my-feature
```

建立 `tasks.md`：

```markdown
# My Feature

Repo: root
Coordination: sequential

## Tasks

- [ ] 1. Implement feature X
  - [ ] 1.1 Create data model
  - [ ] 1.2 Add API endpoint
  - [ ] 1.3 Write unit tests

- [ ] 2. Update documentation
```

### 2. 啟用 Spec

編輯 `.ai/config/workflow.yaml`：

```yaml
specs:
  active:
    - my-feature
```

### 3. 執行審計

```bash
awkit kickoff --dry-run
```

`--dry-run` 會顯示將執行的操作，但不會實際執行。

### 4. 開始工作流程

確認無誤後，執行完整流程：

```bash
# 先登入 GitHub CLI
gh auth login

# 啟動工作流程
awkit kickoff
```

### 5. 停止工作流程

```bash
touch .ai/state/STOP
```

---

## 常見安裝問題

### Python 套件安裝失敗

**問題：** `pip3 install` 時出現權限錯誤

**解決：**
```bash
pip3 install --user pyyaml jsonschema jinja2
```

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
| `awkit kickoff` | 啟動工作流程 |
| `awkit kickoff --dry-run` | 預覽工作流程 |
| `awkit kickoff --resume` | 從上次狀態恢復 |
| `awkit validate` | 驗證配置 |
| `awkit doctor` | 檢查專案健康狀態，報告問題 |
| `awkit reset` | 重設專案狀態（支援 `--all`、`--dry-run` 等 flags） |
| `awkit evaluate` | 執行品質評估（離線/線上 gate 檢查與評分） |

### init 選項

```bash
awkit init [path] [options]

Options:
  --preset <name>     使用指定 preset (generic, react-go, go, python 等)
  --scaffold          建立專案基本結構
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
