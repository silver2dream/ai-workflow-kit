# 架構

`awkit` 本身怎麼做的。對象是要改 AWK 的人；用 AWK 的人看 [開始使用](../user/getting-started.md) 就夠了。

---

## 一句話

**模型做判斷，程式做決定。** 寫程式、審查、辯論交給模型；「下一步做什麼」「這算不算通過」「能不能合併」由 Go 程式依 GitHub 上的狀態與實際跑出來的結果決定。模型交回來的是表單，不是結論。

派工的是 awkit 自己的 engine（Go 程式）；模型扮演的角色都是外部的 CLI：

| 角色 | 誰扮演 | 做什麼 | 不做什麼 |
|---|---|---|---|
| Engine | awkit（`awkit run` 預設） | 依線的狀態決定下一步，派工、送審、解衝突、合併，把線投影成標籤 | 不寫程式、不審查內容 |
| Worker | `codex` 或 `claude`（`worker.backend`） | 在自己的 worktree 裡改檔案、跑驗證 | 不碰 git、不開 pull request（runner 做） |
| Reviewer | engine 啟動的唯讀審查者（`agents.reviewer`，預設 Claude Code） | 讀 diff 與 engine 跑的測試結果，交一份結構化的審查 | 不決定合併（`reviewer.SubmitReview` 驗證之後決定；engine 的路上，通過的進合併佇列） |
| Resolver | engine 啟動的解衝突者（`agents.resolver`） | 在 rebase 停住的 worktree 裡只改衝突的檔案 | 不 commit、不 push（engine 檢查後才做） |

`AWKIT_ENGINE=principal` 改用原本的主控：一個 Claude Code session 照 `principal-workflow` skill 的迴圈把 awkit 的指令接起來，審查與解衝突交給 `pr-reviewer`、`conflict-resolver` subagent。這個做法保留一個版本，下一版移除。

規劃是另一條線：`awkit plan` 由程式主持一場會議，每個角色只回答一個問題、交回一段文字；寫檔的只有程式。

## 兩條流程

