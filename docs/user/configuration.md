# Configuration Guide

本文件詳細說明 `.ai/config/workflow.yaml` 的所有配置選項。

---

## 配置檔結構

```yaml
version: "1.2"
project: { ... }
repos: [ ... ]
git: { ... }
specs: { ... }
tasks: { ... }
audit: { ... }
github: { ... }
rules: { ... }
agents: { ... }
timeouts: { ... }
escalation: { ... }
review: { ... }
feedback: { ... }
lessons: { ... }
hooks: { ... }
worker: { ... }
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
| `verify.setup` | 否 | 建置/測試前的依賴安裝指令（未設定時依 language/package_manager 自動推斷,如 `npm ci`、`pip install -r requirements.txt`） |
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
    needs_human_review: "needs-human-review"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"
    completed: "completed"
```

| 欄位 | 說明 |
|------|------|
| `repo` | GitHub repo (owner/name)，空白則自動偵測 |
| `labels.task` | 待處理任務 |
| `labels.in_progress` | 已派工執行中 |
| `labels.pr_ready` | Worker 完成、PR 待審 |
| `labels.review_failed` | 審查/驗證失敗,待重審 |
| `labels.worker_failed` | Worker 失敗,需人工介入 |
| `labels.needs_human_review` | 超過重試上限,需人工審查 |
| `labels.merge_conflict` | PR 有合併衝突 |
| `labels.needs_rebase` | PR 落後 base,需 rebase |
| `labels.completed` | 完成（success_no_changes） |

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

## agents - Agent 設定

```yaml
agents:
  builtin:
    - pr-reviewer           # PR 審查 agent
    - conflict-resolver     # 合併衝突解決 agent
  custom: []                # 自訂 subagent
```

`builtin` 為 kit 內建 agent（由 `awkit generate` 管理）。`custom` 可定義額外 subagent,每個會在 `awkit generate` 時生成到 `.claude/agents/`。

自訂 agent 欄位：

| 欄位 | 說明 |
|------|------|
| `name` | Agent 名稱（小寫字母、數字、連字號） |
| `description` | 描述（必填） |
| `tools` | 允許的工具（預設 `Read, Grep, Glob, Bash`） |
| `model` | `haiku` \| `sonnet` \| `opus`（預設 `opus`） |
| `trigger` | `review_pr` \| `check_result` \| `dispatch_worker` \| `generate_tasks` |
| `condition` | 觸發條件表達式 |

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
  score_threshold: 7           # PR 審核通過的最低分數（1-10）
  merge_strategy: squash       # 合併策略：squash | merge | rebase
  severity_consistency: true   # severity/verdict 一致性閘門（預設開）
  multi_model: false           # multi-model 共識審查
  # secondary_reviewers:
  #   - backend: claude
  #     model: opus
  #     focus_area: architecture
  # jittest:
  #   enabled: false
  #   max_tests: 5
  #   timeout_seconds: 120
  #   failure_policy: warn      # warn | block
```

Principal 審查 Worker 提交的 PR 時使用的設定。

| 欄位 | 預設值 | 說明 |
|------|--------|------|
| `score_threshold` | `7` | 審核通過的最低分數（1-10）。分數必須 >= 此值才 approve |
| `merge_strategy` | `squash` | 合併方式：`squash` / `merge` / `rebase` |
| `severity_consistency` | `true` | 一致性閘門：低於門檻的分數必須列出 >=1 個 Critical/Important 發現;達門檻的分數不得有 Critical 發現。不符則 `review_blocked` |
| `multi_model` | `false` | 開啟後,`submit-review` 取 PR diff、平行執行次要審查者,套用加權共識（primary×0.7 + secondaries 均分 0.3），任一審查者回報 `[ERROR]` 時共識分數上限 6 |
| `secondary_reviewers[]` | — | 次要審查者清單,每項 `{backend, model, focus_area}`。省略時預設一個 architecture-focused opus 審查者 |
| `jittest` | 停用 | 審查時從 PR diff 生成獨立測試（`awkit jittest`）。`failure_policy: block` 時失敗會 `review_blocked` |

---

## feedback - 審查回饋設定

```yaml
feedback:
  enabled: true               # 記錄審查拒絕並注入 Worker prompt
  max_history_in_prompt: 10   # 注入 Worker prompt 的最大歷史筆數
```

Review 拒絕會被記錄到 `.ai/state/review_feedback.jsonl`,並將 top rejection categories 注入後續 Worker prompt。此系統也是[學習迴圈](#lessons---學習迴圈設定)的資料來源。

| 欄位 | 預設值 | 說明 |
|------|--------|------|
| `enabled` | `true` | 是否記錄與注入 feedback |
| `max_history_in_prompt` | `10` | 注入 prompt 的最大歷史筆數 |

---

## lessons - 學習迴圈設定

```yaml
lessons:
  enabled: true               # 學習迴圈總開關
  max_active: 30              # active+proven 教訓上限
  inject_top_k: 3             # 每個 prompt 注入的教訓數上限
  inject_max_chars: 800       # 注入區段的字元上限
  distiller:
    backend: claude           # 蒸餾器後端（claude --print）
    model: sonnet
    timeout_seconds: 60
