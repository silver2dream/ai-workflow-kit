# Audit Epic Review

當 `awkit analyze-next` 回傳 `NEXT_ACTION=audit_epic` 時，執行 Epic 覆蓋度稽核。

## 步驟

### 1. 執行 Audit

```bash
awkit audit-epic --spec "$SPEC_NAME"
```

輸出為 JSON，包含：
- `tasks`: 目前 Epic 中的所有 task 及其狀態
- `design_sections`: design.md 的 H2/H3 標題
- `design_requirements`: 擷取的需求標籤 (R1, R2...)
- `gap_hints`: 機器偵測到的潛在缺口
- `suggested_action`: "ok" | "gaps_detected"

### 2. 分析 Gap Hints

逐一檢查 `gap_hints`：

| Hint | 意義 | 行動 |
|------|------|------|
| `REPO_UNCOVERED:<repo>` | design.md 提及該 repo 但 task list 中沒有 | 補充該 repo 的 task |
| `REQ_UNCOVERED:<req>` | 需求標籤在 task list 中找不到對應 | 補充該需求的 task |
| `LOW_TASK_COUNT` | task 數量少於 design 的 section 數 | 檢查是否有合併過度 |
| `MISSING_INTEGRATION_TASK` | 多 repo 但沒有整合 task | 補充 wiring/integration task |
| `NO_DESIGN_FILE` | design.md 不存在 | 無法分析，跳過 |

### 3. 如果 suggested_action == "ok"

直接回到 main-loop Step 1，無需動作。

### 4. 如果 suggested_action == "gaps_detected"

1. 讀取 `<specs.base_path>/<spec>/design.md` 作為參考
2. 比對 gap_hints 與 design.md 內容，判斷哪些是真正的缺口
3. 對於確認的缺口，產生補充 task 文字（每個 task 一行 `- [ ] <description>`）
4. 使用 `awkit append-epic-tasks` 更新 Epic body（若不可用，使用以下方式）：
   ```bash
   gh issue view <epic_issue> --json body --jq .body > /tmp/epic-body.md
   # 手動附加新 task 行到 Tasks section
   gh issue edit <epic_issue> --body-file /tmp/epic-body.md
   ```
5. 回到 main-loop Step 1

### 5. 限制

- 每次 audit 最多補充 **5 個 task**（避免過度膨脹）
- 補充的 task 應聚焦在 **結構性缺口**（缺少整個 repo 的任務、缺少整合任務），而非細粒度拆分
- **不要修改**已完成或正在進行的 task
- 如果 gap_hints 只有 `NO_DESIGN_FILE`，直接跳過
