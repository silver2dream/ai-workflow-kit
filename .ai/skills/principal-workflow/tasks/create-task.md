# 票券格式與建立

票券就是一個帶有 task 標籤（預設 `ai-task`）的 GitHub Issue。主迴圈只處理已存在的票券，不會自己產生票券；需要新增工作時，照這份文件寫出票券內文，再用 `awkit create-task` 建立。

## 1) 撰寫 ticket body

把內文寫入一個檔案，例如 `.ai/temp/ticket.md`。不要直接 `gh issue create`：`awkit create-task` 會先檢查內文，缺必備 section 就拒絕建立。

必備 section（標題需完全符合）：

- `## Summary`
- `## Scope`
- `## Acceptance Criteria`（至少一個 `- [ ]` checkbox）
- `## Testing Requirements`
- `## Metadata`

模板：

```markdown
## Summary
<一句話說清楚要做什麼>

## Dependencies
- Depends on: #<issue-number> (<說明>)

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

## Metadata
- **Repo**: <workflow.yaml 裡的 repo 名稱，例如 root / backend / frontend>
- **Priority**: P2
- **Release**: false
```

撰寫要點：

- **Acceptance Criteria 描述意圖**（預期行為），不是精確的測試函數名稱；測試怎麼命名、怎麼組織由 Worker 決定。審查時每一條都要對應到一個實際跑過的測試。
- **Repo 一定要填對**：Worker 依它決定在哪個 repo 工作、跑哪一組 verify 指令。
- **Dependencies** 沒有就整段省略。有的話，先確認被依賴的 Issue 已經存在，寫它的編號。
- 票券要能獨立讀懂：需要參考的文件，寫出路徑，讓 Worker 自己去讀。

**Constraints section（可選）**：runner 預設會拒絕 Worker 對工作流自身檔案的修改——`.ai/config/`、`.ai/rules/`、`.ai/skills/`、`.claude/agents/`、`CLAUDE.md`、`AGENTS.md`——因為這些檔案決定 Worker 如何被派工與審查。只有當任務本身就是要修改它們時，才在 ticket 加上：

```markdown
## Constraints
- allow-script-changes
```

## 2) 建立 Issue

```bash
awkit create-task \
  --title "[feat] <簡短標題>" \
  --body-file .ai/temp/ticket.md
```

- `--title` 必填，用專案的 commit 格式（`[type] subject`）。
- `--dry-run` 只檢查內文並印出將執行的 `gh` 指令，不建立 Issue。
- 成功時輸出 `Created Issue #<N>`。Issue 會帶 task 標籤，下一輪 `awkit analyze-next` 就會看到它。

## 3) 驗證

- 內文缺 section 或沒有 checkbox：指令以非零結束並列出缺少的項目，補齊後重跑。
- 建立後不需要再做任何登記；Issue 本身就是唯一的工作來源。
