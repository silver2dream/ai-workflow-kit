# 疑難排解

東西不動的時候，先問這兩個指令。它們會說出了什麼事，也會說怎麼處理。

```bash
awkit doctor   # run 需要的東西是否齊全；之前的 run 留下了什麼
awkit status   # run 是否在跑、engine 在做哪一步、每張票走到哪、下一步
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
| A ticket cannot be read. | 某張票的 `awk-ticket` 區塊有誤（例如 `paths` 在它的 repo 資料夾外，或包括工作流程自己的檔案）。流程已在那個 issue 上留言說明哪裡錯 |
| The project has no configuration. / The configuration cannot be used. | 沒有 `.ai/config.yaml`（`awkit init`），或它無效（`awkit validate`） |

## 一張票卡住了

`awkit status` 先列 engine 手上的票（`Lanes`）：每張走到哪、它的 pull request、重試或失敗了幾次（以及上限）、從什麼時候；engine 正在做的那張多一行「now」，需要你的那張多一行原因，被 guardrail 擋下或標記的多一行判定。上一輪做完的也列著，更早的只算張數。

接著把每張開著的票依階段列出來。階段是從 issue 的標籤讀的：

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
- **建置或測試超過時間上限。** 錯誤訊息是 `timeout after 10m0s, the limit of one build or test command (timeouts.verify_minutes in .ai/config.yaml)`。測試本來就要跑這麼久的話，調高 `timeouts.verify_minutes`；否則找出是哪個測試卡住。
- **票太大或太模糊。** 把它拆開，或把驗收條件寫成可以測的句子。
- **Worker 改了受保護的檔案**（`.claude/`、`.ai/config.yaml`、`.ai/roster.yaml`、`CLAUDE.md`、`AGENTS.md`、`.mcp.json`）。票真的需要改它們時，在 `awk-ticket` 區塊加 `allow_script_changes: true`。
- **diff 裡有看起來像密碼或金鑰的字串。** 確實需要時（例如測試用的假金鑰），在區塊加 `allow_secrets: true`。

修正之後把票放回佇列：

```bash
awkit reset --labels   # worker-failed → ai-task、review-failed → pr-ready
awkit run
```

### 需要人處理（`needs-human-review`）

流程在這些情況下會把票交給你：同一個 pull request 的審查重來的次數達到 `escalation.max_review_attempts`；衝突解不掉；票的 `awk-ticket` 區塊讀不懂；幾張票的 `depends_on` 互相等待、繞成一圈（每一張都會轉給你，留言說是哪幾張、怎麼繞的）；pull request 上有你留下的審查意見，而 Worker 被退回之後沒有推任何修改；Worker 要讀的內容，或它的 pull request 要寫到 GitHub 的內容，被 guardrail 擋下（見下一節）。issue 上的留言會說是哪一種。

處理完之後拿掉標籤，票就回到佇列：

```bash
gh issue edit 42 --remove-label needs-human-review
awkit run
```

### 被 guardrail 擋下（`blocked`）

Worker 開工前，AWK 會檢查它要讀的內容。工單是你寫的或核可過的，照原樣給；審查意見、教訓這些由 agent 寫的內容，會先拿掉看不見的字元與 HTML 註解，再比對注入的寫法（例如「ignore all previous instructions」「忽略之前的所有指示」、假的對話或工具呼叫格式、把密鑰送到外部的指令）。明顯是注入的會擋下：這張票不派出去，標上 `needs-human-review`，issue 上的留言寫出是哪條規則、內容來自哪裡，但不貼原文（Worker 會讀 GitHub）。

```bash
awkit events --issue 42 --type guardrail_verdict   # 被擋下的原文與原因，只在這台電腦上
```

那段文字如果沒問題（例如是在討論注入的例子），到留言寫的來源把它改掉或刪掉，再拿掉標籤。只是可疑、不到擋下的內容，會照樣交給 Worker，但包在標明來源的資料區塊裡，並記一筆在同一個地方。

**要寫到 GitHub 的內容被擋下。** awkit 每一次用 `gh` 寫到 GitHub（開 pull request、留言、開 issue、送審查），送出之前都會掃過要寫的標題與內文：看起來像憑證的（GitHub、Anthropic、OpenAI、AWS、Slack、Google 的金鑰與 token、私鑰、JWT），或個人資料（檢查碼對得上的信用卡號、身分證字號）就不送。Worker 的 pull request 被擋時，分支已經推上去、但 pull request 沒開，票標上 `needs-human-review`；其他指令被擋時，指令失敗並印出 `AWK guard:` 開頭的原因。記錄裡只有規則與欄位，沒有內容本身：

```bash
awkit events --issue 42 --type guardrail_verdict   # 哪一條規則、哪一個 gh 指令
```

把憑證或個資從分支與 Worker 的摘要拿掉；真的寫出去過的憑證，撤銷換新。付款服務公開的測試卡號（例如 `4242 4242 4242 4242`）與表單上的範例身分證字號不會被擋。

**教訓被隔離（`quarantined`）。** 教訓由 agent 從審查的案例濃縮出來，而案例裡可能有別人寫的文字。所以一條教訓在交給 Worker 之前會先過篩檢：注入的寫法，或講到流程的權限與檢查（`--no-verify`、`permission_mode`）、連外（`curl`、網址）、流程自己的檔案與指令（`.ai/state`、`awkit submit-review`）的，會被隔離，不再給任何 Worker，並記一筆在 `awkit events`。`awkit lessons list` 會列出隔離的原因；讀過、確定沒問題，就放行：

```bash
awkit lessons list
awkit lessons approve L-012   # 放行後當成新的教訓重新試用
awkit lessons reject L-012    # 不採用：停用，不會再給任何 AI
```

工作台的「教訓」頁做的是同一件事，被擋下的教訓也會出現在「需要你處理」。你的決定記在事件裡：`awkit events --all --type guardrail_decision`。

你用 `awkit lessons add` 自己加的教訓不篩檢。

**可疑的變更（只記錄）。** Worker 的變更通過密鑰檢查之後，AWK 會看它新增的內容有沒有這些情況：新的對外連線、執行期才決定的指令或程式碼、改了 CI 或建置、改了相依套件或安裝時執行的腳本、二進位檔、大量刪除。有的話記一筆 `flag`（規則是 `diff-signals/1#…`，只記檔名，不記內容），變更照樣去跑測試與審查；審查一個 pull request 時可以用同一個指令看它有沒有這些記錄。

