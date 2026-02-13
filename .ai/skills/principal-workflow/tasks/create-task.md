# Create Task

當 `awkit analyze-next` 回傳 `NEXT_ACTION=create_task` 時，代表某個 Spec 有一條尚未建立對應 GitHub Issue 的任務。

本步驟採用「兩段式」流程：
1. Principal 先把 ticket 內容寫完整（包含可驗收的條件與測試要求）
2. 再用腳本建立 Issue，並把 Issue 編號寫回 tracking source（epic body 或 tasks.md）

## Inputs

**github_epic 模式**（推薦，當 `specs.tracking.mode: github_epic`）:
- `SPEC_NAME`: Spec 名稱
- `EPIC_ISSUE`: Tracking Issue 編號
- `TASK_TEXT`: 任務文字（來自 epic body）

**tasks_md 模式**（可選，當 `specs.tracking.mode: tasks_md`）:
- `SPEC_NAME`: Spec 名稱
- `TASK_LINE`: `tasks.md` 的行號（1-based）

**github_epic 模式**（當 `specs.tracking.mode: github_epic`）:
- `SPEC_NAME`: Spec 名稱
- `EPIC_ISSUE`: Tracking Issue 編號
- `TASK_TEXT`: 任務文字（來自 epic body）

## Workflow (two-stage)

### 1) 讀取任務上下文

**epic 模式**:
- 任務文字來自 `analyze-next` 輸出的 `task_text` 欄位（從 epic body 解析）
- 讀取 `"<specs.base_path>/$SPEC_NAME/design.md"` 了解需求與架構脈絡（若存在）

**tasks_md 模式**:
- 讀取 `"<specs.base_path>/$SPEC_NAME/tasks.md"` 的第 `$TASK_LINE` 行（`specs.base_path` 由 `.ai/config/workflow.yaml` 決定，預設 `.ai/specs`）
- 讀取 `"<specs.base_path>/$SPEC_NAME/design.md"` 了解需求與架構脈絡（若存在）

### 2) 撰寫 ticket body 草稿（只寫內容，不要直接 `gh issue create`）

把 ticket body 寫入：`.ai/temp/create-task-body.md`

必備 section（標題需符合）：
- `## Summary`
- `## Scope`
- `## Acceptance Criteria`（至少一個 `- [ ]` checkbox）
- `## Testing Requirements`
- `## Metadata`

建議模板：
```markdown
## Summary
<一句話說清楚要做什麼>

## Scope
- <列出要改/要加的功能點>

## Acceptance Criteria
- [ ] <描述預期行為，而非測試函數名稱>
- [ ] <描述邊界條件處理>
- [ ] Unit tests added for new functionality
- [ ] Existing tests updated if modifying functionality
- [ ] All tests pass (`go test ./...` or equivalent)

**注意**: Acceptance Criteria 應描述「意圖」（預期行為），而非精確的測試函數名稱。Worker 自行決定測試的命名和結構。

## Testing Requirements
- New features MUST have corresponding unit tests
- Modified features MUST have updated or new test cases
- Test coverage should cover happy path and error cases

## Metadata
- **Spec**: <SPEC_NAME>
- **Task Line**: <TASK_LINE>
- **Repo**: <從 workflow.yaml 推導 / 或直接寫 root/backend/frontend>
- **Priority**: P2
- **Release**: false
```

### 3) 建立 Issue 並更新 tracking source

**epic 模式** (推薦):
```bash
awkit create-task \
  --spec "$SPEC_NAME" \
  --task-line 0 \
  --body-file .ai/temp/create-task-body.md \
  --epic-issue "$EPIC_ISSUE" \
  --task-text "$TASK_TEXT"
```

`awkit create-task` 會自動偵測 epic 模式（從 config），建立 Issue 後將 `- [ ] #N` 附加到 epic body。

**tasks_md 模式** (可選):
```bash
awkit create-task \
  --spec "$SPEC_NAME" \
  --task-line "$TASK_LINE" \
  --body-file .ai/temp/create-task-body.md
```

腳本會建立 Issue 並在 `tasks.md` 第 `$TASK_LINE` 行追加 `<!-- Issue #N -->`。

可選參數（兩種模式通用）：
- `--title "<title>"`：指定 Issue title（否則從 task line/text 自動生成）
- `--repo "<owner/repo>"`：指定 GitHub repo（若 `.ai/config/workflow.yaml` 已填 `github.repo` 可省略）
- `--dry-run`：只輸出將執行的 `gh issue create ...` 命令，不實際建立 Issue

### 4) 驗證並回到 Main Loop

- **epic 模式**: epic body 應該新增 `- [ ] #N` 條目
- **tasks_md 模式**: `tasks.md` 第 `$TASK_LINE` 行應該被追加 `<!-- Issue #N -->`
- 回到 `phases/main-loop.md` 的 Step 1，重新 `awkit analyze-next --json`

## Notes / Guardrails

- 這個 step 只負責「建立 Issue」，不要在這裡 dispatch worker 或 review PR。
- Ticket body 不可空白/模板化；Acceptance Criteria 要可測、可驗收。
- **Acceptance Criteria 不可預先指定精確的測試函數名稱**（如 `TestFooBar passes`），應描述預期行為（如 `Wall collision correctly ends the game`）。
- **epic 模式**: 若 epic body 中該 checkbox 已經 linked to issue，`awkit create-task` 會跳過避免重複。
- **tasks_md 模式**: 若 `tasks.md` 該行已存在 `<!-- Issue #N -->`，`awkit create-task` 會直接 no-op（避免重複開 Issue）。
