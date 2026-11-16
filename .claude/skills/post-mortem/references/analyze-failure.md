# Analyze Failure

## Evidence

先收集，再下結論。每一項記下有沒有拿到；拿不到的寫原因。

**流程自己的紀錄**

```bash
awkit status                           # 現在的狀態：session、迴圈計數、停止原因、卡住的 issue
awkit doctor                           # 殘留的鎖與計數器、停滯的標籤
awkit events --level error --last 50   # 最近的錯誤事件
awkit events --level decision          # 每一次決定下一步的依據
awkit events --issue <N>               # 一張票的完整經過，跨 session
```

| 位置 | 內容 |
|---|---|
| `.ai/state/logs/principal.log` | Principal 的輸出 |
| `.ai/state/logs/issue-<N>.worker.log` | 那張票的 Worker 的輸出 |
| `.ai/state/results/issue-<N>.json` | Worker 的結果：`status`、`failure_stage`、`error`、`pr_url` |
| `.ai/state/runs/issue-<N>/` | 那張票每一次嘗試留下的檔案，含 Worker 的 `plan.md` |
| `.ai/state/review_feedback.jsonl` | 被退回的審查與原因 |
| `.ai/state/workflow-report-*.md` | 每次停止時寫下的報告 |
| `.ai/state/STOP` | 存在就表示流程被要求停下；由 `awkit stop-workflow` 建立的會寫明原因 |

**GitHub**

```bash
gh issue view <N> --comments           # 流程在 issue 上的留言：失敗原因、審查意見
gh pr view <PR> --json state,mergeable,statusCheckRollup
gh pr checks <PR>                      # CI 的每一項
```

**Repo**

```bash
git status
git log --oneline -20
git worktree list                      # Worker 的 worktree 在 .worktrees/ 下
```

錯誤訊息、exit code、stack trace 逐字記下，附上出處（哪個檔案的哪一行，或哪個指令的輸出）。

## Root cause

歸到一個類別。一次失敗常有近因與遠因：近因是直接弄壞的那件事，遠因是讓它沒被擋下的缺口；兩個都寫。

| 類別 | 是什麼 | 徵兆 |
|---|---|---|
| 設定 | `.ai/config.yaml` 的欄位缺漏或寫錯：分支、repo、verify 指令、標籤 | 訊息點名某個設定欄位；`awkit validate` 不過；`exit_reason` 是 `config_invalid` |
| 程式 | Worker 寫出來的程式不對（測試失敗、建置不過），或 `awkit` 自己的缺陷 | 測試斷言失敗、編譯錯誤、panic 與 stack trace |
| 環境 | 工具沒裝或版本不對、權限不足、磁碟或行程上限、登入過期 | `command not found`、`permission denied`、`timeout`、要求重新登入 |
| 外部 | GitHub 中斷或限流、上游套件壞了、別人同時推了相衝突的變更 | HTTP 5xx、`rate limit`、合併衝突、相依解析失敗 |

把握程度：高（證據直接指到）、中（證據吻合，但還有別的解釋）、低（推測）。中與低要列出其他假設。

## Blast radius

- 直接：哪些 issue 與 PR 受影響；狀態檔或 git 歷史有沒有壞；有沒有做到一半的變更已經合併。
- 間接：後面的票是不是被擋住（`depends_on`）；流程要不要人處理才能繼續；有沒有分支或 worktree 停在髒的狀態。

## Recovery

寫出確切的指令與順序。常用的：

```bash
awkit reset --dry-run                  # 先看會清掉什麼
awkit reset                            # 清迴圈計數、連續失敗、STOP、暫存
awkit reset --labels                   # 把 worker-failed、review-failed 的 issue 放回佇列
awkit run --issue <N>                  # 只重跑一張票
```

確認復原成功：`awkit doctor` 沒有警告、`awkit status` 的下一步合理、`git status` 乾淨。

## Prevention

| 類型 | 例子 |
|---|---|
| 設定檢查 | 讓 `awkit validate` 擋下這種寫法 |
| 程式修正 | 修掉 `awkit` 或專案程式的缺陷，並補上會失敗的測試 |
| 啟動前檢查 | `awkit run` 的 preflight 或 `awkit doctor` 多查一項 |
| 規則 | `.claude/rules/` 多一條，讓 Worker 不再寫出這種東西 |
| 票券 | 票的寫法造成的（範圍太大、驗收條件含糊）：改寫法 |
| 訊息 | 錯誤訊息要說出原因與下一步 |

每一條寫：類型、要改的位置、優先度（P0 到 P2）、大小（小、中、大）。

## Report template

```markdown
# Post-Mortem: <一句話說明什麼失敗>

- 日期：<YYYY-MM-DD>
- 嚴重度：<P0 | P1 | P2 | P3>
- 狀態：<已復原 | 已緩解 | 調查中>

## 經過
- <時間>：<事件>

## 根因
- 類別：<設定 | 程式 | 環境 | 外部>
- 元件：<檔案、指令或服務>
- 近因：<直接弄壞的事>
- 遠因：<讓它沒被擋下的缺口>
- 把握：<高 | 中 | 低>
- 證據：<出處>

## 影響範圍
- Issue：<編號>
- PR：<編號>
- 分支與 worktree：<名稱，或「無」>
- 流程是否卡住：<是 | 否>

## 復原
1. <指令>

確認方式：<指令與預期結果>

## 預防
| # | 類型 | 位置 | 優先度 | 大小 | 說明 |
|---|---|---|---|---|---|

## 還不知道的事
- <問題>：<要查什麼才知道>
```

預設只在回覆裡給出報告。owner 要留存時，存成 `docs/post-mortems/<YYYY-MM-DD>-<主題>.md`。