### 審查一直退回

審查的結果寫在 pull request 的留言裡。兩種情況值得分開看：

- **審查指出真的問題，Worker 沒修好。** 把留言裡的重點補進票的驗收條件，或自己在 pull request 上留審查意見：你的意見會原文交給 Worker。
- **審查的證據對不上**（它引用的測試沒有跑過、或找不到）。確認 `verify.test` 真的會執行票裡要求的測試，而且測試名稱出現在輸出裡。
- **審查重跑測試時超過時間上限**（留言裡是 `the tests ran out of time after …`）。被截斷的測試不算證據，即使前面的測試已經通過；調高 `timeouts.verify_minutes`。

## run 停在奇怪的地方

- **Too many attempts failed in a row.** 連續 5 次失敗。通常是環境問題（登入過期、網路、建置指令壞了），不是某一張票的問題。`awkit events --level error` 看這幾次失敗的共同點；修好之後 `awkit reset`，再 `awkit run`。
- **The run reached the limit of its loop.** 迴圈跑了太多次，表示有票在繞圈。`awkit events --level decision` 看是哪一張；處理之後 `awkit reset`。
- **Claude Code 停下來問你要不要允許某個指令。** `awkit generate` 會把 run 需要的權限補進 `.claude/settings.local.json`；設定檔的建置或測試指令換了工具之後，再執行一次 `awkit generate`。
- **出現 `AWK guard:` 開頭的拒絕。** Worker、審查者與解衝突者（還有 `AWKIT_ENGINE=principal` 時主控的 session）的每一個工具呼叫都先經過 `awkit guard tool`：不能自己合併 pull request（那是審查通過後 `awkit submit-review` 的事）、不能推到整合或發版分支、不能讀放憑證的地方、不能連外、不能印出環境變數、不能用 `gh issue create` 開票。`awkit events --all --type guardrail_verdict` 看是哪一條規則擋下了什麼。awkit 啟動它們時就把這個檢查一起帶上，你自己在專案裡開的 Claude Code 不受它管。
- **你自己開的 Claude Code 也出現 `AWK guard:` 的拒絕，或每個動作都失敗。** 舊版的 awkit 會把主控的檢查寫進專案的 `.claude/settings.local.json`，於是專案裡每一個 Claude Code 都被當成主控檢查；PATH 上的 awkit 太舊或找不到時，每一個工具呼叫都會失敗。執行一次 `awkit upgrade`（或 `awkit generate`）就會把它拿掉，你自己加的 hook 與權限都留著。
- **想讓它停。** `awkit stop`：做完手上這一步就停，不會留下做到一半的東西。要立刻停，用 `awkit stop --now`。

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

`awkit events --json > events.jsonl` 會把整個 run 的事件倒出來：run 自己的，加上 engine（每張工單從哪一步到哪一步、收到的停止、和主控的比對）與 guardrail 的 hook 在那段時間寫在自己檔案裡的。`awkit events --session engine` 只看 engine 的檔案（每一輪都在裡面）。到 [GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues) 回報時，附上 `awkit doctor` 的輸出、`awkit status` 的輸出，以及出事那張票的 `awkit events --issue <N>`。
