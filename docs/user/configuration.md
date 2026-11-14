# 設定檔

`.ai/config.yaml` 是專案對 AWK 說的話：有哪些 repo、往哪個分支合併、用什麼指令建置與測試。`awkit init` 寫下第一份。

- 改過之後執行 `awkit generate`：`CLAUDE.md`、`AGENTS.md`、`.claude/rules/_kit/` 與 CI workflow 都是從這份檔案產生的。
- `awkit validate` 檢查這份檔案，並列出**檔案裡有、但沒有任何程式會讀的鍵**：舊版寫下而現在不用的、拼錯的。那些鍵不會有任何作用，可以刪掉。
- 沒寫的鍵用預設值。必填的只有 `project.name`、`project.type`、`git.integration_branch`，以及每個 repo 的 `name` 與 `path`。

規劃議會的名冊是另一個檔案，見[最後一節](#名冊airosteryaml)。

---

## 新專案的設定檔

`awkit init --preset godot-go` 寫下的就是這些；其他的鍵都用預設值，要改再加。

```yaml
version: "1.2"

project:
  name: "demo"
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

git:
  integration_branch: "develop"
  release_branch: "main"

rules:
  custom:
    - backend
    - frontend
    - testing

worker:
  backend: codex

review:
  score_threshold: 7
  merge_strategy: squash
```

---

## `version`

設定檔格式的版本，目前是 `"1.2"`。`awkit upgrade` 看到較舊的版本會把檔案轉成新格式，原檔留一份 `.bak`。沒有這個鍵會被當成 `"1.0"`。

## `project`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `name` | 必填 | 專案名稱，出現在 `CLAUDE.md` 與 `awkit status` |
| `type` | 必填 | `single-repo` 或 `monorepo`。`monorepo` 時，產生的 git 規則會列出各個 repo；有 submodule 的 monorepo 另外多一個檢查 submodule 的 CI workflow |
| `description` | 空 | 給讀這份檔案的人看；AWK 不讀它 |

## `repos`

一張票只會給一個 repo（票的 `awk-ticket` 區塊的 `repo:`）。`root`（repository 根目錄）永遠可以用，不必列出來；列出來是為了給它建置與測試指令。

| 鍵 | 預設 | 說明 |
|---|---|---|
| `name` | 必填 | 票券用來指名這個 repo 的名字，大小寫不拘 |
| `path` | 必填 | 相對於 repository 根目錄的路徑；`root` 是 `./` |
| `type` | `directory` | `root`：repository 本身。`directory`：同一個 repository 裡的資料夾。`submodule`：git submodule，Worker 先在 submodule 裡 commit、push，再更新外層的參照 |
| `language` | 空 | 任何名稱都可以。`go`、`csharp`／`godot`／`dotnet` 會讓 CI workflow 先裝好工具鏈；`go`、`csharp`、`typescript`／`javascript`、`python` 會讓審查逐一核對測試名稱。其他語言只執行你給的指令 |
| `verify.build`、`verify.test` | 空 | 建置與測試指令。Worker 開 pull request 前執行，審查時測試會再跑一次，CI workflow 也用同一組。兩個都沒設的 repo 不會有 CI workflow，也沒有任何東西能證明變更是對的：**開工前要填** |
| `verify.setup` | 空 | 建置前先跑的指令（安裝相依套件）。沒填時 Worker 依語言推斷：Node 用 `package_manager` 指定的工具安裝，Python 執行 `pip install -r requirements.txt`，.NET 執行 `dotnet restore`；其他語言不做事。CI workflow 只在有填的時候才有這一步 |
| `package_manager` | `npm` | `npm`、`yarn` 或 `pnpm`；只在 Node 的 repo 沒填 `verify.setup` 時有用 |

指令在 repo 的目錄裡執行；在 Windows 上透過 Git Bash。

## `git`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `integration_branch` | 必填 | pull request 合併進去的分支。Worker 的分支從它切出來，所以它必須已經在 `origin` 上 |
| `release_branch` | `main` | 發版用的分支。只有一條主幹的專案，兩個鍵填同一個分支 |

`awkit init` 會填 repository 裡已有的分支：有 `develop` 就是 `develop` 合併、預設分支發版；沒有就是單一主幹。

commit 訊息與 pull request 標題的格式固定是 `[type] subject`（例如 `[feat] add login endpoint`），不是設定。

## `github`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `repo` | 空 | `owner/name`。空的時候用 `gh` 從 `origin` 認出來的 repository |

工作流程用九個標籤記錄每張票走到哪裡：`ai-task`、`in-progress`、`pr-ready`、`review-failed`、`worker-failed`、`needs-human-review`、`merge-conflict`、`needs-rebase`、`completed`。`awkit run` 開始時會建立缺少的標籤。**這些名字不能改**：設定檔的 `github.labels` 若寫了別的名字，`awkit validate` 會報錯，run 也不會開始。

## `rules`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `kit` | `[git-workflow]` | kit 的規則，`awkit generate` 寫到 `.claude/rules/_kit/`，升級時會更新 |
| `custom` | `[]` | 專案自己的規則。名稱 `backend` 對應 `.claude/rules/backend.md`；列在這裡，`AGENTS.md` 才會要 Worker 讀它。檔案不存在時 `awkit generate` 會警告 |

寫規則的方式見 [Skills 與規則](skills.md)。

## `agents`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `builtin` | `[pr-reviewer, conflict-resolver]` | kit 的 subagent |
| `custom` | `[]` | 專案自己的 subagent，`awkit generate` 會把每一個寫成 `.claude/agents/<name>.md` |

`custom` 的每一項：

| 鍵 | 說明 |
|---|---|
| `name` | 必填。小寫字母、數字與連字號，字母開頭；不能和內建的同名 |
| `description` | 必填。Claude Code 依它決定何時使用這個 subagent |
| `tools` | 允許的工具，例如 `Read, Grep, Glob, Bash` |
| `model` | `haiku`、`sonnet` 或 `opus` |
| `trigger` | `review_pr`、`check_result` 或 `dispatch_worker`：寫進 subagent 的檔案，說明它在流程的哪一步被用到 |
| `condition` | 一句話，寫進 subagent 的檔案，說明什麼情況下用它 |

## `worker`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `backend` | `codex` | 誰來寫程式：`codex` 或 `claude-code`。對應的 CLI 必須裝好並登入，`awkit doctor` 會檢查 |
| `codex.full_auto` | `true` | 用 `--full-auto` 執行 codex |
| `claude_code.model` | `sonnet` | Claude Code 當 Worker 時用的模型 |
| `claude_code.max_turns` | `50` | 一次嘗試最多幾輪 |
| `claude_code.dangerously_skip_permissions` | `false` | 以 `--dangerously-skip-permissions` 執行。Worker 在自己的 worktree 裡工作，但這個旗標讓它不再受任何權限設定限制；只在隔離的機器上打開 |
| `knowledge_graph` | `auto` | `auto`：專案有 `.understand-anything/knowledge-graph.json` 時，把和這張票有關的一小段放進 Worker 的 prompt。`off`：不放 |

## `review`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `score_threshold` | `7` | 審查分數（1–10）達到這個數字才算通過 |
| `merge_strategy` | `squash` | `squash`、`merge` 或 `rebase` |
| `severity_consistency` | `true` | 分數與發現的問題要對得上：低於門檻的審查至少要列一項 Critical 或 Important，通過的審查不能有 Critical |
| `human_feedback` | `true` | 有寫入權限的人在 pull request 上留下、還沒解決的審查意見，會擋住合併並把票退回給 Worker；合併之後才留下的意見會在下一次派工前被讀進來，變成教訓。合併本身不等人 |
| `multi_model` | `false` | 除了主要的審查者，再請其他模型各看一次，分數合併：主要審查者占 0.7，其餘平分 0.3；任何一位回報 `[ERROR]` 等級的問題時，最後分數不超過 6 |
| `secondary_reviewers` | 一位著重架構的 `opus` | `multi_model` 打開時的其他審查者。每一項有 `model`、`focus_area`（例如 `architecture`、`security`），以及只用來顯示的 `backend` |
| `jittest.enabled` | `false` | 審查時依 diff 產生測試、跑一次、然後丟掉 |
| `jittest.max_tests` | `5` | 最多產生幾個測試 |
| `jittest.timeout_seconds` | `120` | 產生與執行的時間上限 |
| `jittest.failure_policy` | `warn` | `warn`：失敗的測試寫進審查留言。`block`：失敗的測試讓審查不通過 |
| `jittest.model` | `sonnet` | 產生測試的模型 |

## `escalation`

工作流程重試幾次之後才找人。

| 鍵 | 預設 | 說明 |
|---|---|---|
| `max_review_attempts` | `3` | 同一個 pull request 被審查退回這麼多次之後，票會被標上 `needs-human-review` |
| `retry_count` | `2` | Worker 的一次派工裡，失敗後再試幾次。`0` 會被當成沒寫（也就是 2）；不要重試請寫 `-1` |
| `retry_delay_seconds` | `5` | 兩次嘗試之間等幾秒 |

另外有兩個不在設定檔裡的上限：同一張票的派工失敗 3 次之後會被標上 `worker-failed`；連續 5 次失敗時整個 run 停下來。

## `timeouts`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `git_seconds` | `120` | Worker 的每個 git 指令 |
| `gh_seconds` | `60` | Worker 的每個 `gh` 指令 |
| `codex_minutes` | `30` | Worker 的一次嘗試，不論 backend 是 `codex` 或 `claude-code` |
| `verify_minutes` | `10` | 一個建置、測試或 setup 指令：Worker 交付前跑的每一個，以及審查重跑的測試。超過就連同它啟動的行程一起停掉，算驗證失敗 |
| `gh_retry_count` | `3` | `gh` 指令失敗時重試幾次 |
| `gh_retry_base_delay` | `2` | 重試之間的基本等待秒數，每次加倍 |

一次派工最多等 Worker 60 分鐘。`codex_minutes` 乘上嘗試次數、再加上驗證指令的時間若超過這個數字，派工會先放棄。測試跑得久的專案，調高 `verify_minutes`。

## `feedback`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `enabled` | `true` | Worker 的 prompt 帶上「審查最常退回的三類問題」。關掉只是不帶，紀錄仍會留下（`.ai/state/review_feedback.jsonl`） |

## `lessons`

被審查退回、後來終於通過的變更，會被濃縮成教訓，放進之後動到同樣檔案的 Worker 與審查者的 prompt。教訓存在 `.ai/state/lessons.json`：只在這份 checkout 裡，不進版控。

| 鍵 | 預設 | 說明 |
|---|---|---|
| `enabled` | `true` | 關掉之後不再濃縮、也不再放進 prompt |
| `max_active` | `30` | 最多保留幾條有效的教訓；超過時分數最低的先退場 |
| `inject_top_k` | `3` | 一個 prompt 最多放幾條 |
| `inject_max_chars` | `800` | 教訓那一段的字元上限 |
| `distiller.model` | `sonnet` | 濃縮教訓用的模型 |
| `distiller.timeout_seconds` | `60` | 濃縮一次的時間上限 |

## `hooks`

在流程的幾個時間點執行你的指令。指令在專案根目錄執行（Unix 用 `sh -c`，Windows 用 `cmd /c`），輸出會出現在 run 的輸出裡。

```yaml
hooks:
  pre_dispatch:
    - command: "echo dispatching issue $AWK_ISSUE"
      timeout: "30s"
      on_failure: warn
  on_merge:
    - command: "./scripts/notify.sh"
      env:
        CHANNEL: "dev"
```

| 事件 | 何時 | 指令拿到的環境變數 |
|---|---|---|
| `pre_dispatch` | Worker 開始之前 | `AWK_ISSUE`、`AWK_REPO` |
| `post_dispatch` | Worker 結束之後 | `AWK_ISSUE`、`AWK_STATUS`，成功時還有 `AWK_PR_URL` |
| `on_failure` | 一張票的派工失敗次數用完之後 | `AWK_ISSUE`、`AWK_FAILURE_REASON`、`AWK_ATTEMPTS` |
| `pre_review` | 審查結果送出、還沒處理之前 | `AWK_PR`、`AWK_ISSUE` |
| `post_review` | 審查結果處理完之後 | `AWK_PR`、`AWK_ISSUE`、`AWK_SCORE`、`AWK_DECISION` |
| `on_merge` | pull request 合併之後 | `AWK_PR`、`AWK_ISSUE` |

每個事件還會拿到 `AWK_EVENT`。每一項的鍵：

| 鍵 | 預設 | 說明 |
|---|---|---|
| `command` | 必填 | 要執行的指令 |
| `timeout` | 無 | 例如 `30s`、`2m` |
| `on_failure` | `warn` | 指令失敗時：`warn` 印出警告後繼續；`ignore` 不說話；`abort` 讓這一步停下來（只對 `pre_dispatch` 與 `pre_review` 有意義） |
| `env` | 無 | 額外的環境變數 |

---

## 名冊：`.ai/roster.yaml`

`awkit plan` 開會時坐在桌邊的角色。`awkit init` 依 preset 寫下第一份；這個檔案是你的，升級不會動它。名冊裡未知的鍵是錯誤，`awkit validate` 會檢查。

```yaml
language: ""           # 議會用什麼語言寫；空的時候跟需求的語言走
model: sonnet          # 沒有自己指定模型的角色用這個
limits:
  disputes: 5          # 最多辯幾個爭點；其餘直接交給決策角色
  dispute_rounds: 2    # 每個爭點來回幾次
  calls: 80            # 一次規劃最多幾次模型呼叫
  parallel: 3          # 同時幾個呼叫
  timeout_seconds: 600 # 每次呼叫的時間上限
roles:
  - id: producer
    title: Producer
    decides: true      # 由這個角色裁定談不攏的爭點（恰好一個）
    stance: |
      你為「做得完、值得做」負責：範圍、時程、砍什麼。
      反對不在核心循環裡的功能，也反對「順便做一下」。
    owns:
      - docs/product/vision.md
  - id: tech-lead
    title: Tech Lead
    plans: true        # 由這個角色擬票券清單（恰好一個）
    stance: |
      你為「做得出來、維護得動」負責。反對沒有驗收條件的需求，
      也反對還沒量過就說做不到。
    owns:
      - docs/tech/architecture.md
```

| 角色的鍵 | 說明 |
|---|---|
| `id` | 必填。小寫字母、數字與連字號。出現在會議紀錄與其他角色的發言裡 |
| `title` | 介紹這個角色時用的稱呼；沒寫就用 `id` |
| `stance` | 必填。這個角色主張什麼、反對什麼。立場要寫得會互相衝突：一桌人都點頭的規劃沒有被檢驗過 |
| `owns` | 這個角色擁有的文件，路徑在 `docs/` 下。一份文件只能有一個擁有者，而且只有擁有者會寫它 |
| `model` | 扮演這個角色的模型；沒寫就用名冊的 `model` |
| `decides`、`plans` | 各恰好一個角色為 `true`，可以是同一個角色 |

至少要有兩個角色。
