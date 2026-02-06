# Configuration Guide

本文件詳細說明 `.ai/config/workflow.yaml` 的所有配置選項。

---

## 配置檔結構

```yaml
version: "1.0"
project: { ... }
repos: [ ... ]
git: { ... }
specs: { ... }
tasks: { ... }
audit: { ... }
github: { ... }
rules: { ... }
escalation: { ... }
timeouts: { ... }
review: { ... }
# notifications: (planned for future release)
```

---

## project - 專案設定

```yaml
project:
  name: "my-project"           # 專案名稱
  description: "Description"   # 專案描述
  type: "monorepo"             # monorepo | single-repo
```

| 欄位 | 必填 | 說明 |
|------|------|------|
| `name` | 是 | 專案識別名稱 |
| `description` | 否 | 專案描述 |
| `type` | 是 | `monorepo` 或 `single-repo` |

---

## repos - 倉庫設定

```yaml
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
```

### 欄位說明

| 欄位 | 必填 | 說明 |
|------|------|------|
| `name` | 是 | 倉庫識別名稱 |
| `path` | 是 | 相對於專案根目錄的路徑 |
| `type` | 是 | `root` / `directory` / `submodule` |
| `language` | 是 | 程式語言 (影響 CI 模板) |
| `verify.build` | 是 | 建置指令 |
| `verify.test` | 是 | 測試指令 |

### type 類型說明

| 類型 | 使用時機 | 特點 |
|------|----------|------|
| `root` | Single-repo | 整個專案只有一個 repo，path 設為 `./` |
| `directory` | Monorepo 子目錄 | 多個專案在同一個 git repo，共用歷史記錄 |
| `submodule` | Git submodule | 各子專案有獨立的 git repo 和版本控制 |

### language 支援的語言

| 語言 | 值 | CI 模板 |
|------|-----|---------|
| Go | `go` | ci-go.yml.j2 |
| Node.js | `node` / `typescript` / `javascript` | ci-node.yml.j2 |
| Python | `python` | ci-python.yml.j2 |
| Rust | `rust` | ci-rust.yml.j2 |
| .NET | `dotnet` / `csharp` | ci-dotnet.yml.j2 |
| Unity | `unity` | ci-unity.yml.j2 |
| 其他 | `generic` | ci-generic.yml.j2 |

### 語言版本設定

```yaml
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    go_version: "1.25.x"        # Go 版本
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: node
    node_version: "20"          # Node.js 版本
    package_manager: "pnpm"     # npm | yarn | pnpm
    verify:
      build: "pnpm build"
      test: "pnpm test"
```

---

## git - Git 設定

```yaml
git:
  integration_branch: "feat/example"    # 開發整合分支
  release_branch: "main"                # 發布分支
  commit_format: "[type] subject"       # Commit 格式
  pr_body_template: |                   # PR 描述模板
    Closes #${ISSUE_ID}

    ${COMMIT_MSG}
```

| 欄位 | 必填 | 說明 |
|------|------|------|
| `integration_branch` | 是 | PR 預設的 base branch |
| `release_branch` | 是 | 正式發布的分支 |
| `commit_format` | 是 | Commit message 格式 |
| `pr_body_template` | 否 | PR 描述模板，支援變數 |

---

## specs - Spec 設定

```yaml
specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"   # 可選
    design: "design.md"               # 可選
    tasks: "tasks.md"                 # 必要
  auto_generate_tasks: true
  active:
    - my-feature
    - another-feature
```

| 欄位 | 必填 | 說明 |
|------|------|------|
| `base_path` | 是 | Spec 目錄根路徑 |
| `files` | 否 | Spec 檔案名稱對應 |
| `auto_generate_tasks` | 否 | 是否從 design.md 自動生成 tasks.md |
| `active` | 是 | 目前啟用的 spec 清單 |

### Spec 目錄結構

```
.ai/specs/
├── my-feature/
│   ├── requirements.md    # 可選：需求文檔
│   ├── design.md          # 可選：設計文檔
│   └── tasks.md           # 必要：任務清單
└── another-feature/
    └── tasks.md
```

---

## tasks - 任務格式設定

```yaml
tasks:
  format:
    uncompleted: "- [ ]"     # 未完成任務
    completed: "- [x]"       # 已完成任務
    optional: "- [ ]*"       # 可選任務
  source_priority:
    - audit                  # 優先處理 audit 發現的問題
    - specs                  # 再處理 specs 中的任務
```

---

## audit - 審計設定

```yaml
audit:
  checks:
    - dirty-worktree        # 檢查工作目錄是否乾淨
    - submodule-sync        # 檢查 submodule 是否同步
    - missing-tests         # 檢查是否缺少測試
    - missing-ci            # 檢查是否缺少 CI
  custom: []                # 自訂檢查項目
```

---

## github - GitHub 設定

