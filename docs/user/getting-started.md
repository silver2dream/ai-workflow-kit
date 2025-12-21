# Getting Started

本指南幫助你快速上手 AI Workflow Kit (AWK)。

---

## 系統需求

### 必要條件

| 工具 | 版本 | 說明 |
|------|------|------|
| Python | 3.8+ | 核心腳本執行 |
| Bash | 4.0+ | Shell 腳本 (Windows 可用 Git Bash 或 WSL) |
| Git | 2.20+ | 版本控制 |

### Python 套件

```bash
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

### 方式二：手動安裝

1. 複製 `.ai/` 目錄到你的專案
2. 安裝 Python 依賴：`pip3 install pyyaml jsonschema jinja2`
3. 執行生成腳本：`bash .ai/scripts/generate.sh`

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
bash .ai/scripts/generate.sh
```

這會產生：
- `CLAUDE.md` - Principal AI 的指令檔
- `AGENTS.md` - Worker AI 的指令檔
- `.ai/rules/_kit/git-workflow.md` - Git 工作流程規則

### 步驟 3：驗證設定

```bash
python3 .ai/scripts/validate_config.py
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
bash .ai/scripts/kickoff.sh --dry-run
```

`--dry-run` 會顯示將執行的操作，但不會實際執行。

### 4. 開始工作流程

確認無誤後，執行完整流程：

```bash
# 先登入 GitHub CLI
gh auth login

# 啟動工作流程
bash .ai/scripts/kickoff.sh
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
