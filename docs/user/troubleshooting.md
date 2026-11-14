# 疑難排解

東西不動的時候，先問這兩個指令。它們會說出了什麼事，也會說怎麼處理。

```bash
awkit doctor   # run 需要的東西是否齊全；之前的 run 留下了什麼
awkit status   # run 是否在跑、上一次怎麼結束、每張票在哪個階段、下一步
```

---

## `awkit run` 不開始

run 開始前會做一串檢查，第一個不過就停下來並說是哪一個。`awkit doctor` 把同一串檢查一次問完。

| 檢查 | 它在說什麼 | 處理 |
|---|---|---|
| gh CLI、gh Auth | 沒裝 `gh`，或沒登入 | 安裝 [`gh`](https://cli.github.com/)；`gh auth login` |
| claude CLI | 沒裝 Claude Code | 安裝 [Claude Code](https://claude.com/claude-code) 並登入 |
| worker backend | `worker.backend` 指定的 CLI（`codex` 或 `claude`）找不到。run 只警告，但沒有它每張票都會失敗 | 安裝它，或在 `.ai/config.yaml` 換成另一個 backend |
| Git Remote | repository 沒有 `origin` | `git remote add origin <GitHub repository 的網址>` |
| Git Status | 工作目錄有還沒 commit 的變更 | `git add -A && git commit`，或 `git stash` |
| Lock File | 另一個 run 正在這個專案裡跑 | 等它結束，或用 `awkit stop` 請它停。當掉的 run 留下的 lock 會自動清掉；清不掉時 `awkit reset --lock` |
| Config | 設定檔無效，後面是哪個鍵、哪裡不對 | 改 `.ai/config.yaml`；`awkit validate` 會把所有問題列出來 |
| Repo Paths | 某個 repo 的 `path` 不存在 | 建立那個目錄，或修正 `repos[].path` |
| Integration Branch | `git.integration_branch` 不在 `origin` 上 | 分支在本機：`git push -u origin <分支>`。分支不存在：建立它，或把設定改成 pull request 要合併進去的分支 |
| Repo Write Access | 登入的帳號對 repository 只有讀取權限 | 換一個有寫入權限的帳號或 token 登入 `gh` |

「Stop Marker」只是告知：上一次 run 留下的 STOP 標記，下一次 run 開始時會自己清掉。

## run 一開始就結束

| 它說 | 可能的原因 |
|---|---|
| Every ticket is done. | 沒有開著的票。票是帶 `ai-task` 標籤的 open issue：自己開的 issue 要有這個標籤與 `awk-ticket` 區塊（`awkit create-task` 會處理） |
| Every open ticket waits for an issue that is not done. | 每張票的 `depends_on` 都指向還沒完成的 issue。`awkit status` 會列出誰在等誰 |
| A ticket cannot be read. | 某張票的 `awk-ticket` 區塊有誤。流程已在那個 issue 上留言說明哪裡錯 |
| The project has no configuration. / The configuration cannot be used. | 沒有 `.ai/config.yaml`（`awkit init`），或它無效（`awkit validate`） |

## 一張票卡住了

`awkit status` 把每張開著的票依階段列出來。階段是從 issue 的標籤讀的：

| 階段 | 標籤 | 意思 | 你要做的 |
|---|---|---|---|
| needs you | `needs-human-review`、`worker-failed`，或 `awk-ticket` 區塊讀不懂 | 流程放棄了，等人處理 | 見下面兩節 |
| in review | `pr-ready`、`review-failed` | pull request 等審查，或審查要重來一次 | 不用做事；run 會審它 |
| in progress | `in-progress` | Worker 正在做 | 不用做事。run 被中斷而留下這個標籤也沒關係：下一次 run 會先確認那個 Worker 的結果 |
| back to a Worker | `merge-conflict`、`needs-rebase` | pull request 與 integration branch 衝突，或落後於它 | 不用做事；run 會解衝突或把票交回給 Worker |
| ready | 只有 `ai-task` | 可以開工。被審查退回、等 Worker 重做的票也在這裡 | `awkit run` |
| waiting | 只有 `ai-task` | 在等 `depends_on` 的 issue | 完成那些 issue |

### Worker 每次都失敗（`worker-failed`）

同一張票的派工失敗三次之後，流程會標上 `worker-failed` 並在 issue 上留言。

```bash
gh issue view 42 --comments              # 最後一次的錯誤
awkit status --issue 42                  # 結果檔與 log 在哪裡
awkit events --issue 42 --level error    # 這張票出過的錯
```

Worker 的 log 在 `.ai/state/logs/issue-42.worker.log`，它拿到的 prompt 與留下的摘要在 `.ai/state/runs/issue-42/`。常見的原因：

- **建置或測試指令在乾淨的 checkout 裡跑不起來。** Worker 在 `.worktrees/` 下的新 worktree 裡工作；需要先安裝相依套件的 repo 要設 `verify.setup`。
- **票太大或太模糊。** 把它拆開，或把驗收條件寫成可以測的句子。
- **Worker 改了受保護的檔案**（`.claude/`、`.ai/config.yaml`、`.ai/roster.yaml`、`CLAUDE.md`、`AGENTS.md`、`.mcp.json`）。票真的需要改它們時，在 `awk-ticket` 區塊加 `allow_script_changes: true`。
- **diff 裡有看起來像密碼或金鑰的字串。** 確實需要時（例如測試用的假金鑰），在區塊加 `allow_secrets: true`。

修正之後把票放回佇列：

```bash
awkit reset --labels   # worker-failed → ai-task、review-failed → pr-ready
awkit run
```

### 需要人處理（`needs-human-review`）

流程在這些情況下會把票交給你：同一個 pull request 的審查重來的次數達到 `escalation.max_review_attempts`；衝突解不掉；票的 `awk-ticket` 區塊讀不懂；pull request 上有你留下的審查意見，而 Worker 被退回之後沒有推任何修改。issue 上的留言會說是哪一種。

處理完之後拿掉標籤，票就回到佇列：

```bash
gh issue edit 42 --remove-label needs-human-review
awkit run
```

### 審查一直退回

審查的結果寫在 pull request 的留言裡。兩種情況值得分開看：

- **審查指出真的問題，Worker 沒修好。** 把留言裡的重點補進票的驗收條件，或自己在 pull request 上留審查意見：你的意見會原文交給 Worker。
- **審查的證據對不上**（它引用的測試沒有跑過、或找不到）。確認 `verify.test` 真的會執行票裡要求的測試，而且測試名稱出現在輸出裡。

## run 停在奇怪的地方

- **Too many attempts failed in a row.** 連續 5 次失敗。通常是環境問題（登入過期、網路、建置指令壞了），不是某一張票的問題。`awkit events --level error` 看這幾次失敗的共同點；修好之後 `awkit reset`，再 `awkit run`。
- **The run reached the limit of its loop.** 迴圈跑了太多次，表示有票在繞圈。`awkit events --level decision` 看是哪一張；處理之後 `awkit reset`。
- **Claude Code 停下來問你要不要允許某個指令。** `awkit generate` 會把 run 需要的權限補進 `.claude/settings.local.json`；設定檔的建置或測試指令換了工具之後，再執行一次 `awkit generate`。
- **想讓它停。** `awkit stop`：做完手上這一步就停，不會留下做到一半的東西。要立刻停，在啟動它的終端機按 Ctrl-C。

## 設定檔

```bash
awkit validate
```

- **錯誤**（結束碼 1）：缺必填的鍵、`project.type` 或 repo 的 `type` 不是允許的值、標籤被改名、自訂 subagent 的名稱不合規則、值的型別不對（該是數字的地方寫了字）。
- **沒有程式讀的鍵**（仍然有效）：舊版 awkit 寫下而現在不用的鍵，或拼錯的鍵。它們不會有任何作用。以前的 preset 寫過 `audit`、`escalation.triggers`、`escalation.max_single_pr_files` 之類的鍵，看起來像是有保護，其實從來沒有生效；刪掉即可。

改過設定檔之後執行 `awkit generate`，產生的檔案才會跟上。

## 升級之後

```bash
awkit upgrade --dry-run   # 先看它會做什麼
awkit upgrade
```

- 舊版把設定放在 `.ai/config/workflow.yaml`、規則與 skill 放在 `.ai/rules/`、`.ai/skills/`。任何指令遇到這種專案都會請你先 `awkit upgrade`，它會把檔案搬到新位置並 commit。
- 你接手過的 CI workflow（刪掉了開頭那行註解的）不會被動。

## Windows

- `awkit` 可以從 PowerShell 或 Git Bash 執行。建置與測試指令透過 Git Bash 執行，所以要裝 Git for Windows；Claude Code 本身也需要它。
- 終端機的中文變成亂碼：PowerShell 先執行 `chcp 65001`，或改用 Windows Terminal。
- `git` 提示行尾會被換成 CRLF：那是 git 的 `core.autocrlf` 設定，與 AWK 無關，不影響流程。

## 還是不行

`awkit events --json > events.jsonl` 會把整個 run 的事件倒出來。到 [GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues) 回報時，附上 `awkit doctor` 的輸出、`awkit status` 的輸出，以及出事那張票的 `awkit events --issue <N>`。
