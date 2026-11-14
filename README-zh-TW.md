# AWK - AI Workflow Kit

[![CI](https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/silver2dream/ai-workflow-kit/badge)](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

[English](README.md) | [繁體中文](README-zh-TW.md)

AWK 把一句需求變成合併進去的 pull request。先由一桌模型扮演的角色（議會）把需求辯成文件與一份票券清單；你核可清單之後，**Principal**（Claude Code）把每張票交給 **Worker**（Codex 或 Claude Code）、送審 pull request、合併通過的。票券就是 GitHub issue；「通過」由程式檢查，不是模型說了算。

```
awkit plan "…"  ──►  文件 + 決策 + 票券提案  ──►  你核可
                                                    │
awkit run  ──►  下一張票 ──► Worker 寫變更 ──► 審查 ──► 合併
                    ▲                            │
                    └────── 帶著審查意見退回 ──────┘
```

## 需要什麼

| | |
|---|---|
| `git`，而且專案的 `origin` 是一個 GitHub repository | |
| [`gh`](https://cli.github.com/)，已登入且對該 repository 有寫入權限 | 工作流程就活在它的 issue 與 pull request 裡 |
| [`claude`](https://claude.com/claude-code)（Claude Code） | 扮演 Principal、審查者與議會 |
| `codex`，或再一次 `claude` | 扮演 Worker（`worker.backend`，預設 `codex`） |
| Windows：Git for Windows | Claude Code 需要它的 Git Bash，建置與測試指令也透過它執行 |

`awkit doctor` 會在專案裡把這些全部檢查一遍，並說缺了什麼。

## 安裝

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex
```

指令叫 `awkit`，避免和系統的 `awk` 撞名。

## 第一次使用

在要讓它工作的 repository 裡：

```bash
awkit init                       # 或：awkit init --preset godot-go --scaffold
git add -A && git commit -m "[chore] set up awkit" && git push
awkit doctor                     # 工具、remote、設定檔
awkit plan "每日登入獎勵：連續七天、獎勵遞增、漏一天重來"
awkit plan approve <plan>        # 建立 issue
awkit run                        # 一張一張做完
```

| Preset | 內容 |
|---|---|
| `generic`（預設） | 單一 repository、不限技術棧。建置與測試指令由你填進 `.ai/config.yaml`。 |
| `godot-go` | `backend/` 用 Go、`frontend/` 用 Godot C#，附兩邊的規則。加 `--scaffold` 會多一份能建置、測試會過的專案骨架。 |

`awkit init` 會寫下設定檔（分支用 repository 裡已有的）、`.claude/` 下的 kit、兩份指引（`CLAUDE.md`、`AGENTS.md`），以及每個有建置或測試指令的 repo 各一個 CI workflow。結尾會說它寫了什麼、接下來做什麼。

工作進行中：

```bash
awkit status     # run 的狀態、每張票在哪個階段、下一步
awkit stop       # 請 run 做完手上這一步之後停下
```

run 會在沒有票可做、或有事需要你的時候結束：Worker 每次嘗試都失敗、審查一再退回、某張票在等一個不會完成的 issue。它會說是哪一種，`awkit status` 會指出是哪張票。

[開始使用](docs/user/getting-started.md)把這幾步一步一步講完。

## 規劃：議會

```bash
awkit plan "每日登入獎勵：連續七天、獎勵遞增、漏一天重來"
```

`.ai/roster.yaml` 裡的角色把需求辯清楚：各自表明立場；互相提出異議；最要緊的異議由相關的兩個角色來回對辯；不選邊的主持人整理目前的狀況；決策角色裁定還沒談攏的部分。之後每個角色寫自己擁有的文件，規劃角色擬出票券，其他人可以再反對一次。

發言順序寫在程式裡，不在 prompt 裡：awkit 依序問每個角色、數回合與模型呼叫次數，而且只有它寫檔，所以一個角色寫不到別人擁有的文件。

| 一次規劃留下的東西 | |
|---|---|
| `docs/plans/<plan>/minutes.md` | 誰說了什麼：立場、異議、每個爭點與結果 |
| `docs/decisions.md` | 決策，各有一個不變的編號（`D7`）與不同意見；以及這次規劃的假設（`A3`）。只增不改 |
| `docs/<area>/*.md` | 文件，每份只由擁有它的角色寫 |
| `docs/plans/<plan>/tickets.yaml` | 票券提案，依施工順序 |

這時 GitHub 上還什麼都沒有。看過工作清單、改掉你想改的，再執行 `awkit plan approve <plan>`。

- **只有你答得出來的事。** 規劃不等人：角色需要你才有的答案時，決策角色會寫明這次規劃假設了什麼，結尾會把假設列出來。想被問的話，用 `awkit plan --ask "…"` 開始：議會最多問你七題，再從你的回答辯下去。
- **下一次規劃接著這一次。** 每個角色會讀已記錄的決策與自己現有的文件，規劃角色看得到之前的規劃開過哪些票。
- **名冊由你改：** 角色、各自的立場、擁有哪些文件、由哪個模型扮演、一次規劃最多幾次模型呼叫。中途停下的規劃用 `awkit plan --resume <plan>` 接續。

## 票券

票券是一個帶 `ai-task` 標籤的 GitHub issue，內文有一個小區塊，說明它給哪個 repo、在等哪些票。`awkit plan approve` 會建立它們；也可以自己寫一張：

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
repo: backend          # .ai/config.yaml 裡的 repo 名稱，或 root
depends_on: [12]       # 選填：必須先完成的 issue
```
````

```bash
awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md
awkit run --issue 42   # 只做這一張票
```

票券依編號由小到大處理，而且要等它依賴的 issue 都完成。區塊會被嚴格讀取：未知的欄位、設定檔裡沒有的 repo，在建立 issue 之前就會被拒絕。完整格式在 `.claude/skills/principal-workflow/references/ticket.md`。

## 「通過」是什麼意思

- **審查交的是表單，不是文章。** 審查者交一份結構化的 `review.json`；格式錯了在同一個 session 裡改，證據錯了才要重新審查。
- **證據會被檢查。** awkit 會把測試再跑一次，並確認每一條驗收條件都對得到一個真的跑過而且通過的測試。
- **CI 要是綠的**，而且分數與發現的問題要互相對得上。
- **你的話比審查大。** run 進行中到 GitHub 上審 pull request（要有該 repository 的寫入權限）：有你未解決的審查意見就不會合併，Worker 會帶著這些意見被退回，這些意見也會留成教訓。合併不等你；合併之後你才寫下的意見，會在下一張票開工前被讀進來。
- 選用：多個模型一起審並合併分數（`review.multi_model`）；審查時依 diff 產生測試（`review.jittest`）。

被審查退回的事，等變更終於通過之後，會被濃縮成一條短的教訓；之後的 Worker 與審查者動到同樣的檔案時會看到它。教訓存在 `.ai/state/lessons.json`，只在這份 checkout 裡、不進版控；`awkit lessons list` 可以列出來，`awkit lessons promote` 會把一條已證實的教訓變成一張要求訂規則的 issue。

## 它在你的專案裡放了什麼

```
.ai/
  config.yaml        設定檔：repo、分支、建置與測試指令
  roster.yaml        議會的名冊
  state/             每次 run 留下的東西：結果、log、教訓（git 忽略）
.claude/
  rules/_kit/        kit 的規則（分支、commit、pull request），產生的
  rules/             專案自己的規則；godot-go preset 會先給你三份
  skills/            Principal 照著走的流程：主迴圈、票券格式
  agents/            它把審查與衝突交出去的 subagent
CLAUDE.md, AGENTS.md   告訴 agent 的事，由設定檔產生
docs/                  議會寫的東西：文件、decisions.md、plans/
.github/workflows/     CI，每個 repo 一個 workflow，由設定檔產生
```

CI workflow 會跟著設定檔走，直到你刪掉它開頭那行註解；之後就是你的。你自己的規則、skill 與本機的 Claude Code 設定不會被覆寫。

## 指令

| | |
|---|---|
| `awkit init` | 設定一個專案 |
| `awkit doctor` | 檢查 run 需要的所有東西，並說缺的怎麼補 |
| `awkit generate`、`awkit validate` | 改過設定檔之後：重新產生指引與 CI workflow；檢查設定檔 |
| `awkit plan` | 與議會一起規劃；`awkit plan approve <plan>` 建立票券 |
| `awkit create-task` | 手動補一張票 |
| `awkit run` | 處理所有開著的票（`--issue N` 只做一張，`--dry-run` 只跑檢查） |
| `awkit status` | 工作進行到哪裡 |
| `awkit stop` | 請 run 停下 |
| `awkit events`、`awkit lessons` | run 做了什麼；審查教了什麼 |
| `awkit upgrade` | 把專案裡的 kit 檔案更新到已安裝的版本 |
| `awkit reset` | 清掉之前的 run 留下的東西 |
| `awkit uninstall` | 把 AWK 從專案移除 |

`awkit help <指令>` 有更多說明；`awkit help --all` 會列出工作流程自己呼叫的指令。

## 設定

`.ai/config.yaml`。新專案的設定檔就這麼短，沒寫的鍵用預設值：

```yaml
version: "1.2"

project:
  name: "demo"
  type: "monorepo"              # 或 single-repo

repos:
  - name: backend
    path: backend/
    type: directory             # root | directory | submodule
    language: go
    verify:                     # Worker 與審查都會跑
      build: "go build ./..."
      test: "go test ./..."

git:
  integration_branch: "develop" # pull request 合併進這裡
  release_branch: "main"        # 只有一條主幹的專案，兩者是同一個分支

rules:
  custom: [backend]             # .claude/rules/backend.md

worker:
  backend: codex                # 或 claude-code

review:
  score_threshold: 7
  merge_strategy: squash
```

改完之後執行 `awkit generate`，指引與 CI workflow 會重新產生；`awkit validate` 檢查這份檔案，也會列出檔案裡沒有任何程式會讀的鍵，免得一個沒有作用的設定被當成有作用。每個鍵的說明在 [docs/user/configuration.md](docs/user/configuration.md)。

## 升級

```bash
awkit check-update     # 有沒有新版 awkit
awkit upgrade          # 把這個專案的 kit 檔案更新到已安裝的 awkit
```

`awkit upgrade` 會保留你的設定檔、規則與 skill，並把它改到的東西 commit 起來。舊版 awkit 建立的專案把檔案放在別處（`.ai/config/workflow.yaml`、`.ai/rules/`、`.ai/skills/`）；升級會把它們搬過來，`awkit upgrade --dry-run` 可以先看每一步。

## 文件

| | |
|---|---|
| [開始使用](docs/user/getting-started.md) | 第一次使用，一步一步來 |
| [設定檔](docs/user/configuration.md) | `config.yaml` 的每個鍵，以及名冊 |
| [Skills 與規則](docs/user/skills.md) | 告訴 agent 的事，以及怎麼加自己的 |
| [疑難排解](docs/user/troubleshooting.md)、[FAQ](docs/user/faq.md) | 東西不動的時候 |
| [架構](docs/developer/architecture.md)、[貢獻](docs/developer/contributing.md) | awkit 本身怎麼做的 |

## 授權與安全

[Apache License 2.0](LICENSE)。弱點回報方式見 [SECURITY.md](SECURITY.md)；本 repository 由 [OpenSSF Scorecard](https://securityscorecards.dev/viewer/?uri=github.com/silver2dream/ai-workflow-kit) 監看。