```

學習迴圈:審查拒絕經 LLM 蒸餾成精簡教訓（存於可提交的 `.ai/state/lessons.json`），注入後續 Worker/Reviewer prompt,並依 PR 結果結算命中/落空。已驗證（proven）的教訓可用 `awkit lessons promote <id>` 開一個人審 issue,固化成硬閘門。

指令:`awkit lessons list | stats | add | distill | promote`。

| 欄位 | 預設值 | 說明 |
|------|--------|------|
| `enabled` | `true` | 關閉後仍會記錄 feedback,但不注入/蒸餾教訓 |
| `max_active` | `30` | active+proven 教訓上限,超過時淘汰分數最低者 |
| `inject_top_k` | `3` | 每個 prompt 注入的教訓數上限（小 k 較佳） |
| `inject_max_chars` | `800` | 注入區段的硬字元上限 |
| `distiller.backend` | `claude` | 蒸餾器後端 |
| `distiller.model` | `sonnet` | 蒸餾器模型 |
| `distiller.timeout_seconds` | `60` | 蒸餾單筆的逾時 |

---

## hooks - 生命週期 Hook

```yaml
hooks:
  pre_dispatch:
    - command: "echo dispatching issue $AWK_ISSUE"
      timeout: "30s"
      on_failure: warn        # warn（預設）| abort | ignore
  on_merge:
    - command: "curl -X POST $SLACK_WEBHOOK -d '{\"text\": \"PR merged\"}'"
      timeout: "10s"
      on_failure: ignore
```

在工作流事件執行自訂 shell 命令。事件:`pre_dispatch`、`post_dispatch`、`pre_review`、`post_review`、`on_merge`、`on_failure`。

| 欄位 | 說明 |
|------|------|
| `command` | 要執行的 shell 命令 |
| `timeout` | 逾時（如 `30s`，經 `time.ParseDuration`） |
| `on_failure` | `warn`（預設，記錄）\| `abort`（中止工作流）\| `ignore` |
| `env` | 額外環境變數 |

---

## worker - Worker 設定

```yaml
worker:
  backend: codex              # codex（預設）| claude-code
  knowledge_graph: auto       # auto（存在時注入）| off
  # codex:
  #   full_auto: true
  #   max_attempts: 1
  # claude_code:
  #   model: sonnet
  #   max_turns: 50
  #   dangerously_skip_permissions: false
```

| 欄位 | 預設值 | 說明 |
|------|--------|------|
| `backend` | `codex` | Worker 後端:`codex` 或 `claude-code` |
| `knowledge_graph` | `auto` | 當 `.understand-anything/knowledge-graph.json` 存在時,把票券相關的程式碼地圖切片注入 Worker prompt;`off` 停用 |
| `codex.full_auto` | `true` | codex 使用 `--full-auto` |
| `codex.max_attempts` | `1` | codex 層級的重試次數 |
| `claude_code.model` | `sonnet` | claude-code Worker 的模型 |
| `claude_code.max_turns` | `50` | claude-code 的最大回合數 |
| `claude_code.dangerously_skip_permissions` | `false` | 是否跳過權限確認 |

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
version: "1.2"

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
version: "1.2"

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

## Config Migration

AWK 升級時可能引入 config 格式變更。Migration 系統會自動處理這些變更。

### 自動遷移

`awkit upgrade` 會自動偵測 `workflow.yaml` 的版本並執行所需的 migration：

```bash
# 升級 AWK 並自動遷移 config
awkit upgrade

# 預覽會執行的 migration（不修改檔案）
awkit upgrade --dry-run

# 跳過 config migration
awkit upgrade --skip-migrate
```

### 偵測過期 Config

使用 `awkit doctor` 可以檢查 config 是否需要遷移：

```bash
awkit doctor
```

如果 config 版本過舊，doctor 會顯示警告並建議執行 `awkit upgrade`。

### 手動遷移

通常執行 `awkit upgrade` 即可自動遷移。以下為各版本的主要變更。

**v1.0 → v1.1：**

1. **Label 值修正**：`review_failed: "review-fail"` → `review_failed: "review-failed"`
2. **新增 labels**：`merge_conflict`、`needs_rebase`、`completed`
3. **新增 timeout 欄位**：`gh_retry_count`、`gh_retry_base_delay`
4. **新增 review section**：`score_threshold`、`merge_strategy`
5. **版本號更新**：`version: "1.0"` → `version: "1.1"`

**v1.1 → v1.2：**

1. **新增 `agents` section**：`builtin`（pr-reviewer、conflict-resolver）+ `custom`
2. **新增 `feedback` section**：`enabled`、`max_history_in_prompt`
3. **新增 `lessons` section**（學習迴圈）：`enabled`、`max_active`、`inject_top_k`、`inject_max_chars`、`distiller.*`
4. **新增 `hooks` section**：生命週期 shell 命令
5. **新增 `worker` section**：`backend`（codex/claude-code）、`knowledge_graph`、`codex.*`、`claude_code.*`
6. **擴充 `review`**：`severity_consistency`、`multi_model`、`secondary_reviewers[]`、`jittest.*`
7. **新增 `verify.setup`**：建置前依賴安裝
8. **版本號更新**：`version: "1.1"` → `version: "1.2"`

### 備份

Migration 執行前會自動建立 `workflow.yaml.bak` 備份。如果遷移結果有問題，可以還原：

```bash
cp .ai/config/workflow.yaml.bak .ai/config/workflow.yaml
```

---

## 下一步

- [故障排除](troubleshooting.md) - 配置錯誤的解決方案
- [FAQ](faq.md) - 常見問題