```yaml
github:
  repo: ""                  # 留空表示使用 git remote origin
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    review_failed: "review-failed"
    worker_failed: "worker-failed"
```

| 欄位 | 說明 |
|------|------|
| `repo` | GitHub repo (owner/name)，空白則自動偵測 |
| `labels` | Issue/PR 使用的標籤 |

---

## rules - 規則設定

```yaml
rules:
  kit:
    - git-workflow          # Kit 核心規則 (自動生成)
  custom:                   # 使用者自訂規則
    - backend-architecture
    - frontend-patterns
```

### 規則查找順序

1. `.ai/rules/{rule}.md` (使用者自訂優先)
2. `.ai/rules/_kit/{rule}.md` (Kit 核心)

### 使用範例規則

```bash
# 複製範例規則
cp .ai/rules/_examples/backend-go.md .ai/rules/

# 在 workflow.yaml 中啟用
rules:
  custom:
    - backend-go
```

---

## escalation - 升級設定

```yaml
escalation:
  triggers:
    - pattern: "security|vulnerability"
      action: "require_human_approval"
    - pattern: "delete|drop|destroy"
      action: "pause_and_ask"
    - pattern: "migration|schema"
      action: "notify_only"

  max_consecutive_failures: 3
  retry_count: 2
  retry_delay_seconds: 5
  max_single_pr_files: 50
  max_single_pr_lines: 500
```

### action 類型

| Action | 說明 |
|--------|------|
| `require_human_approval` | 必須人工審批 |
| `pause_and_ask` | 暫停並詢問 |
| `notify_only` | 僅通知 |

---

## timeouts - 逾時設定

```yaml
timeouts:
  git_seconds: 120        # Git 操作逾時（秒）
  gh_seconds: 60          # GitHub CLI 操作逾時（秒）
  codex_minutes: 30       # Codex Worker 執行逾時（分鐘）
  gh_retry_count: 3       # GitHub API 重試次數
  gh_retry_base_delay: 2  # 重試基礎延遲（秒，指數退避）
```

各項操作的超時與重試設定。如果未設定，將使用上述預設值。

| 欄位 | 預設值 | 說明 |
|------|--------|------|
| `git_seconds` | `120` | Git 操作（如 fetch、push、rebase）的逾時秒數 |
| `gh_seconds` | `60` | GitHub CLI 操作（如 issue view、pr view）的逾時秒數 |
| `codex_minutes` | `30` | Codex Worker 單次執行的逾時分鐘數 |
| `gh_retry_count` | `3` | GitHub API 呼叫失敗時的最大重試次數 |
| `gh_retry_base_delay` | `2` | 重試的基礎延遲秒數，採用指數退避策略（2s → 4s → 8s） |

---

## review - 審查設定

```yaml
review:
  score_threshold: 7       # PR 審核通過的最低分數（1-10）
  merge_strategy: squash   # 合併策略：squash | merge | rebase
```

Principal 審查 Worker 提交的 PR 時使用的設定。

| 欄位 | 預設值 | 說明 |
|------|--------|------|
| `score_threshold` | `7` | PR 審核通過的最低分數（範圍 1-10）。Reviewer 給予的分數必須 >= 此值才會 approve PR |
| `merge_strategy` | `squash` | PR 通過審核後的合併方式。可選值：`squash`（壓縮合併）、`merge`（一般合併）、`rebase`（變基合併） |

---

## notifications - 通知設定 (planned for future release)

Slack/Discord webhook notifications are defined in the configuration schema but **not yet implemented** in the Go codebase. This section is reserved for a future release.

```yaml
# notifications: (planned for future release)
# slack_webhook: "${AI_SLACK_WEBHOOK}"
# discord_webhook: "${AI_DISCORD_WEBHOOK}"
# system_notify: true
```

---

## 完整範例

### Single-Repo (Python)

```yaml
version: "1.0"

project:
  name: "my-python-app"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: python
    python_version: "3.11"
    verify:
      build: "python -m py_compile src/*.py"
      test: "pytest"

git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  active: []

github:
  repo: ""

rules:
  kit:
    - git-workflow
  custom: []

escalation:
  max_consecutive_failures: 3
```

### Monorepo (Go + React)

```yaml
version: "1.0"

project:
  name: "fullstack-app"
  type: "monorepo"

repos:
  - name: api
    path: api/
    type: directory
    language: go
    go_version: "1.25.x"
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: web
    path: web/
    type: directory
    language: node
    node_version: "20"
    package_manager: "pnpm"
    verify:
      build: "pnpm build"
      test: "pnpm test"

git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  active: []

github:
  repo: ""

rules:
  kit:
    - git-workflow
  custom: []

escalation:
  max_consecutive_failures: 3
```

---

## 下一步

- [故障排除](troubleshooting.md) - 配置錯誤的解決方案
- [FAQ](faq.md) - 常見問題