```
awkit plan "…"
  └─ council.Run：主持人排議程（agenda.yaml；--review-agenda 時停下等擁有者看）→ 問擁有者（--ask，或 --ask-in-workbench 停下等回答）→ 逐議題 → 里程碑與第一個里程碑的票
     每一步之間的停點（point）：要暫停就停；擁有者說的話在這裡收進來，已有裁定時決策角色先重審全部決策、受影響文件整份重看（review.go）
       每個議題：主講提方案 → 其他角色依序發言、提異議 → 對辯 → 主持人整理 → 裁定（寫進 decisions.md）→ 擁有者寫文件
       文件：每個角色照對它的檔案生效的規則寫（internal/rules）→ 對照規則檢查 → 擁有者修一次 → 讀者回答哪幾點還在
       遊戲資料（有 luban.conf 時，internal/gamedata）：先寫表、再寫文字（告知缺的鍵）→ 檢查 → 擁有者修一次 → 還找得到的記錄
       寫出各角色的文件、docs/decisions.md、docs/plans/<plan>/{minutes.md,tickets.yaml}；寫了遊戲資料時加 copy.md、tables.md
awkit plan approve <plan>
  └─ council.Approve → task.CreateTask：每張票一個 GitHub issue（ai-task 標籤 + awk-ticket 區塊）

awkit run
  ├─ kickoff：啟動前的檢查（PreflightChecker.steps）、lock、建立標籤、清掉上次的 STOP 標記
  └─ engine.Serve：run 檔的心跳（同時在跑的每一步）、收件匣的停止指令；迴圈（Engine.Run）一輪一輪做，
     開始時與每有一步做完時各一輪：
       Reconcile                 → 人在 GitHub 上做的事轉成線的轉換、匯入新票（正在跑的線不碰）
       Precheck（analyzer）       → 停止、輪數、連續失敗
       合併佇列的第一張 → 手上的線 → Candidates（analyzer） → 能開始的都開始：最多 engine.max_parallel 步、
                                    各 backend 最多 engine.max_per_backend 步；合併一次一個，不佔名額
       每一步在自己的 goroutine 裡跑，做完交回迴圈：
         ├─ dispatch_worker → worker.DispatchWorker → worker.RunIssue（實作者，各自的程序）
         ├─ check_result    → worker.CheckResult
         ├─ review_pr       → ReviewStep：engine 跑測試 → 唯讀的 reviewer → reviewer.SubmitReview（驗證；通過就核准、進合併佇列，或退回）
         ├─ merge_pr        → MergeStep：rebase → 不是核准的 commit 就重驗（diff 政策、測試）→ push → 等 CI
         │                    → 維護者的審查 → gh pr merge --match-head-commit → reviewer.AfterMerge
         ├─ dispatch_worker（解衝突）→ ResolveStep：engine rebase → resolver 改檔 → engine 檢查、push（佇列來的回佇列）
         ├─ none（把一條線交給人）→ 只轉那條線，這一輪接著決定
         └─ none / all_complete（整輪停止，或沒有別的可做）→ workflow.StopWorkflow（寫 STOP 標記與報告；還有線等人時原因是「需要你」）
       每個轉換寫進事件（events/engine.jsonl），再把工單的標籤改成線的投影
       AWK 自己下的 git 指令都先拿 repo 的鎖（gitlock），各程序的 Worker 不會同時改主 repo 的 refs 與設定

AWKIT_ENGINE=principal awkit run（保留一個版本）
  └─ 迴圈：啟動一個 Principal session（claude），結束後看 STOP 標記；沒有就再開一個
       Principal 在 session 裡重複：
         awkit analyze-next        → next_action（analyzer.Decide）
         ├─ dispatch_worker → awkit dispatch-worker   （worker.DispatchWorker → worker.RunIssue）
         ├─ check_result    → awkit check-result      （worker.CheckResult）
         ├─ review_pr       → pr-reviewer subagent
         │                      awkit prepare-review   （reviewer.PrepareReview）
         │                      awkit submit-review    （reviewer.SubmitReview：驗證、合併或退回）
         └─ none / all_complete → awkit stop-workflow <reason>（寫 STOP 標記與報告）
```

`awkit run` 最後用 `workflow.EndingFor(reason)` 把停止原因講成人話；工作做完或人要求停下是結束碼 0，其餘是 1。

## 狀態在哪裡

| 狀態 | 放在哪 | 誰寫 |
|---|---|---|
| 有哪些工作、各走到哪一步 | engine 的線：轉換在 `.ai/state/events/engine.jsonl`，快照在 `.ai/state/engine/lanes/`；GitHub 的標籤是線的投影 | `engine`（主控模式下是 `worker`、`reviewer`、`analyzer` 直接改標籤） |
| engine 的 run 與收件匣 | `.ai/state/engine/run.json`、`inbox/`、`outbox/` | `engine` |
| 一張票的結果、prompt、log | `.ai/state/results/`、`runs/issue-N/`、`logs/` | `worker` |
| 發生過的每件事 | `.ai/state/events/`（`awkit events` 讀） | `trace` |
| 迴圈次數、連續失敗次數、STOP 標記、lock | `.ai/state/` | `analyzer`、`worker`、`stopmark`、`kickoff` |
| 審查紀錄與教訓 | `.ai/state/review_feedback.jsonl`、`lessons.json` | `reviewer`、`lessons` |
| 一場規劃的每次模型呼叫，以及會議走到哪（`live.json`）、逐句紀錄（`transcript.jsonl`）、整理成資料的紀錄（`record.json`）、擁有者說的話（`owner.jsonl`）與暫停、議程、提問的標記、核可走到哪（`approval.json`） | `.ai/state/council/<plan>/` | `council`；擁有者的話與標記由 `awkit plan say`、`pause` 與工作台寫 |

