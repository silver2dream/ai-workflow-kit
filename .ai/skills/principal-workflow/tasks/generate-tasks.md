# Generate Tasks

從 spec 的 design.md 生成任務清單（推薦使用 GitHub Epic，或 tasks.md）。

## 輸入

- `SPEC_NAME`: Spec 名稱（可選，若空則從 workflow.yaml 讀取 active specs）

## 步驟

### 1. 讀取配置

讀取 `.ai/config/workflow.yaml` 獲取：
- `specs.base_path`
- `specs.active`
- `specs.tracking.mode` (github_epic | tasks_md)

### 2. 根據 Tracking Mode 執行

#### A. Epic Mode (github_epic, RECOMMENDED)

**Epic Mode 是推薦的任務追蹤方式**，適用於需要自動進度更新和更好可見性的項目。

當 `specs.tracking.mode` 為 `github_epic` 時：

1. 讀取 `<base_path>/<spec>/design.md`
2. 生成任務分解
3. 將 epic body 寫入 `.ai/temp/create-epic-body.md`，格式：
   ```markdown
   # <spec-name> Task Tracking

   ## Tasks

   - [ ] Task 1 description
   - [ ] Task 2 description
   - [ ] Task 3 description

   ## Progress

   This is a GitHub Tracking Issue. Checkboxes update automatically when linked issues are closed.
   ```
4. 執行：`awkit create-epic --spec "<SPEC_NAME>" --body-file .ai/temp/create-epic-body.md`
   - **REQUIRED**: `--body-file` 參數必填
   - 此命令會創建 GitHub Tracking Issue 並更新 `workflow.yaml` 的 tracking mode
5. 回到 main-loop

#### B. Tasks.md Mode (tasks_md, 可選)

**Tasks.md Mode 仍受支持**，適用於輕量級本地追蹤或 analyzer 的 tasks_md 模式。

當 `specs.tracking.mode` 為 `tasks_md` 時：

1. 對每個 active spec：
   - 讀取 `<base_path>/<spec>/design.md`
   - 如果 `tasks.md` 不存在或需要更新，生成任務清單
2. 任務格式：
   ```markdown
   - [ ] 1. 任務標題
     - 描述
     - 驗收標準
   ```
3. 創建或更新 `<base_path>/<spec>/tasks.md`
4. 回到 main-loop

## 輸出

- **Epic Mode**: 創建 GitHub Tracking Issue，更新 workflow.yaml
- **Tasks.md Mode**: 創建或更新 `<base_path>/<spec>/tasks.md`
