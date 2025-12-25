# Generate Tasks

從 spec 的 design.md 生成 tasks.md。

## 輸入

- `SPEC_NAME`: Spec 名稱（可選，若空則從 workflow.yaml 讀取 active specs）

## 步驟

1. 讀取 `.ai/config/workflow.yaml` 獲取 `specs.base_path` 和 `specs.active`
2. 對每個 active spec：
   - 讀取 `<base_path>/<spec>/design.md`
   - 如果 `tasks.md` 不存在或需要更新，生成任務清單
3. 任務格式：
   ```markdown
   - [ ] 1. 任務標題
     - 描述
     - 驗收標準
   ```

## 輸出

- 創建或更新 `<base_path>/<spec>/tasks.md`
- 回到 main-loop
