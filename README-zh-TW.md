# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Bash](https://img.shields.io/badge/Bash-required-4EAA25?logo=gnubash&logoColor=white)]()
[![Python](https://img.shields.io/badge/Python-3.11-3776AB?logo=python&logoColor=white)](https://www.python.org/)
[![GitHub CLI](https://img.shields.io/badge/gh-required-181717?logo=github&logoColor=white)](https://cli.github.com/)

> 「睡前啟動，早上收割」的 AI 開發工作流 Kit：以 **Spec → 實作 → PR → 合併** 為主線，搭配 **Claude Code (Principal)** + **Codex (Worker)** 完成閉環；Spec 格式與 **Kiro** 相容。

[![下載](https://img.shields.io/badge/下載-最新版本-brightgreen?style=for-the-badge&logo=github)](https://github.com/silver2dream/ai-workflow-kit/releases/latest)

[English](README.md) | [繁體中文](README-zh-TW.md)

---

## 📋 目錄

- [特色](#-特色)
- [架構概覽](#-架構概覽)
- [技術棧](#-技術棧)
- [專案結構](#-專案結構)
- [快速開始](#-快速開始)
- [設定](#-設定)
- [Directory Monorepo 範例](#-directory-monorepo-範例)
- [CI](#-ci)
- [評估](#-評估)
- [文件](#-文件)
- [貢獻](#-貢獻)
- [授權](#-授權)

---

## ✨ 特色

### 核心工作流
- **Spec 驅動**：讀取 `.ai/specs/<name>/tasks.md`（Kiro 相容）決定下一步
- **GitHub 作為狀態機**：Issues/PR + labels 追蹤進度
- **派工 + 審查閉環**：派工給 Worker 產 PR，再由 Principal 審查、合併或退回產生修正 issue

### Kit 品質
- **Offline Gate**：離線可驗證（不需網路）
- **Strict mode**：`--strict` 強制 audit 無 P0（適用 CI/發布前）
- **Extensibility checks**：檢查 CI 是否會被 `feat/example` 觸發（避免分支對齊誤判）

---

## 🏗️ 架構概覽

```
┌─────────────────────────────────────────────────────────────┐
│                                                              │
│  You ──► awkit kickoff ──► Claude Code (Principal)            │
│                              │                               │
│                              ├─► read specs/tasks.md          │
│                              ├─► create GitHub Issue          │
│                              ├─► dispatch to Codex (Worker)   │
│                              ├─► review PR                    │
│                              ├─► merge or reject              │
│                              └─► loop                         │
│                                                              │
│  Morning ──► gh pr list ──► harvest                            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

完整架構文件：`docs/ai-workflow-architecture.md`

---

## 🛠️ 技術棧

### Offline（必備）
- `bash`（Windows: Git Bash / WSL）
- `git`
- `python3` + `pyyaml` + `jsonschema` + `jinja2`

### Online / E2E（選配）
- `gh`（GitHub CLI）+ `gh auth login`
- `claude`（Claude Code）
- `codex`（Worker）

---

## 📁 專案結構

```
.
├── .ai/                         # kit (scripts/templates/rules/specs)
│   ├── config/workflow.yaml     # main config
│   ├── scripts/                 # automation scripts
│   ├── templates/               # generators (CLAUDE/AGENTS/CI)
│   ├── rules/                   # architecture + git workflow rules
│   ├── docs/evaluate.md         # evaluation standard
│   └── specs/                   # Kiro-style specs
├── .github/workflows/ci.yml     # root CI example
├── backend/                     # directory example (Go)
└── frontend/                    # directory example (Unity skeleton)
```

---

## 🚀 快速開始

### 0) 安裝 `awkit`（建議）

`awkit` 是跨平台的 AWK 安裝 CLI（命名為 `awkit` 是為了避免和系統內建的 `awk` 指令衝突）。

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex
```

把 AWK 安裝到你的專案：

```bash
# 在當前目錄初始化 AWK
awkit init

# 使用 preset 並自動建立專案結構
awkit init --preset go --scaffold

# Monorepo：React + Go
awkit init --preset react-go --scaffold

# 預覽會建立哪些檔案
awkit init --preset python --scaffold --dry-run
```

### 可用的 Presets

| 類別 | Presets |
|------|---------|
| Single-Repo | `generic`, `go`, `python`, `rust`, `dotnet`, `node` |
| Monorepo | `react-go`, `react-python`, `unity-go`, `godot-go`, `unreal-go` |

執行 `awkit list-presets` 查看詳細說明。scaffold 檔案結構請參考 [Getting Started](docs/getting-started.md)。

注意：`awkit install` 是 `awkit init` 的別名（向後相容）。

### 0.1) 更新 `awkit`

確認版本與更新：

```bash
awkit version
awkit check-update
```

更新 CLI：

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

更新專案內的 kit 檔案（保留你的 workflow.yaml）：

```bash
awkit upgrade
bash .ai/scripts/generate.sh
```

其他更新選項：

```bash
# 只套用不同的 preset 到 workflow.yaml
awkit init --preset react-go --force-config

# 升級 kit 檔案，並覆蓋 workflow.yaml（需要 --preset）
awkit upgrade --force-config --preset react-go

# 完整重置：更新 kit 檔案並套用 preset 到 workflow.yaml
awkit init --preset react-go --force
```

### 1) 安裝 offline 依賴

```bash
pip3 install pyyaml jsonschema jinja2
```

### 2) 生成輸出

```bash
bash .ai/scripts/generate.sh
```

### 3)（選配）跑完整工作流

```bash
gh auth login

# 使用 awkit CLI（建議）
awkit kickoff --dry-run    # 預覽會執行什麼
awkit kickoff              # 啟動工作流
awkit kickoff --resume     # 從上次狀態恢復
awkit validate             # 只驗證設定

# 或使用 bash 腳本（legacy）
bash .ai/scripts/kickoff.sh --dry-run
bash .ai/scripts/kickoff.sh
```

停止：

```bash
touch .ai/state/STOP
```

---

## ⚙️ 設定

主設定：`.ai/config/workflow.yaml`

### Repo type

AWK 支援三種專案結構類型，在 `.ai/config/workflow.yaml` 中設定：

| Type | 說明 | 使用情境 |
|------|------|----------|
| `root` | 單一 repository | 獨立專案 |
| `directory` | Monorepo 子目錄 | 共用 .git 的 monorepo |
| `submodule` | Git submodule | 獨立 .git 的 monorepo |

**各類型行為差異：**
- **root**：所有操作在 repo 根目錄執行。Path 必須是 `./`。
- **directory**：操作在 worktree root 執行，變更限定在子目錄。
- **submodule**：commit/push 先在 submodule 執行，再更新 parent reference。

範例：
```yaml
repos:
  - name: backend
    path: backend/
    type: directory  # 或: root, submodule
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
```

### Specs

Spec 資料夾結構（Kiro 相容）：

```
.ai/specs/<feature-name>/
├── requirements.md   # optional
├── design.md         # optional
└── tasks.md          # required
```

要啟用 spec，將 spec 資料夾名稱加入 `.ai/config/workflow.yaml` 的 `specs.active`。

---

## 📦 Directory Monorepo 範例

這個 repo 內建一個可用的 directory 範例：

- `backend/`：最小 Go module + unit test（`go test ./...`）
- `frontend/`：Unity skeleton（CI 只做結構與 JSON sanity，不需要 Unity Editor）
- Spec 範例：`.ai/specs/example/`
- 入門指南：`docs/getting-started.md`

---

## 🔁 CI

Root CI workflow：`.github/workflows/ci.yml`

**使用者專案：**
- `awkit init` 會自動為你的專案建立 CI workflow
- `awkit upgrade` 會自動遷移舊版 CI 設定（移除過時的 `awk` job）

**此 repo（awkit 本身）：**
此 repo 內建的是手寫 CI 範例。`bash .ai/scripts/generate.sh` 預設不會改動 workflows；需要從模板生成時才使用 `--generate-ci`。

包含：
- AWK evaluation：`bash .ai/scripts/evaluate.sh --offline` 與 `--offline --strict`
- Kit tests：`bash .ai/tests/run_all_tests.sh`
- Backend：`go test ./...`（在 `backend/`）
- Frontend：`frontend/Packages/manifest.json` JSON 檢查 + 資料夾存在性

---

## 🧪 評估

- 僅供 kit 維護者 / CI 使用，一般使用者可跳過。
- 標準：`.ai/docs/evaluate.md`
- 執行器：`.ai/scripts/evaluate.sh`

---

## 📚 文件

### 使用者文件

| 文件 | 說明 |
|------|------|
| [Getting Started](docs/user/getting-started.md) | 快速入門指南 |
| [Configuration](docs/user/configuration.md) | workflow.yaml 參考 |
| [Troubleshooting](docs/user/troubleshooting.md) | 錯誤排解 |
| [FAQ](docs/user/faq.md) | 常見問題 |

### 開發者文件

| 文件 | 說明 |
|------|------|
| [Architecture](docs/developer/architecture.md) | 系統內部架構 |
| [API Reference](docs/developer/api-reference.md) | Scripts & modules |
| [Contributing](docs/developer/contributing.md) | 開發指南 |
| [Testing](docs/developer/testing.md) | 測試框架 |

### 其他

- [Architecture Overview](docs/ai-workflow-architecture.md) - 高階系統設計

---

## 🤝 貢獻

詳見 [Contributing Guide](docs/developer/contributing.md)：
- 開發環境設定
- 程式碼規範
- PR 工作流

快速參考：
- 分支策略與 commit 格式：`.ai/rules/_kit/git-workflow.md`
- PR base 預設 target `feat/example`

---

## 📄 授權

本專案採用 [Apache License 2.0](LICENSE) 授權。
