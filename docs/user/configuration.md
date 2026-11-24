# 設定檔

`.ai/config.yaml` 是專案對 AWK 說的話：有哪些 repo、往哪個分支合併、用什麼指令建置與測試。`awkit init` 寫下第一份。

- 改過之後執行 `awkit generate`：`CLAUDE.md`、`AGENTS.md`、`.claude/rules/_kit/` 與 CI workflow 都是從這份檔案產生的。
- 常改的部分也可以在工作台（`awkit ui`）的「設定」頁改：它會連同 `awkit generate` 產生的檔案開成 PR，CI 過了就合併，見[工作台](workbench.md#設定)。
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
| `custom` | `[]` | 專案自己的規則。名稱 `backend` 對應 `.claude/rules/backend.md`；列在這裡，`AGENTS.md` 才會列出它與它適用的檔案，Worker 才知道哪些票要讀它。新專案這裡列的是 `awkit init` 寫下的起始規則。檔案不存在時 `awkit generate` 會警告 |

寫規則的方式見 [Skills 與規則](skills.md)。

## `agents`

| 鍵 | 預設 | 說明 |
|---|---|---|
| `builtin` | `[pr-reviewer, conflict-resolver]` | kit 的 subagent |
| `custom` | `[]` | 專案自己的 subagent，`awkit generate` 會把每一個寫成 `.claude/agents/<name>.md` |
| `reviewer.backend`、`reviewer.model`、`reviewer.effort`、`reviewer.minutes`、`reviewer.usd` | `claude-code`；模型與執行者不同（執行者是 Claude Code 的 `opus` 時用 `sonnet`，否則用 `opus`）；20 分鐘；不限金額 | engine 的審查者（`awkit run` 用它；`AWKIT_ENGINE=principal` 時不用）。它只能讀檔，不能執行指令；`minutes` 最多 120。換 backend 時模型與 effort 從那個 backend 的預設開始 |
| `resolver.backend`、`resolver.model`、`resolver.effort`、`resolver.minutes`、`resolver.usd` | 跟執行者一樣（`worker.backend` 與 Claude Code 的模型、effort）；20 分鐘；不限金額 | engine 的解衝突者（`awkit run` 用它；`AWKIT_ENGINE=principal` 時不用）：在 rebase 停住的 worktree 裡只改衝突的檔案，engine 檢查後才繼續、push。`minutes` 最多 120 |

這兩組設定超出角色允許的範圍（例如審查者設了 backend 沒有的值、`minutes` 超過上限）時，engine 不會啟動，並說明是哪一項。

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
| `backend` | `codex` | 誰來寫程式：`codex` 或 `claude-code`。對應的 CLI 必須裝好並登入，`awkit doctor` 會檢查。`claude-code` 需要支援 `--settings`、`--setting-sources` 與 `dontAsk` 權限模式的 Claude Code：Worker 只帶 awkit 為它寫的設定執行（見下方說明），太舊的 CLI 不會被執行 |
| `codex.full_auto` | `true` | 用 `--full-auto` 執行 codex |
| `claude_code.model` | `sonnet` | Claude Code 當 Worker 時用的模型 |
| `claude_code.effort` | 不設 | Worker 想得多深：`low`、`medium`、`high`、`xhigh` 或 `max`，越高越仔細，也越慢、越花錢。不設就用 claude CLI 自己的預設。其他值是錯誤：CLI 遇到不認得的值只會警告、照預設跑，打錯了不會有人發現。CLI 太舊、沒有 `--effort` 時照常執行，並提醒一次 |
| `claude_code.max_turns` | `50` | 一次嘗試最多幾輪 |
| `claude_code.dangerously_skip_permissions` | `false` | 以 `--dangerously-skip-permissions` 執行：Claude Code 不再問、也不再擋權限設定裡沒允許的工具。awkit 的工具檢查（下方說明）照樣執行。只在隔離的機器上打開 |
| `knowledge_graph` | `auto` | `auto`：專案有 `.understand-anything/knowledge-graph.json` 時，把和這張票有關的一小段放進 Worker 的 prompt。`off`：不放 |

**Worker 能做什麼。** Claude Code 當 Worker 時，只帶 awkit 為那一次寫的設定執行：不讀你的、也不讀專案的 Claude Code 設定；寫程式要用的工具直接允許，其他的不問就拒絕；每一個工具呼叫都先經過 `awkit guard tool` 檢查。它不能用 `gh`、不能 push 或 commit（那是 runner 的事）、不能連外（`curl`、`wget`、網頁工具）、不能讀放憑證的地方（`~/.ssh`、gh 與 Claude Code 的登入資料）、不能印出環境變數，也不能改 `.git/`；`.claude/` 與 `.ai/` 只有工單設了 `allow_script_changes: true` 才能改。兩種 backend 都只拿到建置與工具鏈需要的環境變數，以及它自己的登入變數：`GH_TOKEN` 這類 token 不會給它。被擋下的呼叫寫進事件（`awkit events --all --type guardrail_verdict`）。

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

新的教訓先以「未驗證」的身分試用，prompt 裡會這樣標明。濃縮出來的教訓在交給任何 Worker 之前都會先過篩檢，沒過的會被隔離，等你在工作台的「教訓」頁或用 `awkit lessons approve`、`reject` 決定（見[疑難排解](troubleshooting.md#被-guardrail-擋下blocked)）。

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

`awkit plan` 開會時坐在桌邊的角色。`awkit init` 依 preset 與專案的 repo 寫下第一份：每個角色負責一個資料夾，工程的資料夾跟著各 repo（`<repo>/docs/`；整個 repository 就是一個 repo 時是 `docs/engineering/`），和專案起始的規則說的位置一致。這個檔案是你的，升級不會動它。名冊裡未知的鍵是錯誤，`awkit validate` 會檢查。

```yaml
language: ""           # 議會用什麼語言寫；空的時候跟需求的語言走
model: sonnet          # 沒有自己指定模型的角色用這個
effort: high           # 沒有自己指定的角色想得多深；不寫就用 claude CLI 的預設
limits:
  topics: 8            # 議程最多幾個議題
  disputes: 2          # 一個議題最多辯幾個爭點；其餘直接交給決策角色
  dispute_rounds: 2    # 每個爭點來回幾次
  calls: 200           # 一次規劃最多幾次模型呼叫
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
      - docs/product/
  - id: tech-lead
    title: Tech Lead
    plans: true        # 由這個角色擬票券清單（恰好一個）
    stance: |
      你為「做得出來、維護得動」負責。反對沒有驗收條件的需求，
      也反對還沒量過就說做不到。
    owns:
      - backend/docs/
      - frontend/docs/
```

| 角色的鍵 | 說明 |
|---|---|
| `id` | 必填。小寫字母、數字與連字號。出現在會議紀錄與其他角色的發言裡 |
| `title` | 介紹這個角色時用的稱呼；沒寫就用 `id` |
| `stance` | 必填。這個角色主張什麼、反對什麼。立場要寫得會互相衝突：一桌人都點頭的規劃沒有被檢驗過 |
| `owns` | 這個角色擁有的文件，路徑從專案根目錄寫起：一個資料夾（結尾加 `/`，例如 `docs/design/`、`backend/docs/`），角色在裡面寫、也可以開新檔；或一份檔案（`docs/qa/risks.md`）。文件是 Markdown 檔，或遊戲資料的表（`.csv`）與它的 schema（`.xml`），見[遊戲資料](#遊戲資料數值與文案)。一份文件只能有一個擁有者，而且只有擁有者會寫它：兩個角色不能列同一個路徑。一個角色的資料夾裡可以有別的角色列出的資料夾或檔案，那一部分就是別人的（例如 `gamedata/Datas/` 是數值企劃的，裡面的 `gamedata/Datas/text/` 是文案的）；同一個角色不能把自己資料夾裡的東西再列一次。不能是議會自己維護的檔案（`docs/decisions.md`、`docs/README.md`、`docs/plans/`，所以 `docs/` 整個不能當資料夾）、給 agent 的指示（`CLAUDE.md`、`AGENTS.md`），或以 `.` 開頭的目錄（`.claude/`、`.ai/`、`.github/`） |
| `model` | 扮演這個角色的模型；沒寫就用名冊的 `model` |
| `effort` | 這個角色想得多深：`low`、`medium`、`high`、`xhigh` 或 `max`；沒寫就用名冊的 `effort`，名冊也沒寫就用 claude CLI 的預設。越高越仔細，也越慢、越花錢：負責裁定、反覆推敲的角色比較值得調高。其他值會讓名冊無法使用 |
| `decides`、`plans` | 各恰好一個角色為 `true`，可以是同一個角色 |

至少要有兩個角色。

### 會議怎麼開

1. **議程**：主持人（用名冊的 `model` 與 `effort`，不選邊）讀需求、名冊與現有的文件，排出議程，存在 `docs/plans/<plan>/agenda.yaml`。每個議題寫明：要回答什麼（`question`）、誰主講（`lead`）、誰依序發言（`speakers`）、先讀哪些文件（`reads`）、結束時寫進哪些文件（`writes`）。寫文件的角色一定在發言名單裡。`reads` 也可以是專案裡已經在的文字檔，例如設定或契約（`gamedata/luban.conf`、`api/match.proto`）；以 `.` 開頭的目錄裡的檔案不行。
2. **只有你答得出來的問題**：議程也列出這些問題。用 `awkit plan --ask` 時，第一個議題開始前會問你；沒問的話，決策角色在用到時寫下假設。
3. **逐議題**：主講者先讀文件、提出方案；其他角色照順序發言，每個人都看得到前面的人說了什麼，看到不能接受的就提出異議；兩個角色有分歧時直接對辯（`dispute_rounds` 回合）；主持人整理；決策角色裁定，寫進 `docs/decisions.md`；擁有者把結論寫進 `writes` 裡自己的文件（照下一節的規則）。下一個議題從這些文件開始。決策只記「文件要照著做、而且當初可以有別的選擇」的事，一個議題最多 8 條決策、4 條假設；大家同意的細節由擁有者寫進文件。票券長什麼樣、要哪些欄位，不是會議決定的事。
4. **工作清單**：規劃角色把整個計畫排成里程碑（每個都是看得到在運作的東西），只為第一個里程碑擬票；其他角色可以反對一次。其餘里程碑等第一個做完再規劃。

議程是你的：想改議題、順序或發言的人，停下來改 `agenda.yaml`，再用 `awkit plan --resume <plan>` 接續。已經開過的議題維持原樣；要重開一個議題，給它新的 `id`。議程需要的呼叫次數超過 `limits.calls` 時，第一個議題開始前就會停下來告訴你。

會議紀錄：`docs/plans/<plan>/minutes.md` 是總表（議程、問答、決策、文件、里程碑、票），每個議題另有一份 `docs/plans/<plan>/topics/<編號>-<id>.md`（誰說了什麼、對辯、主持人的整理、裁定、寫了哪些文件）。

### 文件照專案的規則寫

議會寫文件時遵守 `.claude/rules/` 裡的規則，不是 awkit 內建的格式：

1. 每個角色拿到「對它要寫的檔案生效」的規則全文：`paths` 符合那些檔案的規則，加上沒有 `paths` 的規則。判斷方式與 Claude Code 載入規則的方式相同。AWK 自己出貨的規則（`.claude/rules/_kit/`）講的是 git 的做法，不在其內。
2. 寫完之後，由一個不選邊的讀者（用名冊的 `model` 與 `effort`）拿同一批規則逐份對照，列出文件與規則不一致的地方。
3. 有不一致時，擁有者修一次。要搬到別人文件裡的內容，擁有者從自己的文件拿掉並指向那份文件。
4. 讀者再回答一件事：剛才列的那幾點，哪些還在。它不重新整份檢查（每次重看都會找到別的，收不了斂）。
5. 還在的幾點不再來回：記在會議紀錄的 `By the project's rules` 一節，`awkit plan` 結束時也會列出來，由你決定改文件還是改規則。

文件放哪裡、檔頭寫什麼、可以宣稱什麼狀態、超過多長要拆，都由規則決定（起始的那一份是 `docs-source-of-truth.md`）。專案沒有對這些檔案生效的規則時，文件只以標題與一行擁有者、日期開頭，也不做對照。

規則全文合計超過 60000 位元組時，先放有指名這些檔案的規則，再放通用的；放不下的規則只列出檔名，會議紀錄會寫明哪些沒有傳給角色。只對照這次會議寫出的文件；擁有者維持原樣的文件不在其內。

每個角色在每一次發言前都看得到名冊裡全部的文件、位置與擁有者。有哪些文件、放在哪裡、由誰寫，不是會議可以決定的事：要改就改名冊。

### 遊戲資料：數值與文案

遊戲讀的數值與玩家看到的字，不寫在文件裡，寫在遊戲資料：Luban 讀的表。awkit 從專案根目錄下哪個資料夾有 `luban.conf` 知道它在哪裡；`awkit init --preset godot-go --scaffold` 會寫出 `gamedata/`：

```
gamedata/
  luban.conf         groups：c（客戶端）、s（伺服器）
  Defines/*.xml      schema：每張表的欄位、型別、group、白話說明
  Datas/*.csv        資料：一張表一個檔
  Datas/text/*.csv   文字表：key、zh_TW、zh_CN、en、_en_依據、_上限
```

規矩在起始規則 `gamedata.md`：數字不准空白（Luban 會讀成 0）、group 一律標、機率與權重只給伺服器、玩家看得到的字只放文字鍵、繁中是原文、英文過期要重看。

名冊裡，數值企劃擁有 `gamedata/Defines/` 與 `gamedata/Datas/`，文案擁有 `gamedata/Datas/text/`。一個議題要寫表與文字時：

1. 寫表的角色先寫；寫文字的角色後寫，並被告知剛寫好的表用到哪些文字表還沒有的鍵。
2. 都寫完之後，awkit 檢查遊戲資料（和 `awkit gamedata check` 同一套）。找到的問題交回該檔的擁有者修一次；文字表缺的鍵交給文案。
3. 修過還找得到的，記在議題紀錄的 `The check of the game data` 一節，`awkit plan` 結束時也會列出來。

給人審的兩頁放在計畫的資料夾：`docs/plans/<plan>/copy.md`（這次寫的文字，各語言並排、每個鍵用在哪張表哪一列、要注意的地方）與 `tables.md`（這次寫的表，每一欄的說明、文字鍵旁附原文）。它們從檔案產生，不手改；要改就改表。

隨時都可以檢查與審閱：

```bash
awkit gamedata check            # 沒過時結束碼是 1
awkit gamedata review copy      # 整份文字表，給人審
awkit gamedata review tables --out review/tables.md
```

Luban 本身（版本、生成伺服器端與客戶端的程式碼、接進 CI）由第一個里程碑的票做；在那之前，沒有程式讀得到這些資料。