engine 當掉重開，線從事件重建，接著做手上那一步。主控模式下 Principal 的對話也不是狀態：session 隨時可以結束再開，下一個 session 從 `awkit analyze-next` 重新得知一切。`.ai/state/` 整個被 git 忽略。

標籤就是狀態機：

| 標籤 | 意思 | 接下來 |
|---|---|---|
| `ai-task` | 一張票；沒有其他標籤時可以派工 | 派工 → `in-progress` |
| `in-progress` | Worker 在做 | 開了 pull request → `pr-ready`；失敗 → 拿掉標籤、下一輪重派，第三次失敗 → `worker-failed`；不需要改任何東西 → `completed` |
| `pr-ready` | 等審查 | 通過且 CI 通過 → 合併、關閉 issue；要求修改 → 回到只有 `ai-task`，Worker 帶著意見重做；證據不符或 CI 沒過 → `review-failed`；合併時撞到衝突 → `merge-conflict` 或 `needs-rebase` |
| `review-failed` | 審查要重來一次 | → `pr-ready`；次數達到 `escalation.max_review_attempts` → `needs-human-review` |
| `merge-conflict`、`needs-rebase` | pull request 與 integration branch 衝突或落後 | 派 Worker 處理（`--merge-issue`）→ `pr-ready`；處理不了 → `needs-human-review` |
| `worker-failed`、`needs-human-review` | 等人 | 人處理之後拿掉標籤，或 `awkit reset --labels` |

標籤名稱是固定的（`config.DefaultLabels`）。`analyzer` 與 `status` 從設定讀名稱，`worker` 與 `reviewer` 直接寫預設名稱，所以設定檔改名會被 `config.Validate` 拒絕。要讓名稱可以設定，得先把後者約五十處改成讀設定。

## 「通過」怎麼判定

`awkit submit-review`（`internal/reviewer`）依序檢查，任何一項不過都不合併：

1. **表單**：`review.json` 的格式（`structured.go`）。格式錯誤以結束碼 2 回報，reviewer 在同一個 session 裡修正。
2. **證據**（`evidence.go`）：重跑 `verify.test`，確認每條驗收條件對到的測試真的出現在輸出裡而且通過。不符是 `review_blocked`，要重新審查，不能在同一個 session 裡自己改。
3. **分數與發現的問題一致**（`review.severity_consistency`）。
4. **多模型共識**（`review.multi_model`，選用，`multi.go`）。
5. **JiT 測試**（`review.jittest`，選用，`internal/jittest`）。
6. **人的審查意見**（`human_feedback.go`）：有寫入權限的人在目前這個 commit 上留下、還沒解決的意見會擋住合併，並原文交回給 Worker。合併不等人；合併之後才出現的意見由 `late_feedback.go` 在下一次派工前讀進來。
7. **CI**：`prepare-review` 讀到的 CI 狀態不是 `passed` 就不合併。

通過之後 `gh pr merge`（`review.merge_strategy`）、關閉 issue、清掉 worktree，並結算學習迴圈。

## Worker 的一次派工

`awkit dispatch-worker`（`worker.DispatchWorker`）等 Worker 做完才返回，最多 60 分鐘（`worker.DefaultWorkerTimeout`；`awkit run` 把 Principal 的 shell 逾時設成比它多 5 分鐘）。runner（`runner.go`）做的事：

1. 讀 issue 的 `awk-ticket` 區塊（`internal/ticket`），找出 repo 與它的驗證指令。
2. 在 `.worktrees/issue-N` 建 worktree，從 integration branch 切出 `feat/ai-issue-N`。
3. 執行 `verify.setup`（安裝相依套件），再組 prompt：票的內文、審查意見（重做時）、相關的教訓、knowledge graph 的片段。
4. 啟動 backend（`backend_codex.go`、`backend_claude.go`），失敗時依 `escalation.retry_count` 重試。
5. 安全檢查（`security.go`）：受保護的路徑、敏感字串、submodule 邊界。票的區塊可以逐項放行。
6. 執行建置與測試指令，commit（`[type] subject`，`commit.go`），push，開或更新 pull request。setup、建置、測試各有 `timeouts.verify_minutes` 的時間上限；超過就連同它啟動的行程一起停掉（`util.RunWithChildren`）。
7. 寫結果檔；`check-result` 與 `analyzer` 依它改標籤。

