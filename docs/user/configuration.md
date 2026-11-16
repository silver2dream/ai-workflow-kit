# Configuration Guide

本文件詳細說明 `.ai/config.yaml` 的所有配置選項。

---

## 配置檔結構

```yaml
version: "1.2"
project: { ... }
repos: [ ... ]
git: { ... }
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

`language` 可以填任何名稱。下列語言 AWK 有額外支援，其他語言（或 `generic`）只會執行 `verify` 的指令，不做任何假設。

| 語言 | 值 | 額外支援 |
|------|-----|---------|
| Go | `go` | CI 依 `go.mod` 安裝 Go；審查時逐一核對測試名稱 |
| C# / Godot | `csharp` / `godot` | CI 安裝 .NET SDK；審查時以 `dotnet test` 逐一核對測試名稱 |
| Node.js | `node` / `typescript` / `javascript` | 審查時核對測試名稱；依 `package_manager` 推斷安裝指令 |
| Python | `python` | 審查時核對測試名稱 |

CI workflow 由 `awkit init` / `awkit generate` 依每個 repo 的 `verify.setup` / `build` / `test` 產生：`root` repo 是 `.github/workflows/ci.yml`，`directory` repo 各一份 `ci-<repo>.yml`。build 與 test 都沒設的 repo 不會有 workflow。產生的 workflow 開頭有一段註解，註解還在時 awkit 會讓它跟著設定檔更新；要自己改就先刪掉註解，之後 awkit 不再動它。

### 範例：Go 後端 + Godot C# 前端

```yaml
repos:
  - name: backend
    path: backend/
    type: directory
    language: go                # Go 版本取自 backend/go.mod
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: csharp            # Godot C#：只需要 .NET SDK
    verify:
      setup: "dotnet tool restore"   # 可省略：建置前要先跑的指令
      build: "dotnet build"
      test: "dotnet test"
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

1. `.claude/rules/{rule}.md` (使用者自訂優先)
2. `.claude/rules/_kit/{rule}.md` (Kit 核心)

### 使用範例規則

```bash
# 複製範例規則
# 規則包在 AWK repo 的 docs/examples/rules/，把要用的那一份複製進專案
cp <ai-workflow-kit>/docs/examples/rules/testing-strategy.md .claude/rules/

# 在 config.yaml 中啟用
rules:
  custom:
    - testing-strategy
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
  human_feedback: true         # 維護者的審查意見優先於自動審查（預設開）
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
| `human_feedback` | `true` | 維護者在 PR 上的審查意見優先於自動審查,見下方「人工糾正」 |

### 人工糾正:在 PR 上留審查意見

AWK 不等人:自動審查通過且 CI 通過就合併,不會為了等人看而停下來。你的審查意見什麼時候留都算數,差別只在 AWK 怎麼處理它。

| 你留意見時,PR 的狀態 | AWK 怎麼做 |
|---|---|
| 還沒合併 | 不會越過它合併:退回給 Worker 在同一個 PR 修,修完由審查者逐則核對 |
| 已經合併 | 記成教訓,下一次派工前就生效;已合併的程式不會被自動修改 |

意見要用 GitHub 的審查介面留在 AWK 開的 PR 上:在 diff 的某一行留言,或用「Review changes」送出一段說明。只認有寫入權限的帳號。

**PR 還沒合併時**

- **不會越過它合併。** PR 目前的 commit 上若有尚未解決的意見,即使自動審查給了通過的分數,AWK 也不會 approve 或合併,而是把 issue 退回給 Worker,並把你的意見原文交給它。
- **Worker 改完後會被核對。** 下一輪審查時,審查者會拿到你留過的每一則意見,逐一對照目前的程式碼;沒處理到的會被列為 Critical。
- **成為教訓。** 每則意見記入 `.ai/state/review_feedback.jsonl`（`source: human`）。這個 PR 通過後,蒸餾會以你的意見為主軸,產生的教訓直接是 `active`,不必先當 `candidate`。
- **你自己動手也算。** AWK 判定不通過或交給人工的 PR,若你推了修正再自行合併,下一次蒸餾會自動發現並把「被拒 → 你修好」當成一個案例。

**PR 已經合併時**

- **會被看到。** 每次派工前、每次合併後,以及執行 `awkit lessons distill` 時,AWK 都會檢查它合併過的 PR 有沒有新的審查意見。有的話記入 `review_feedback.jsonl`（`source: human`、`after_merge: true`）,連同你留言的那段程式碼。
- **立刻成為教訓。** 這種意見不需要等修正出現,你寫的話就是依據。能推廣到其他票的,蒸餾成 `active` 教訓,之後派出的 Worker 只要票的範圍相關就會帶著它。只跟那一次修改有關的意見（錯字、單純的提問）不會變成教訓。
- **已合併的程式不會被回頭修改。** 想修,請另開一張票。
- 想收回某則意見:在 AWK 下次檢查前把那串對話標為已解決（Resolve conversation）。

幾個細節:

- 只認審查介面寫的內容（review 的說明、行內留言）。PR 與 issue 下方的一般對話留言不算,留給討論用。
- PR 還開著時,想讓 AWK 放行某則意見:把那串對話標為已解決,或撤銷那次 review。
- AWK 唯一會停下來等你的情況:你在還沒合併的 PR 上留了意見,Worker 被退回一輪卻沒有推任何修改（例如你的留言是個問題）。這時 AWK 不會再退一輪,也不會合併,而是標上 `needs-human-review`。
- AWK 與你共用同一個 GitHub 帳號也沒關係:AWK 自己貼的內容都帶有標記,不會被當成你的意見。
- `lessons.enabled: false` 時不會檢查已合併的 PR。

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

學習迴圈:被拒絕的 PR 之後通過時,LLM 以「審查者反對什麼」加上「從被拒版本到通過版本改了什麼」蒸餾成精簡教訓（存於可提交的 `.ai/state/lessons.json`），注入後續 Worker/Reviewer prompt,並依 PR 結果結算命中/落空。已驗證（proven）的教訓可用 `awkit lessons promote <id>` 開一個人審 issue,固化成硬閘門。

會影響你看到的數字的幾條規則:

- 拒絕不會單獨蒸餾,要等同一個 issue 通過。一直沒通過的 issue 不會自動產生教訓,請用 `awkit lessons add --issue N` 手動記錄;`awkit lessons stats` 會顯示有幾筆拒絕在等待。
- 有 scope 的教訓,只有 PR 改到該 scope 底下的檔案時才計命中或落空;讀不到 PR 的檔案清單時不計。
- 同一個 issue 對一條教訓最多計一次落空,落空之後該 issue 的合併不算命中。

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
    verify:
      build: "python -m py_compile src/*.py"
      test: "pytest"

git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"

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
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: web
    path: web/
    type: directory
    language: node
    package_manager: "pnpm"
    verify:
      build: "pnpm build"
      test: "pnpm test"

git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"

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

## 議會名冊：`.ai/roster.yaml`

`awkit plan` 規劃時坐在桌邊的角色，寫在 `.ai/roster.yaml`，不在 `config.yaml` 裡。`awkit init` 會依 preset 寫出一份（`godot-go` 是小型遊戲工作室：producer、game-designer、numerical-designer、tech-lead、qa；其他 preset 是 product-owner、user-advocate、architect、skeptic）；專案沒有名冊時，第一次執行 `awkit plan` 也會寫一份。這個檔案是專案自己的，升級不會覆寫它。

```yaml
# language: 繁體中文      # 議會書寫的語言；不寫就跟著需求的語言
model: sonnet            # 沒有另外指定 model 的角色都用它，主持人也是

