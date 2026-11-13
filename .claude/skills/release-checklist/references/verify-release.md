# Verify Release

每一項各自判定 PASS、WARN 或 FAIL。下面的 `<integration>` 與 `<release>` 是 `.ai/config.yaml` 的 `git.integration_branch` 與 `git.release_branch`。

## 1. 建置與測試

對 `.ai/config.yaml` 的每一個 repo，在它的 `path` 下執行它的 `verify.build` 與 `verify.test`。

- PASS：每個指令都以 0 結束
- WARN：某個 repo 沒有設定指令
- FAIL：任何一個指令非 0；附上失敗的輸出

## 2. 工作目錄

```bash
git status --porcelain
```

- PASS：沒有輸出
- WARN：只有未追蹤的檔案
- FAIL：有改過或已 staged 的追蹤檔案

## 3. 版本與變更紀錄

專案怎麼標版本，就查那裡：tag、版本檔、變更紀錄。

```bash
git tag --sort=-version:refname | head -5
git describe --tags --abbrev=0
ls CHANGELOG* 2>/dev/null
```

- PASS：這次要發的版本號確定，還沒有同名的 tag，變更紀錄有這一版
- WARN：沒有變更紀錄，或沒有這一版的條目
- FAIL：版本號在不同地方互相矛盾，或要發的版本已經有 tag

## 4. 相依套件

用專案語言的工具查已知弱點。例：

```bash
go mod verify && govulncheck ./...                       # Go；govulncheck 沒裝就記 WARN
dotnet list package --vulnerable --include-transitive   # .NET
```

- PASS：沒有已知弱點
- WARN：只有低風險的，或這台機器沒有掃描工具
- FAIL：有高風險或重大的弱點

## 5. CI

```bash
gh run list --branch <integration> --limit 5 --json status,conclusion,name,headSha,url
git rev-parse HEAD
```

- PASS：`HEAD` 這個 commit 最近一次執行是 `success`
- WARN：沒有任何執行，或還在跑（記下來，不等）
- FAIL：最近一次是 `failure` 或 `cancelled`，或成功的那次不是 `HEAD` 這個 commit

## 6. 與 release 分支的差距

```bash
git fetch origin <release>
git rev-list --count origin/<release>..HEAD     # 這次要發出去的 commit 數
git rev-list --count HEAD..origin/<release>     # release 分支有、這裡沒有的 commit 數
git merge-tree --write-tree --name-only HEAD origin/<release>   # 以 1 結束表示有衝突，並列出檔案
```

- PASS：要發出去的 commit 至少 1 個，合併沒有衝突
- WARN：release 分支有這裡沒有的 commit，但合併沒有衝突（常見於 release 分支用 squash 合併）
- FAIL：合併有衝突，或沒有任何 commit 要發

## Decision

| 情況 | 判定 |
|---|---|
| 全部 PASS | GO |
| 只有 WARN | GO，列出要注意的事 |
| 任何一項 FAIL | NO-GO |

## Report template

```markdown
# Release Verification

- 日期：<YYYY-MM-DD>
- 分支：<integration> → <release>
- Commit：<SHA>

| # | 檢查 | 結果 | 依據 |
|---|---|---|---|
| 1 | 建置與測試 | <PASS / WARN / FAIL> | <指令與結果> |
| 2 | 工作目錄 | | |
| 3 | 版本與變更紀錄 | | |
| 4 | 相依套件 | | |
| 5 | CI | | |
| 6 | 與 release 分支的差距 | | |

## 判定：<GO | NO-GO>

### 阻擋項
1. <哪一項、失敗的輸出、怎麼修>

### 要注意的事
1. <WARN 的內容>

### 接下來
- <步驟>
```

GO 之後的步驟由專案的發版方式決定。由工作流程發版時，開一張 `repo: root`、`release: true` 的票（格式見 `.claude/skills/principal-workflow/references/ticket.md`），它的 PR 才會以 release 分支為目標。
