# 架構

`awkit` 本身怎麼做的。對象是要改 AWK 的人；用 AWK 的人看 [開始使用](../user/getting-started.md) 就夠了。

---

## 一句話

**模型做判斷，程式做決定。** 寫程式、審查、辯論交給模型；「下一步做什麼」「這算不算通過」「能不能合併」由 Go 程式依 GitHub 上的狀態與實際跑出來的結果決定。模型交回來的是表單，不是結論。

三個角色，都是外部的 CLI：

| 角色 | 誰扮演 | 做什麼 | 不做什麼 |
|---|---|---|---|
| Principal | Claude Code（`awkit run` 啟動的 session） | 照 `principal-workflow` skill 的迴圈，把 awkit 的指令接起來 | 不寫程式、不審查、不合併、不決定下一步 |
| Worker | `codex` 或 `claude`（`worker.backend`） | 在自己的 worktree 裡改檔案、跑驗證 | 不碰 git、不開 pull request（runner 做） |
| Reviewer | Claude Code 的 `pr-reviewer` subagent | 讀 diff、交一份結構化的 `review.json` | 不決定合併（`awkit submit-review` 驗證之後決定） |

規劃是另一條線：`awkit plan` 由程式主持一場會議，每個角色只回答一個問題、交回一段文字；寫檔的只有程式。

## 兩條流程

```
awkit plan "…"
  └─ council.Run：立場 → 提問 → 異議 → 對辯 → 主持人整理 → 裁定 → 文件 → 票券清單
       寫出 docs/…、docs/decisions.md、docs/plans/<plan>/{minutes.md,tickets.yaml}
awkit plan approve <plan>
  └─ council.Approve → task.CreateTask：每張票一個 GitHub issue（ai-task 標籤 + awk-ticket 區塊）

awkit run
  ├─ kickoff：啟動前的檢查（PreflightChecker.steps）、lock、建立標籤、清掉上次的 STOP 標記
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
| 有哪些工作、各走到哪一步 | GitHub：issue 的標籤、pull request | `worker`、`reviewer`、`analyzer` |
| 一張票的結果、prompt、log | `.ai/state/results/`、`runs/issue-N/`、`logs/` | `worker` |
| 發生過的每件事 | `.ai/state/events/`（`awkit events` 讀） | `trace` |
| 迴圈次數、連續失敗次數、STOP 標記、lock | `.ai/state/` | `analyzer`、`worker`、`stopmark`、`kickoff` |
| 審查紀錄與教訓 | `.ai/state/review_feedback.jsonl`、`lessons.json` | `reviewer`、`lessons` |
| 一場規劃的每次模型呼叫 | `.ai/state/council/<plan>/` | `council` |

Principal 的對話不是狀態：session 隨時可以結束再開，下一個 session 從 `awkit analyze-next` 重新得知一切。`.ai/state/` 整個被 git 忽略。

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
| `install` | `init`、`upgrade`、`uninstall`：preset（`presets.go`）、起始規則（`starter-rules/`）、專案骨架、kit 檔案的安裝與汰換 |
| `generate` | 從設定產生的檔案：`CLAUDE.md`、`AGENTS.md`、kit 的 git 規則、自訂 subagent、CI workflow、Claude Code 的本機權限 |
| `migrate` | 設定檔格式的版本轉換，以及舊目錄結構的搬遷 |
| `council` | `awkit plan`：名冊、會議的每一步、文件、票券清單、核可 |
| `ticket`、`task` | `awk-ticket` 區塊的讀取與驗證；建立票券 |
| `kickoff` | `awkit run`：啟動前的檢查、lock、PTY、log |
| `analyzer` | 決策引擎：從 GitHub 的狀態決定下一步，計數迴圈與連續失敗 |
| `worker` | 派工、runner、backend、安全檢查、結果檢查 |
| `reviewer` | `prepare-review`、`submit-review`、證據驗證、多模型審查、人的意見、合併 |
| `jittest` | 審查時依 diff 產生並執行測試 |
| `lessons` | 學習迴圈：記錄 → 濃縮 → 放進 prompt → 結算 |
| `hooks` | 設定檔的 `hooks` |
| `trace`、`session` | 事件流；Principal 與 Worker 的 session 紀錄 |
| `status`、`doctor`、`reset` | 同名的指令 |
| `stopmark`、`workflow` | STOP 標記；停止原因、結束語、報告 |
| `selfheal` | 修復結構性的壞狀態（integration branch 遺失骨架時補回去） |
| `audit`、`evaluate` | `awkit evaluate`：給 CI 用的檢查 |
| `repo` | 語言與工具鏈的對照 |
| `ghutil`、`claudecli` | 帶重試的 `gh` 呼叫；單輪 `claude` 呼叫的參數 |
| `updatecheck`、`buildinfo`、`util` | 版本檢查、版本號、跨平台的小工具 |

## kit 怎麼出貨

這個 repository 本身就是一個 AWK 專案，所以 kit 就是它自己的 `.claude/`。`kit_embed.go` 用 `go:embed` 把列名的檔案（`rules/_kit`、三個 skill、兩個 subagent）嵌進 binary，`install` 再把它們寫進使用者的專案。

- repository 自己的 `.ai/config.yaml` 與 `.ai/roster.yaml` 不出貨；專案的這兩份來自 preset（`internal/install/presets.go`、`internal/council/rosters/`）。
- preset 的起始規則在 `internal/install/starter-rules/<preset>/`，只在建立專案時寫一次，之後歸專案所有。
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
- [Skills 與規則](../user/skills.md)：kit 的 skill 怎麼寫