limits:
  disputes: 5            # 拿出來對辯的爭點數
  dispute_rounds: 2      # 每個爭點來回的次數
  calls: 80              # 一次規劃的模型呼叫上限
  parallel: 3            # 同時進行的呼叫數
  timeout_seconds: 600   # 單次呼叫的時限

roles:
  - id: product-owner
    title: Product Owner
    decides: true        # 仍有爭議時由它裁決；整份名冊恰好一個
    stance: |
      You answer for the outcome: what is built, for whom, and what is left out...
    owns:
      - docs/product/requirements.md
  - id: architect
    title: Architect
    plans: true          # 由它擬出票券；整份名冊恰好一個
    model: opus          # 這個角色改用別的模型
    stance: |
      You answer for how it is built and in what order...
    owns:
      - docs/tech/architecture.md
```

| 欄位 | 說明 |
|------|------|
| `roles[].id` | 小寫英數與連字號；出現在會議記錄與其他角色的發言裡 |
| `roles[].stance` | 這個角色主張什麼、反對什麼。立場要寫到彼此會衝突：大家都點頭的議會，產出的是沒人檢驗過的計畫 |
| `roles[].owns` | 只有這個角色會寫的文件。必須是 `docs/` 底下的 `.md`，一份文件只能有一個擁有者；`docs/decisions.md`、`docs/README.md` 與 `docs/plans/` 由議會自己維護，不能指派 |
| `roles[].decides` / `plans` | 各恰好一個角色；可以是同一個角色，但分開比較好 |
| `limits.calls` | 超過就停下，已完成的部分會保留，用 `awkit plan --resume <plan>` 接著做 |

一份文件最長約 40,000 位元組：角色每次都是整份重寫，超過這個長度的文件議會不會動它，並在會議記錄裡說明。名冊寫錯（沒有人裁決、兩個角色擁有同一份文件、拼錯欄位）會在 `awkit validate` 與 `awkit plan` 一開始就報錯。

---

## Config Migration

AWK 升級時可能引入 config 格式變更。Migration 系統會自動處理這些變更。

### 自動遷移

`awkit upgrade` 會自動偵測 `config.yaml` 的版本並執行所需的 migration：

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

Migration 執行前會自動建立 `config.yaml.bak` 備份。如果遷移結果有問題，可以還原：

```bash
cp .ai/config.yaml.bak .ai/config.yaml
```

---

## 下一步

- [故障排除](troubleshooting.md) - 配置錯誤的解決方案
- [FAQ](faq.md) - 常見問題
