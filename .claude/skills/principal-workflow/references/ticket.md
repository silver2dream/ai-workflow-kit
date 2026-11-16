# Ticket

票券是一個帶 task 標籤（預設 `ai-task`）的 GitHub issue。主迴圈只處理已存在的票券，不自己開票。一批新工作由 `awkit plan` 規劃，核可之後由它建票；單獨補一張票時，照這份文件寫內文，再用 `awkit create-task` 建立。

## 1. 寫內文

把內文寫入一個檔案，例如 `.ai/state/tmp/ticket.md`。不要直接 `gh issue create`：`awkit create-task` 會先檢查內文，不合格就拒絕建立。

內文必須有：

- `## Summary`
- `## Scope`
- `## Acceptance Criteria`（至少一個 `- [ ]` checkbox）
- `## Testing Requirements`
- 一個 `yaml awk-ticket` 區塊：工作流程從這裡讀取這張票給哪個 repo、要等哪些票先完成

模板：

````markdown
## Summary
<一句話說清楚要做什麼>

## Scope
- <列出要改/要加的功能點>

## Acceptance Criteria
- [ ] <描述預期行為，而非測試函數名稱>
- [ ] <描述邊界條件處理>
- [ ] Unit tests added for new functionality
- [ ] Existing tests updated if modifying functionality
- [ ] All tests pass

## Testing Requirements
- New features MUST have corresponding unit tests
- Modified features MUST have updated or new test cases
- Test coverage should cover happy path and error cases

```yaml awk-ticket
schema: 1
repo: backend
depends_on: [12, 15]
```
````

### `yaml awk-ticket` 區塊

| 欄位 | 必填 | 說明 |
|------|------|------|
| `schema` | 是 | 固定寫 `1` |
| `repo` | 是 | `.ai/config.yaml` 裡的 repo 名稱，或 `root`（repo 根目錄）。Worker 依它決定在哪裡工作、跑哪一組 verify 指令 |
| `depends_on` | 否 | 必須先完成的 Issue 編號清單。沒有就整行省略 |
| `severity` | 否 | `P0`、`P1`、`P2` 或 `P3` |
| `release` | 否 | `true` 表示 PR 送往 release 分支；只有 `repo: root` 的票可以設 |
| `allow_script_changes` | 否 | `true` 才允許 Worker 修改工作流自身的檔案（見下方） |
| `allow_parent_changes` | 否 | `true` 才允許 submodule 的票修改 submodule 以外的檔案 |
| `allow_secrets` | 否 | `true` 才允許 diff 中出現被判定為敏感資訊的內容 |

區塊會被嚴格讀取，內文其他地方的文字不會被當成設定：

- 一張票只能有一個區塊。未知或拼錯的欄位、讀不懂的值、設定檔裡沒有的 repo，都會讓這張票被拒絕。`awkit create-task` 在建立前就會擋下；如果是直接在 GitHub 上寫壞的，`awkit analyze-next` 會在 Issue 留言說明原因、加上 `needs-human-review` 標籤，並以 `invalid_ticket` 停下。
- `depends_on` 裡的每個 Issue 都以 completed 關閉後，這張票才會被派工。票券依編號由小到大處理，還在等的票會被跳過。
- 被依賴的 Issue 必須已經存在，寫它的實際編號。它如果以 not planned 關閉，這張票永遠不會開始；當所有待辦的票都在等，流程會以 `blocked_by_dependencies` 停下。

撰寫要點：

- **Acceptance Criteria 描述意圖**（預期行為），不是精確的測試函數名稱；測試怎麼命名、怎麼組織由 Worker 決定。審查時每一條都要對應到一個實際跑過的測試。
- **`repo` 一定要填對**：填錯會讓 Worker 在錯的地方工作，或讓票被拒絕。
- 票券要能獨立讀懂：需要參考的文件，寫出路徑，讓 Worker 自己去讀。

**`allow_script_changes`**：runner 預設會拒絕 Worker 對工作流自身檔案的修改——整個 `.claude/`（規則、技能、subagent、設定）、`.ai/config.yaml`、`.ai/roster.yaml`、`CLAUDE.md`、`AGENTS.md`、`.mcp.json`——因為這些檔案決定 Worker 如何被派工與審查。只有當任務本身就是要修改它們時，才在區塊裡加上：

```yaml
allow_script_changes: true
```

## 2. 建立

```bash
awkit create-task \
  --title "[feat] <簡短標題>" \
  --body-file .ai/state/tmp/ticket.md
```

- `--title` 必填，用專案的 commit 格式（`[type] subject`）。
- `--dry-run` 只檢查內文並印出將執行的 `gh` 指令，不建立 Issue。
- 成功時輸出 `Created Issue #<N>`。Issue 會帶 task 標籤，下一輪 `awkit analyze-next` 就會看到它。
- 一次建立多張有依賴關係的票時，先建立被依賴的那張，拿到編號後再寫依賴它的票的 `depends_on`。

## 3. 被拒絕時

- 內文缺 section、沒有 checkbox、沒有區塊或區塊有誤：指令以非零結束並說明原因，修正後重跑。
- 建立後不需要再做任何登記；Issue 本身就是唯一的工作來源。

## 舊格式的票

區塊出現之前建立的票沒有 `yaml awk-ticket` 區塊。它們仍然可以執行：Worker 會照舊從內文讀取（`- **Repo**: backend`、`## Constraints` 下的 `allow-script-changes` 等），沒有依賴關係，並在事件紀錄裡留下一筆 `deprecated_path`。新的票一律要有區塊；要讓舊票改用區塊，直接編輯 Issue 內文加上即可。