## 套件

`cmd/awkit` 一個指令一個檔，只做旗標解析與輸出；邏輯在 `internal/`。

| 套件 | 管什麼 |
|---|---|
| `config` | `.ai/config.yaml` 唯一的讀取處：結構、預設值（`applyDefaults`）、驗證、沒有程式讀的鍵（`UnreadKeys`） |
| `layout` | 專案裡每個檔案的位置。路徑只在這裡寫一次 |
| `install` | `init`、`upgrade`、`uninstall`：preset（`presets.go`）、起始規則（`starter-rules/`，範本，用設定檔與 owner 的回答寫出來）、專案骨架、kit 檔案的安裝與汰換 |
| `rules` | 讀專案的規則檔（`.claude/rules/`）：標題、它適用的檔案（`paths:`）、內文；以及哪些規則對某個檔案生效（`For`、`Match`，語法照 Claude Code） |
| `generate` | 從設定產生的檔案：`CLAUDE.md`、`AGENTS.md`、kit 的 git 規則、自訂 subagent、CI workflow、Claude Code 的本機權限 |
| `migrate` | 設定檔格式的版本轉換，以及舊目錄結構的搬遷 |
| `council` | `awkit plan`：名冊、會議的每一步、文件、票券清單、核可前的修改與核可；會議中擁有者的話、暫停、議程與提問（owner.go）、重審（review.go）、給外部讀的即時狀態與紀錄（live.go、record_json.go）、核可走到哪的紀錄（approval.go） |
| `landing` | 把替人做的修改送進整合分支：`awkit plan approve --merge-docs` 的議會文件（從工作目錄複製），與 `awkit settings apply` 的設定（`Edit`：直接改 worktree 裡整合分支版本的檔案）。在 `.ai/state/tmp/` 另開的 worktree 裡從 origin 的整合分支開分支、commit、push、開 PR、等 checks、合併；之後只在工作目錄位在整合分支、每個檔案都和合併進去的一致時，才把工作目錄快轉到整合分支。不繞過分支保護 |
| `settings` | 工作台設定頁與 `awkit settings apply`：名冊與設定檔中人會改的部分。改 YAML 節點而不重寫檔案（註解、空行、引號、頁面不管的鍵都留著）；寫之前用議會與 `config` 自己的檢查，以及 engine 的角色設定（`agent.FromConfig`）驗過；以頁面讀到的版本做三方合併，整合分支上別人同時改的欄位不被蓋掉 |
| `gamedata` | 遊戲資料（Luban）：從 `luban.conf` 找位置、讀 schema、表與文字表；Luban 擋不住的檢查；給人審的文案頁與表頁。`awkit gamedata` 與議會共用 |
| `ticket`、`task` | `awk-ticket` 區塊的讀取與驗證（schema 2 的 `paths` 與 `risk`、路徑的 glob 與交集；schema 1 照讀）；建立票券 |
| `kickoff` | `awkit run`：啟動前的檢查、lock、PTY、log |
| `analyzer` | 決策引擎：從 GitHub 的狀態決定下一步，計數迴圈與連續失敗 |
| `engine` | 取代 LLM 主控的確定性 engine（[Agent Engine](specs/agent-engine/README.md) P1；`awkit run` 預設由它跑）：線的狀態機（`lane.go`）、狀態與 GitHub 標籤的對照（`labels.go`，投影與讀回）、事件與快照（`journal.go`，事件在 `events/engine.jsonl`，快照在 `.ai/state/engine/lanes/`）、審查步驟（`review.go`：自己跑測試、組 reviewer 的 context、唯讀的 reviewer、經 `reviewer.SubmitReview` 做決定，回傳線的轉換）、解衝突步驟（`resolve.go`：engine 自己 rebase 與 push，resolver 只改衝突檔案，engine 檢查標記與範圍）、主人的核可（`approval.go`：高風險的改動停在 `awaiting_approval`，核可或退回經收件匣，`awkit lane approve|reject`）、合併佇列（`merge.go`：審查通過的線依進佇列的先後一次合併一個；rebase、重驗不是核准的 commit、push、等 CI、合併前再看維護者的審查、只合併驗過的 commit）、主迴圈（`engine.go`：依線決定下一步，順序照 analyzer；需要人的線只停它自己；投影標籤、轉人留言；每一輪先對帳，人在 GitHub 上做的事轉成線的轉換）、排程（`schedule.go`：哪些線佔名額、哪些佔住工單的 `paths`、依賴繞成的圈；可以開工的工單問 analyzer 的 `Candidates`，跳過路徑被佔住的）、多條線同時跑（`parallel.go`：一輪一輪做，能開始的步驟都開始、各在自己的 goroutine 裡跑；`engine.max_parallel`、`engine.max_per_backend`，合併一次一個、不佔名額；Worker 的輸出每行帶工單編號）、實作者步驟（`implement.go`：P1 沿用 worker）、GitHub 讀寫（`github.go`）、當作專案的 run 執行（`process.go`：心跳與同時在跑的每一步寫進 `.ai/state/engine/run.json`）、指令的收件匣與回覆（`inbox.go`）、影子（`shadow.go`：每一步讓主控的 analyzer 對同一份狀態只算不做，比對兩邊的決定，寫 `engine_shadow` 事件；`awkit status` 顯示最近一次的比對）、給別人讀的狀態（`view.go`：`Look` 把 run 檔、線、上限、最近的 guardrail 判定、影子比對與指令的回覆組成一份，工作台的執行頁與 `awkit status` 讀同一份）。`awkit run` 預設由它跑，`AWKIT_ENGINE=shadow` 另外開著影子，`AWKIT_ENGINE=principal` 改用主控（保留一個版本，`cmd/awkit/run_engine.go`） |
| `agent` | engine 的 agent runtime（engine 的審查者與解衝突者經由它跑）：角色設定的預設與上限（`role.go`）、從設定檔解出三個角色的設定（`config.go` 的 `FromConfig`：engine 啟動、`awkit validate` 與設定頁存檔前都用它）、`Run(ctx, envelope) → Result`、schema 核對與重問、逾時連同子程序一起結束、每次派工的 `agent_run` 事件（`runtime.go`）、Claude Code 與 Codex 的轉接器（`claude.go`、`codex.go`）、每個角色的最少 context、依來源推導的信任等級、遮蔽、資料區塊與信封存檔（`context.go`） |
| `worker` | 派工、runner、backend、安全檢查（受保護路徑、密鑰、工單 `paths` 的範圍）、結果檢查 |
| `lanenote` | 主人對一張工單說的話（退回高風險改動的理由，2.6 起補充說明）：engine 寫、Worker 每次派工以 owner 等級讀 |
| `guard` | guardrail 共用的部分：判定（`Verdict`，寫成 `guardrail_verdict` 事件）、帶來源與信任等級的 context 片段、清理看不見的字元、規則層注入偵測（`injection.go`，含中英文；語料在 `testdata/injection/`）、把不可信的片段包成資料區塊。Worker 的 prompt 由它組成，被擋下的票不派出去（`blocked`）。工具政策（`policy.go`、`shell.go`：拆開組合指令與 `sh -c`、`cmd /c` 這類包裝，依角色決定）由 `awkit guard tool` 當作 Claude Code 的 PreToolUse hook 執行（`hook.go`）：awkit 啟動的每一個 session 都在啟動時的設定裡帶著它，指令寫的是這個 awkit 的完整路徑。Claude 的 Worker 與 engine 的 agent 只帶 awkit 寫的設定（`--settings`、`--setting-sources ""`、`dontAsk`）；`AWKIT_ENGINE=principal` 的主控以 `--settings` 加在專案的設定之上。專案自己的設定檔裡沒有它，人自己開的 session 不受它管。`env.go` 決定 agent 拿到哪些環境變數。`outbound.go` 在每一個寫到 GitHub 的 `gh` 呼叫送出前掃描憑證與個資（`GHGate`：`ghutil.RunWithRetry`、`landing.GH` 與 `awkit lessons promote` 都經過它；被擋的錯誤包著 `ErrBlocked`，Worker 的 pull request 被擋時結果是 `blocked`、階段 `guard_output`）；呼叫屬於哪個專案與工單由 `context` 裡的 `Scope` 帶著。`diff.go` 在 Worker 的變更裡找可疑的模式，只記錄（`flag`）。教訓的篩檢與隔離在 `lessons/screen.go`。見 [Agent Engine](specs/agent-engine/design.md) §9；高風險規則（`risk.go`：`high-risk/1`，審查通過的改動要不要等主人核可） |
| `reviewer` | `prepare-review`、`submit-review`、證據驗證、多模型審查、人的意見、合併。判斷在 `SubmitReview`，對 GitHub 與專案做的事經過 `Publisher`（`publisher.go`：主控的用 gh 與標籤，engine 的不改標籤），結果的 `Next` 說工單接下來去哪裡。engine 的審查以 `Queue` 只核准、不合併（`queue.go`：核准記在 `reviews/pr-N/approval.json`；合併佇列用 `MergeAt` 只合併驗過的 commit，`HumanGate` 合併前再看維護者的審查，`AfterMerge` 做合併之後的事） |
| `jittest` | 審查時依 diff 產生並執行測試 |
| `lessons` | 學習迴圈：記錄 → 濃縮 → 篩檢 → 放進 prompt → 結算；篩檢沒過的隔離，由人放行或不採用（`Decide`，決定寫成 `guardrail_decision` 事件） |
| `hooks` | 設定檔的 `hooks` |
| `trace`、`session` | 事件流；Principal 與 Worker 的 session 紀錄。一輪的事件是它的 session 檔，加上沒有 run 串流的程序在那段時間寫進旁檔的（`engine`、`guard`、`agent`，名稱定在 `trace`）：`ReadRun` 從旁檔的尾端往回讀到這一輪開始為止，工作台的活動紀錄與 `awkit events` 都用它 |
| `status`、`doctor`、`reset` | 同名的指令 |
| `workbench` | `awkit ui`：只聽 127.0.0.1 的 HTTP 伺服器（每次啟動一把新鑰匙、只接受自己頁面送來的變更）、各頁讀的資料（和 `status`、`events`、`doctor`、`plan` 同一套讀法）、變更的推送、使用者自己的設定檔（專案清單、教學進度）；從頁面開的會議和核可（`meetings.go`）、教訓與人對隔離教訓的決定（`lessons.go`）、從頁面開始的工作（`runs.go`）、設定的修改（`project_settings.go`，`awkit settings apply`）由 `awkit detach` 另起一個不屬於工作台的程序來做，關掉工作台、甚至連子程序一起結束它，都照做；會議停只能靠暫停或開完，工作停靠停止標記（和 `awkit stop` 一樣），engine 的 run 另外經由它的收件匣交給它（兩種停止都是；立刻停等不到回覆才結束程序），主控的 run 立刻停是結束它的整個程序樹（`kill_windows.go` 用 `taskkill /T`，其他系統結束它的程序群組）。執行頁讀 `engine.Look`（`GET …/engine`，串流的 `engine` 區塊在 `.ai/state/engine/` 有變時通知）。即時輸出直接讀 `.ai/state/logs/` 的 run、主控與執行者 log，只讀那個資料夾裡的檔案。頁面本身在 `web/`（React、TypeScript、Tailwind、shadcn/ui），`npm --prefix web run build` 建到 `internal/workbench/dist/`，由 `go:embed` 放進 awkit；repository 裡只有 `dist/.keep` |
| `stopmark`、`workflow` | STOP 標記；停止原因、結束語、報告 |
| `selfheal` | 修復結構性的壞狀態（integration branch 遺失骨架時補回去） |
| `audit`、`evaluate` | `awkit evaluate`：給 CI 用的檢查 |
| `repo` | 語言與工具鏈的對照 |
| `ghutil`、`claudecli` | 帶重試的 `gh` 呼叫；單輪 `claude` 呼叫的參數 |
| `gitlock` | repo 的 git 鎖：engine 的線同時跑，每個 Worker 是自己的程序，共用主 repo 的 `.git`。AWK 自己下的 git 指令（Worker、審查、解衝突、教訓、文件落地）都先拿鎖再跑：作業系統的檔案鎖（Windows `LockFileEx`、其他 `flock`），鎖檔在 repo 共用的 git 目錄裡（worktree 與主 repo 同一把，submodule 另一把），程序結束就放掉；一次只鎖一個指令。碰到 git 自己的鎖衝突（agent 或人在用）等一下再試；刪「留下來的」`index.lock` 也在鎖裡做 |
| `updatecheck`、`buildinfo`、`util` | 版本檢查、版本號、跨平台的小工具 |

