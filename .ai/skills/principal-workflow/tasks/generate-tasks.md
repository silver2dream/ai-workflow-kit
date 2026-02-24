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
2. 生成任務分解（draft）
3. **Gap Verification（必須在 create-epic 前執行）**— 見下方 Section A.1
4. 將驗證通過的 epic body 寫入 `.ai/temp/create-epic-body.md`，格式：
   ```markdown
   # <spec-name> Task Tracking

   ## Tasks

   - [ ] Task 1 description
   - [ ] Task 2 description
   - [ ] Task 3 description

   ## Progress

   This is a GitHub Tracking Issue. Checkboxes update automatically when linked issues are closed.
   ```
5. 執行：`awkit create-epic --spec "<SPEC_NAME>" --body-file .ai/temp/create-epic-body.md`
   - **REQUIRED**: `--body-file` 參數必填
   - 此命令會創建 GitHub Tracking Issue 並更新 `workflow.yaml` 的 tracking mode
6. 回到 main-loop

---

### Section A.1: Gap Verification（MANDATORY）

在步驟 2 產生 task list draft 後、寫入 body file 前，**必須**執行結構化的 gap check。

#### A.1.1 擷取 design.md 的關鍵需求

從 design.md 中提取以下維度：

| 維度 | 說明 | 範例 |
|------|------|------|
| **功能需求** | 每個 requirement / feature / user story | R1: WebSocket server, R2: Game loop |
| **技術元件** | API endpoints, services, models, configs | `POST /api/room`, `GameEngine` class |
| **涉及 Repos** | 哪些 repo 被提及 | backend, frontend |
| **整合點** | 跨 repo 或跨 module 的連接 | "frontend connects to backend WebSocket" |
| **驗證需求** | 測試、build、CI 相關 | "must pass `go test`", "npm run build" |

#### A.1.2 結構化比對清單

逐項確認 task list draft 是否覆蓋：

| # | 檢查項目 | 通過條件 |
|---|----------|----------|
| 1 | 每個功能需求有對應 task | 所有 requirement 都能映射到至少一個 task |
| 2 | 每個技術元件有建立/修改 task | 不遺漏 model/service/handler/config 的建立 |
| 3 | 每個涉及的 repo 都有 task | design 提到 frontend，task list 不可只有 backend |
| 4 | **Integration task 存在** | 若有跨 repo/module 呼叫，必須有串接 task |
| 5 | **Entry point / wiring task 存在** | 新 module 需要 registration，新 route 需要 wiring |
| 6 | Testing task 存在 | 每個 repo 至少有測試（可合併在功能 task 的 AC 中） |
| 7 | Task 順序合理 | 基礎建設 → 核心功能 → 整合串接 → 測試驗證 |

#### A.1.3 輸出 Gap Report

在 context 中輸出（不需要寫檔案）：

```
[GAP CHECK] design.md requirements: N items
[GAP CHECK] task list coverage: M items
[GAP CHECK] result: PASS | FAIL

若 FAIL:
- GAP: <requirement> — 沒有對應 task
- GAP: Integration — <repo_A>/<repo_B> 串接缺少 wiring task
- GAP: Entry point — <module> 缺少 registration/bootstrap task
- GAP: Repo — <repo> 在 design.md 中被提及但 task list 沒有覆蓋
```

#### A.1.4 修正流程

- **PASS** → 繼續到步驟 4
- **FAIL** →
  1. 補充遺漏的 task 到 task list draft
  2. 重新執行 A.1.2 比對
  3. **最多 2 次修正迭代**（避免無限循環）
  4. 第 3 次仍 FAIL → 繼續執行，但在 epic body 末尾加上：
     `<!-- GAP_WARNING: gaps detected, manual review recommended -->`

---

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
