# FAQ

## 它是什麼、不是什麼

### AWK 在我的專案裡做什麼？

它把 GitHub issue 當票券：一個 Principal（Claude Code）依序把每張票交給 Worker（Codex 或 Claude Code）在獨立的 worktree 裡寫變更、開 pull request，再由審查決定合併或退回。票券可以由議會（`awkit plan`）從一句需求辯出來，也可以自己開。

### 可以不用 GitHub 嗎？

不行。票券是 issue，狀態記在 issue 的標籤上，變更經由 pull request 合併；`gh` 是唯一的介面。

### 支援哪些作業系統？

Windows、Linux、macOS。Windows 上要有 Git for Windows（Claude Code 需要它的 Git Bash，建置與測試指令也透過它執行）。

### 支援哪些語言？

任何語言：Worker 與審查只執行你在 `verify.build`、`verify.test` 給的指令。`go` 與 `csharp`／`godot` 另外有 CI 工具鏈的安裝步驟；`godot-go` preset 附一份 Go 後端加 Godot C# 前端的骨架與規則。

### 它會自己合併嗎？會等我嗎？

審查通過、CI 通過就合併，不等人。你隨時可以到 GitHub 上審 pull request：有你未解決的審查意見時不會合併，Worker 會帶著意見重做；合併之後才寫的意見，會在下一張票開工前被讀進來，成為之後的教訓。

---

## 安全

### Worker 能改什麼？

Worker 在 `.worktrees/` 下自己的 worktree 裡工作，只能透過 pull request 把變更送進 integration branch。交付之前，runner 會擋下三種變更，除非票的 `awk-ticket` 區塊明說允許：

| 被擋下的變更 | 票要寫什麼才放行 |
|---|---|
| 改到流程自己的控制檔：`.claude/`、`.ai/config.yaml`、`.ai/roster.yaml`、`CLAUDE.md`、`AGENTS.md`、`.mcp.json` | `allow_script_changes: true` |
| diff 裡出現看起來像密碼、API key、私鑰的字串 | `allow_secrets: true` |
| repo 是 submodule，卻改到 submodule 以外的檔案 | `allow_parent_changes: true` |

### 危險的操作會被攔下來嗎？票提到 security 或 delete 會先問我嗎？

**不會。** 沒有依關鍵字攔截的機制，也沒有 pull request 大小的上限。舊版的設定檔裡有 `escalation.triggers`、`escalation.max_single_pr_files`、`escalation.max_single_pr_lines` 這些鍵，但從來沒有程式讀它們；`awkit validate` 現在會把這種鍵列出來。

真正在把關的是：上一題的三種檢查、每個 pull request 的審查與 CI、以及你在 GitHub 上的審查意見。要讓某類變更一定經過人，用 GitHub 的 branch protection 或 CODEOWNERS。

### `gh` 的 token 怎麼管理？

用 `gh auth login`，或環境變數 `GH_TOKEN`。不要把 token 寫進設定檔或 repository。帳號要對 repository 有寫入權限：唯讀的 token 通過得了登入檢查，卻建不了 issue、合不了 pull request，`awkit doctor` 會指出來。

### `claude_code.dangerously_skip_permissions` 是什麼？

讓 Claude Code 當 Worker 時不受任何權限設定限制。預設關閉。只在隔離的機器（容器、CI runner）上打開。

---

## 使用

### 怎麼讓它停下來？

`awkit stop`：做完手上這一步就停。要立刻停，在啟動 `awkit run` 的終端機按 Ctrl-C。之後 `awkit run` 會從停下的地方接著做。

### `awkit run --dry-run` 做什麼？

只跑啟動前的檢查，不啟動流程。`awkit doctor` 做同樣的檢查，但會一次說完所有問題。

### 只想做一張票？

`awkit run --issue 42`。它只碰那一張，連它依賴的票也不碰。

### 票的順序怎麼決定？

依 issue 編號由小到大，而且要等 `depends_on` 列的 issue 都完成。

### 怎麼換 Worker？

`.ai/config.yaml` 的 `worker.backend`：`codex` 或 `claude-code`。

### 怎麼告訴 agent 我的專案的規矩？

把規則寫在 `.claude/rules/<名稱>.md`，再把名稱列進設定檔的 `rules.custom`，然後 `awkit generate`。見 [Skills 與規則](skills.md)。

### 設定檔可以放別的地方嗎？

不行，固定是 `.ai/config.yaml`。`awkit validate --config <路徑>` 可以檢查別處的檔案，但流程只讀這一份。

### 可以改標籤的名字或 commit 的格式嗎？

都不行。標籤固定是 `ai-task`、`in-progress` 等九個；commit 與 pull request 標題固定是 `[type] subject`。

### 教訓（lessons）會跟團隊共享嗎？

不會。教訓存在 `.ai/state/lessons.json`，這個目錄被 git 忽略，所以只在學到它的那份 checkout 裡。`awkit lessons promote` 可以把一條已證實的教訓變成一張 issue，請人把它寫成大家都會讀到的規則。

### log 在哪裡？

| 要看什麼 | 指令或位置 |
|---|---|
| 一張票的完整經過 | `awkit events --issue 42` |
| 一張票留下的檔案 | `awkit status --issue 42` |
| Worker 的 log | `.ai/state/logs/issue-42.worker.log` |
| Principal 的 log | `.ai/state/logs/principal.log` |
| 流程在 GitHub 上說了什麼 | `gh issue view 42 --comments` |

### 怎麼更新？

```bash
awkit check-update   # 有沒有新版 awkit
awkit upgrade        # 把這個專案的 kit 檔案更新到已安裝的 awkit
```

重新執行安裝指令就會裝上最新版的 `awkit`。

### 怎麼移除？

`awkit uninstall`：移除 kit 的規則、skill 與 subagent、`.ai/state/`、`.worktrees/`，以及設定檔與名冊（`--keep-config` 保留後兩者）。你自己放在 `.claude/` 下的東西、`CLAUDE.md` 與 `AGENTS.md` 會留著。`--dry-run` 先看會移除什麼。

---

## 其他

### 授權？

[Apache License 2.0](../../LICENSE)。

### 怎麼回報問題？

[GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues)。安全性問題請照 [SECURITY.md](../../SECURITY.md) 的方式回報。