## kit 怎麼出貨

這個 repository 本身就是一個 AWK 專案，所以 kit 就是它自己的 `.claude/`。`kit_embed.go` 用 `go:embed` 把列名的檔案（`rules/_kit`、三個 skill、兩個 subagent）嵌進 binary，`install` 再把它們寫進使用者的專案。

- repository 自己的 `.ai/config.yaml` 與 `.ai/roster.yaml` 不出貨；專案的這兩份來自 preset（`internal/install/presets.go`、`internal/council/rosters/`；名冊是範本，工程文件的位置依專案的 repo 填入）。
- 起始規則是範本：每個專案都有的在 `internal/install/starter-rules/common/`，一個技術棧才有的在 `starter-rules/<preset>/`。建立專案時用設定檔（repo 與資料夾）與 owner 的回答（契約的位置、已定的技術選擇）寫一次，之後歸專案所有。
- kit 拿掉的檔案列在 `internal/install/deprecated.txt`，升級時會從專案裡移除。
- `CLAUDE.md`、`AGENTS.md`、`.claude/rules/_kit/git-workflow.md` 是 `generate` 產生的：要改內容，改 `internal/generate/generator.go`，不是改這個 repository 裡的那幾份。

## 讓文件與程式對得上的測試

幾個測試專門防「說的」與「做的」分開：

