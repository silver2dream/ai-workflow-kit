# 開始使用

從零到第一次 run。每一步之後 `awkit` 都會說下一步是什麼；這份文件多說明「為什麼」與「卡住時怎麼辦」。

---

## 1. 準備

| 需要 | 用途 |
|---|---|
| `git`，專案的 `origin` 是一個 GitHub repository | 票券是 issue，Worker 把分支推到 `origin` 再開 pull request |
| [`gh`](https://cli.github.com/)，`gh auth login` 完成，對該 repository 有寫入權限 | 建 issue、改標籤、審查與合併 |
| [`claude`](https://claude.com/claude-code)（Claude Code CLI） | 扮演審查者與議會 |
| `codex` 或 `claude` | 扮演 Worker；用哪個由 `worker.backend` 決定，預設 `codex` |
| Windows：Git for Windows | Claude Code 需要它的 Git Bash，建置與測試指令也透過它執行 |

安裝 `awkit`：

```bash
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash
```

```powershell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex
```

## 2. 設定專案

在要讓它工作的 repository 根目錄：

```bash
awkit init                                # 單一 repo、不限技術棧
awkit init --preset godot-go --scaffold   # Go 後端 + Godot C# 前端，附能建置的骨架
awkit init --dry-run                      # 只看會寫哪些檔
```

在終端機裡執行時，`awkit init` 會先問幾個問題，按 Enter 就用括號裡的預設值：專案用什麼做的、要不要建骨架、誰寫程式、pull request 合併到哪個分支、repo 之間的契約放哪裡、每個 repo 已經決定了哪些技術選擇。答案會寫進設定檔與專案的起始規則（`.claude/rules/`，見 [Skills 與規則](skills.md)）。不想被問就加 `--yes`；每一題也有對應的選項，`awkit init --help` 會列出來。

`awkit init` 結尾會列出它寫了什麼。要看的是設定檔 `.ai/config.yaml`：

- **分支**（`git.integration_branch`、`git.release_branch`）：填的是 repository 裡已有的分支。有 `develop` 就是 `develop` 合併、預設分支發版；沒有就是單一主幹。pull request 都開向 integration branch。
- **建置與測試指令**（每個 repo 的 `verify.build`、`verify.test`）：Worker 交付前會跑，審查時測試會再跑一次。`generic` preset 留空，**開工前一定要填**；`godot-go` 已填好 `go build ./...`、`go test ./...`、`dotnet build`、`dotnet test`。
- **誰來寫程式**（`worker.backend`）：`codex` 或 `claude-code`。

改過設定檔之後執行 `awkit generate`，`CLAUDE.md`、`AGENTS.md` 與 CI workflow 會跟著更新；`awkit validate` 檢查設定檔。每個鍵的說明在[設定檔](configuration.md)。

然後把這些檔案 commit 並推上去。run 從乾淨的工作目錄開始，而且 integration branch 必須在 `origin` 上：

```bash
git add -A && git commit -m "[chore] set up awkit"
git push          # 分支還沒有 upstream 時：git push -u origin <分支>
```

## 3. 檢查

```bash
awkit doctor
```

它把 run 開始前會檢查的事一次問完：工具與登入、設定檔是否有效、`origin` 是否存在、integration branch 是否在 `origin` 上、對 repository 有沒有寫入權限。每個問題都附怎麼修。全部通過再往下。

## 4. 規劃

```bash
awkit plan "每日登入獎勵：連續七天、獎勵遞增、漏一天重來"
```

議會（`.ai/roster.yaml` 裡的角色）先排議程，再一個議題一個議題討論：每個議題由一個角色提方案、其他角色依序發言、有分歧就對辯、決策角色裁定，擁有者把結論寫進文件。最後排出里程碑，並為第一個里程碑提出票券：

- `docs/plans/<plan>/tickets.yaml`：里程碑與第一個里程碑的票券提案。**這是你要看的東西**，可以直接改。
- `docs/plans/<plan>/agenda.yaml`：議程。想改議題或發言的人，停下來改它，再 `awkit plan --resume <plan>`。
- `docs/plans/<plan>/minutes.md`、`docs/plans/<plan>/topics/`、`docs/decisions.md`：會議紀錄（總表與逐議題）、決策。
- 各角色的文件：產品與設計在 `docs/<area>/`，各 repo 的工程文件在那個 repo 的 `docs/`（位置寫在 `.ai/roster.yaml`）。它們照 `.claude/rules/` 的規則寫；寫完會對照規則檢查一次，仍然不一致的地方會在結束時列出來。

想在規劃途中被問問題，用 `awkit plan --ask "…"`。

Worker 是從 integration branch 讀專案的，所以議會寫的文件要先進 integration branch。最簡單是讓核可一起做：

```bash
awkit plan approve <plan> --merge-docs --dry-run   # 先看會建哪些 issue、哪些文件會合併
awkit plan approve <plan> --merge-docs             # 建 issue，文件開 PR、CI 過了就合併，再幫工單掛上標籤
```

`--merge-docs` 先把 issue 建好但不掛工作標籤，等文件的 PR 合併進 integration branch 之後才掛上，所以 Worker 不會在文件到之前動手；最後把工作目錄更新到 integration branch，讓 `awkit run` 能開工。CI 沒過或 GitHub 規定要人審核時，它停下來說明，修好再執行一次會接著做。工作台的「核可」就是這樣做的。

想自己 commit 也可以（漏了的話，核可時會列出還沒 commit 的檔案）；注意核可會把 issue 編號寫回 `tickets.yaml`，這個變更也要 commit，`awkit run` 才會開工：

```bash
git add docs backend/docs frontend/docs && git commit -m "[docs] plan the daily login reward" && git push
awkit plan approve <plan>
```

只想補一張票，不必開會：照 `.claude/skills/principal-workflow/references/ticket.md` 的格式寫內文，再

```bash
awkit create-task --title "[feat] add a health endpoint" --body-file ticket.md
```

## 5. 開工

```bash
awkit run              # 處理所有開著的票
awkit run --issue 42   # 只做一張
awkit run --dry-run    # 只跑啟動前的檢查
```

`awkit run` 在這個終端機裡由 awkit 的 engine（程式，不是模型）派工。每張票的流程：Worker 在自己的 worktree 裡寫變更並開 pull request → 審查（重跑測試、逐條核對驗收條件；CI 要通過）→ 通過就合併並關閉 issue，不通過就帶著意見退回給 Worker。票券依編號由小到大處理，依賴沒完成的會先跳過。設了環境變數 `AWKIT_ENGINE=principal` 時改用舊的做法：啟動 Claude Code 當 Principal 來派工。這個做法下一版移除。

另一個終端機可以看進度或請它停下：

```bash
awkit status   # run 的狀態、每張票在哪個階段、下一步
awkit stop     # 做完手上這一步就停；Worker 正在跑的話會等它做完
```

你也可以在 run 進行中直接到 GitHub 上審 pull request：有你未解決的審查意見時不會合併，Worker 會帶著你的意見重做。合併不會等你；合併之後才寫的意見，會在下一張票開工前被讀進來，變成教訓。

## 6. run 結束時

`awkit run` 最後會說為什麼結束、接下來做什麼。

| 它說 | 意思 | 你要做的 |
|---|---|---|
| Every ticket is done. | 沒有票了 | 規劃下一批（`awkit plan`），或補一張票 |
| The run was stopped, as asked. | 你用 `awkit stop` 停的 | `awkit run` 接著做 |
| A ticket needs you. | 有 issue 被標上 `needs-human-review`（審查退回太多次、衝突解不掉，或 Worker 要讀的內容被 guardrail 擋下） | `awkit status` 看是哪張、為什麼；處理後拿掉標籤，再 `awkit run` |
| A pull request was turned back by the review as often as a review is tried. | 同一個 pull request 被審查退回太多次 | 看 pull request 上的審查留言；處理後 `awkit run` |
| A ticket's Worker failed on every attempt. | Worker 的嘗試用完了 | issue 上有最後一次的錯誤；修正原因或改寫票，`awkit reset --labels`，再 `awkit run` |
| A ticket cannot be read. | 票的 `awk-ticket` 區塊有誤 | issue 上有留言說明；改正區塊、拿掉 `needs-human-review`，再 `awkit run` |
| Every open ticket waits for an issue that is not done. | 剩下的票都卡在依賴上 | 完成或重開被依賴的 issue，或修正 `depends_on` |
| Too many attempts failed in a row. / The run reached the limit of its loop. | 連續失敗，或某張票在繞圈 | `awkit events --level error` 看發生了什麼；修正共同原因，`awkit reset`，再 `awkit run` |

結束碼：工作做完或被你停下是 0，其餘是 1。

想知道某張票到底發生了什麼：

```bash
awkit events --issue 42      # 這張票的完整經過
awkit status --issue 42      # 它留下的結果檔與 log 在哪裡
gh issue view 42 --comments  # 流程在 issue 上的留言
```

## 7. 升級

```bash
awkit check-update   # 有沒有新版
awkit upgrade        # 把專案裡的 kit 檔案更新到已安裝的版本，並 commit
```

升級會保留你的設定檔、規則與自己加的 skill。

---

## 下一步

- [設定檔](configuration.md)：每個鍵，以及議會的名冊
- [Skills 與規則](skills.md)：告訴 agent 的事，以及怎麼加自己的規則
- [疑難排解](troubleshooting.md)、[FAQ](faq.md)
