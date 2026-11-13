# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Bash](https://img.shields.io/badge/Bash-required-4EAA25?logo=gnubash&logoColor=white)]()
[![GitHub CLI](https://img.shields.io/badge/gh-required-181717?logo=github&logoColor=white)](https://cli.github.com/)

> 「睡前啟動，早上收割」的 AI 開發工作流 Kit：以 **票券 → 實作 → 審查 → 合併** 為主線，搭配會**自我改進**的審查閉環。工作就是一串 GitHub Issue；由 **Principal**（Claude Code）協調 **Worker**（Codex 或 Claude Code），品質由確定性的 Go 閘門把關 —— 而非盲信 LLM 輸出。

[![下載](https://img.shields.io/badge/下載-最新版本-brightgreen?style=for-the-badge&logo=github)](https://github.com/silver2dream/ai-workflow-kit/releases/latest)

[English](README.md) | [繁體中文](README-zh-TW.md)

---

## 🌟 AWK 的獨特之處

多數 AI 工作流工具只是「盲信模型輸出的派工迴圈」。AWK 建立在兩個讓它與眾不同的核心理念上：

### 🧠 會自我改進的學習迴圈

AWK 會**從自己的審查歷史中學習**。每次拒絕都走一條四步迴圈 —— **記錄 → 蒸餾 → 注入 → 驗證**：

- 被拒絕的 PR 最後通過時,LLM 把這個案例（審查者反對什麼、修正改了什麼）**蒸餾**成精簡、**可提交的教訓**（`.ai/state/lessons.json`）—— 是一條耐久的檢查,而非原始 log。
- 相關教訓會**注入**後續 Worker/Reviewer prompt（依你正在動的檔案精準比對,重用與 knowledge-graph 接地相同的相關性引擎）。
- 每條教訓依真實結果**結算命中/落空**（只計入有改到該教訓範圍的 PR）,並經 `candidate → active → proven` 晉升 —— 無效的教訓自動退役。
- **已驗證（proven）**的教訓可用 `awkit lessons promote` 經人審 issue **升級為硬閘門**（rule / audit 檢查）。
- **人的糾正是最高等級的訊號。** 工作流執行中在 GitHub 上審查 PR:AWK 不會越過你尚未解決的意見合併,會把意見交給 Worker,並把它蒸餾成一開始就受信任的教訓。

成果:**犯過一次的錯,會變成阻止它再犯的護欄。** 系統隨時間越來越難被騙 —— 而且因為教訓是可提交的 JSON,整個團隊共享這份學習(不像鎖在私有 runtime 裡的 agent 記憶)。

### 🤝 Agent 友善的介面（ACI）

AWK 把它的 agent 當成真正介面的一等使用者,而非「prose 進 / regex 出」:

- Reviewer 提交**結構化的 `review.json`**,由 AWK *渲染*成人類可讀的審查 —— 不再從手排的 markdown 反解析。
- **格式錯誤在同一 session 修正**(退出碼 2,秒級)—— 只有真正的**證據失敗**才需要重審一輪。介面會明確告訴 agent 該修哪個欄位。
- 品質由**確定性的 Go 閘門**把關:證據驗證(重跑測試、檢查每條標準對應到真實通過的斷言)、severity/verdict 一致性、multi-model 共識 —— 全在程式碼裡,非 agent 心算。

---

## 📋 目錄

- [AWK 的獨特之處](#-awk-的獨特之處)
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
- **Issue 就是工作**：票券是帶有 task 標籤的 GitHub Issue，內文用一個小小的 `yaml awk-ticket` 區塊寫明它屬於哪個 repo、要等哪些票先完成。沒有 spec 檔，也沒有要同步的任務清單
- **依編號、依依賴**：票券依編號由小到大處理，依賴的票落地後才開始；`awkit run --issue N` 只做一張票就停
- **GitHub 作為狀態機**：Issues/PR + labels 追蹤進度
- **派工 + 審查閉環**：派工給 Worker 產 PR，審查後合併，或帶著審查意見退回給 Worker 修正
- **Worker backend 可選**：`codex`（預設）或 `claude-code`（`worker.backend`）

### 審查品質
- **結構化 review 提交**：Reviewer 提交 `review.json`（`submit-review --body-file`）；格式錯誤在同 session 修正,只有真正的證據失敗才會 block
- **Evidence 驗證閘門**：合併前重跑測試,並驗證每條驗收標準都對應到一個通過的測試與真實斷言
- **Multi-model 共識**（選配）：`review.multi_model` 平行執行次要審查者並套用加權評分與 `[ERROR]` 封頂 —— 由 Go 強制執行,非 agent 心算
- **Severity/verdict 一致性閘門**：低於門檻的分數必須帶 Critical/Important 發現;通過的分數不得帶 Critical
- **JiT 測試**（選配）：審查時從 PR diff 生成獨立測試

### 學習迴圈
- **記錄 → 蒸餾 → 注入 → 驗證**：被拒後又通過的 PR,連同修正一起蒸餾成精簡、可提交的教訓（`.ai/state/lessons.json`）,注入後續 Worker/Reviewer prompt,並依教訓適用的 PR 結算命中/落空
- **可升級為硬閘門**：`awkit lessons promote` 開一個人審 issue,把已驗證的教訓固化成 rule/audit 檢查

### 上下文接地
- **Knowledge-graph 注入**：當 `.understand-anything/knowledge-graph.json` 存在時,注入與票券相關的程式碼地圖切片（檔案 + 相依者）（`worker.knowledge_graph`）

### Kit 品質與維運
- **Offline Gate**：離線可驗證（不需網路），`awkit evaluate --offline`
- **Strict mode**：`--strict` 強制 audit 無 P0（適用 CI/發布前）
- **跨平台**：原生 Windows 支援（ConPTY）+ Linux/macOS;完整測試套件在 CI 的 `windows-latest` 上執行
- **Token/成本觀測**：每 session 與每 worker 的 LLM token/成本追蹤（`awkit events`、`ResultMetrics`）
- **生命週期 hooks**：在 pre/post dispatch、pre/post review、on merge、on failure 執行 shell 命令

---

## 🏗️ 架構概覽

```
┌─────────────────────────────────────────────────────────────┐
│                                                              │
│  You ──► awkit run ──► Claude Code (Principal)                │
│                              │                               │
│                              ├─► pick the next ticket         │
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
- `bash`（Windows: Git Bash —— 部分驗證步驟會呼叫 `sh`；Principal runner 本身有原生 Windows/ConPTY 支援）
- `git`
- `go` 1.25+

### Offline（可選）
- `dotnet` 8 SDK：建置與測試 C# / Godot 前端（`godot-go` preset）

### Online / E2E（選配）
- `gh`（GitHub CLI）+ `gh auth login`
- `claude`（Claude Code）—— Principal 必備;`worker.backend: claude-code` 時 Worker 也需要
- `codex` —— `worker.backend: codex`（預設）時 Worker 需要

---

## 📁 專案結構

```
.
├── .ai/                         # kit 本體
│   ├── config/workflow.yaml     # 主設定檔
│   ├── rules/                   # git 工作流規則 + 可選的規則包
│   ├── skills/                  # Principal 依循的流程：主迴圈、票券格式
│   └── state/、results/         # 執行期狀態（gitignored）
├── .claude/                     # 給 Claude Code 的 agents、rules、skills（產生的）
├── CLAUDE.md、AGENTS.md         # 給 Principal 與 Worker 的指引（產生的）
└── .github/workflows/           # CI：每個 repo 一份 workflow，由設定檔產生
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
# 在當前目錄初始化 AWK：不限技術棧，建置與測試指令由你填
awkit init

# Go 後端 + Godot C# 前端，並建立一份可直接建置、測試會過的專案骨架
awkit init --preset godot-go --scaffold

# 預覽會建立哪些檔案
awkit init --preset godot-go --scaffold --dry-run
```

### 可用的 Presets

| Preset | 內容 |
|--------|------|
| `generic`（預設） | 單一 repo、不限技術棧。在 `.ai/config/workflow.yaml` 填 `verify.build` 與 `verify.test`，其餘不做任何假設。 |
| `godot-go` | Monorepo：`backend/` 用 Go、`frontend/` 用 Godot C#。加上 `--scaffold` 會建立可直接建置、測試會過的骨架（建置需要 .NET 8 SDK，開啟遊戲需要 Godot 4 .NET 版）。 |

執行 `awkit list-presets` 查看詳細說明。骨架的檔案結構請參考 [Getting Started](docs/getting-started.md)。

`awkit init` 也會替每個有建置或測試指令的 repo 寫一份 CI workflow，用的就是 Worker 驗證時跑的那組指令：單一 repo 是 `.github/workflows/ci.yml`，directory repo 各一份 `ci-<repo>.yml`。workflow 會一直跟著設定檔走；刪掉檔案開頭那段註解之後，它就歸你管，awkit 不再動它。

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
awkit generate
```

其他更新選項：

```bash
# 只套用不同的 preset 到 workflow.yaml
awkit init --preset godot-go --force-config

# 升級 kit 檔案，並覆蓋 workflow.yaml（需要 --preset）
awkit upgrade --force-config --preset godot-go

# 完整重置：更新 kit 檔案並套用 preset 到 workflow.yaml
awkit init --preset godot-go --force
```

### 1) 生成輸出

```bash
awkit generate
```

### 2) 建立票券並執行工作流

工作就是一串票券：帶有 task 標籤的 GitHub Issue，內文各有一個 `yaml awk-ticket` 區塊。

````markdown
## Summary
Add a health endpoint.

## Scope
- GET /healthz in the backend

## Acceptance Criteria
- [ ] GET /healthz answers 200 with the build version

## Testing Requirements
- A unit test covers the handler

```yaml awk-ticket
schema: 1
repo: backend          # a repo of .ai/config/workflow.yaml, or root
depends_on: [12]       # optional: issues that must be done first
```
````

```bash
gh auth login

awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md
awkit run --dry-run    # 只做啟動前檢查
awkit run              # 處理所有未完成的票券
awkit run --issue 42   # 或只做一張票
```

沒有票券可做，或有事情需要人處理時（票的區塊無法使用、Worker 重試後放棄、票在等一個不會完成的 Issue），執行就會停下。`awkit status` 會告訴你目前的狀況。

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

### 票券

票券的 `yaml awk-ticket` 區塊會被嚴格讀取。未知的欄位、設定檔裡沒有的 repo 都會被拒絕：`awkit create-task` 在建立 Issue 前就擋下；直接寫在 GitHub 上的，工作流程會在 Issue 留言說明原因。

| 欄位 | 意義 |
|------|------|
| `schema` | 固定為 `1` |
| `repo` | `workflow.yaml` 裡的 repo，或 `root` |
| `depends_on` | 必須先完成的 Issue |
| `severity` | `P0` 到 `P3`（可省略） |
| `release` | `true` 表示 PR 送往 release 分支（僅限 root repo） |
| `allow_script_changes`、`allow_parent_changes`、`allow_secrets` | 安全旗標；票沒有明確設定就是關閉 |

完整說明：`.ai/skills/principal-workflow/tasks/create-task.md`。

### Config 區段

`workflow.yaml` 的頂層區段（完整參考：[docs/user/configuration.md](docs/user/configuration.md)）：

| 區段 | 用途 |
|------|------|
| `project` / `repos` | Repo 佈局、類型、各 repo 的 `verify` 命令 |
| `git` | 整合/發布分支、commit 格式、PR 模板 |
| `audit` | audit 檢查 |
| `github` | Issue/PR labels、repo 覆寫 |
| `rules` / `agents` | 啟用的 kit/自訂規則與 subagents |
| `timeouts` / `escalation` | 操作逾時、重試/失敗上限、PR 大小上限 |
| `review` | 分數門檻、合併策略、**multi-model 共識**、**severity 閘門**、JiT 測試 |
| `feedback` | Review feedback 記錄/注入 |
| `lessons` | **學習迴圈**：蒸餾/注入預算、distiller model |
| `worker` | Backend（`codex`/`claude-code`）、**knowledge-graph 注入** |
| `hooks` | 生命週期 shell 命令 |

較新選項重點：

```yaml
review:
  score_threshold: 7
  severity_consistency: true    # 閘門：分數 vs Critical/Important 發現（預設開）
  human_feedback: true          # 維護者的 PR 審查意見優先於自動審查（預設開）
  multi_model: false            # 執行次要審查者 + 加權共識
  # secondary_reviewers:
  #   - backend: claude
  #     model: opus
  #     focus_area: architecture

worker:
  backend: codex                # codex（預設）| claude-code
  knowledge_graph: auto         # auto（存在時注入）| off

lessons:
  enabled: true                 # 學習迴圈：把審查拒絕蒸餾成可重用教訓
```

---

## 📦 專案骨架（`godot-go`）

`awkit init --preset godot-go --scaffold` 會寫出一份不用改就能建置、測試會過的專案：

- `backend/`：Go module，含 health handler 與測試（`go build ./...`、`go test ./...`）
- `frontend/`：Godot C# solution（`dotnet build`、`dotnet test`）
  - `game/`：Godot 專案
  - `src/Game.Core/`：不依賴引擎的遊戲邏輯
  - `tests/Game.Core.Tests/`：上述邏輯的 xUnit 測試
- `.github/workflows/ci-backend.yml`、`ci-frontend.yml`：跑同一組指令的 CI

這個 repo 自己的 CI 每次 push 都會初始化一份這樣的專案，並執行它產生的 workflow 裡的指令，所以骨架、preset 與 CI 產生器不會各走各的。

- 入門指南：`docs/getting-started.md`

---

## 🔁 CI

**使用者專案：**
- `awkit init` 會為每個有建置或測試指令的 repo 寫出一份 CI workflow，指令取自 `workflow.yaml`
- `awkit upgrade` 與 `awkit generate` 會讓這些 workflow 跟著設定檔走。把 workflow 第一行的註解刪掉，它就歸你管，awkit 不再動它

**此 repo（awkit 本身）：**
`.github/workflows/ci.yml` 是手寫的，所以 `awkit generate` 不會動它。包含三個 job：
- `awkit_cli`（ubuntu）：`go vet`、`go test -race`（60% 覆蓋率門檻閘門）、AWK evaluation（`awkit evaluate --offline` 與 `--offline --strict`）
- `awkit_cli_windows`（**windows-latest**）：在 Windows 跑完整 `go test ./...` 套件，守護原生 path/ConPTY/process 行為
- `skeleton`（ubuntu）：以 `--scaffold` 初始化一份 `godot-go` 專案，再用 awkit 為它產生的 workflow 裡的指令建置並測試

---

## 🧪 評估

- 僅供 kit 維護者 / CI 使用，一般使用者可跳過。
- 標準：`docs/developer/evaluation.md`
- 執行器：`awkit evaluate --offline`（僅報告）與 `awkit evaluate --offline --strict`（任一閘門失敗即失敗,如 P0 audit 發現 —— 用於 CI/發布前檢查）

---

## 📚 文件

### 使用者文件

| 文件 | 說明 |
|------|------|
| [Quick Start](docs/user/quick-start.md) | 5 分鐘快速設定 |
| [Getting Started](docs/user/getting-started.md) | 詳細入門指南 |
| [Configuration](docs/user/configuration.md) | 完整 `workflow.yaml` 參考 |
| [Skills](docs/user/skills.md) | Slash-command 技能參考 |
| [Troubleshooting](docs/user/troubleshooting.md) | 錯誤排解（含 Windows） |
| [FAQ](docs/user/faq.md) | 常見問題 |

### 開發者文件

| 文件 | 說明 |
|------|------|
| [Architecture](docs/developer/architecture.md) | 系統內部架構 |
| [API Reference](docs/developer/api-reference.md) | 模組與命令 |
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
- PR 的 base 是 `develop`；`main` 只用於發版

---

## 📄 授權

本專案採用 [Apache License 2.0](LICENSE) 授權。

## 🔒 安全性與信任

AWK 遵循開源安全最佳實踐，並由 [OpenSSF Scorecard](https://securityscorecards.dev/) 持續監控。

[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)

### 安全功能

| 功能 | 狀態 | 說明 |
|------|------|------|
| **SECURITY.md** | ✅ | 弱點回報政策與 SLA |
| **Branch Protection** | ✅ | 必要的審查與 CI 檢查 |
| **CI/CD** | ✅ | 所有 PR 自動化測試 |
| **Dependency Updates** | ✅ | 已啟用 Dependabot |
| **Static Analysis** | ✅ | CodeQL 掃描 |
| **Token Permissions** | ✅ | 最小化 GitHub token 權限 |

完整安全政策與弱點回報方式請參閱 [SECURITY.md](SECURITY.md)。