| 測試 | 防什麼 |
|---|---|
| `config.TestFull_NamesEveryKeyAndNoOther` | 設定結構加了欄位卻沒進「每個鍵都有」的範例檔，或反過來 |
| `config.TestConfigurationDocument_DescribesEveryKeyAndNoOther` | `docs/user/configuration.md` 少說明一個鍵，或列了一個沒有程式讀的鍵 |
| `install.TestPresets_WriteOnlyWhatIsRead` | preset 寫出沒有程式讀的鍵 |
| `kit_embed_test.go` | skill 的 frontmatter 不是合法 YAML、點名的檔案不存在、該嵌入的沒嵌入 |
| `install.TestSkeleton…`（CI 的 `skeleton` job） | preset、骨架與產生的 CI workflow 對不上 |
| `generate` 的 `guides_test.go` | 產生的指引漏掉該說的事 |

## 相關文件

- [貢獻](contributing.md)：建置、測試、commit 與發版
- [學習迴圈的設計](specs/learning-loop/design.md)
- [Agent Engine](specs/agent-engine/README.md)：以確定性的 engine 取代 LLM 主控、平行的線與合併佇列、角色設定、分層的 guardrail、容器隔離（已核可，分階段實作中；本頁描述的是已經做到的部分：P1 起 `awkit run` 由 engine 跑，P2 起多條線同時跑、由合併佇列合併；容器還沒有）
- [Skills 與規則](../user/skills.md)：kit 的 skill 怎麼寫
