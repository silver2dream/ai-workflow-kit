# Create Task

從 tasks.md 的未完成任務創建 GitHub Issue。

## 輸入

- `SPEC_NAME`: Spec 名稱（必填）
- `TASK_LINE`: 任務行號（必填）

## 步驟

1. 讀取 `<base_path>/<SPEC_NAME>/tasks.md` 第 `TASK_LINE` 行
2. 讀取 `<base_path>/<SPEC_NAME>/design.md` 獲取相關設計
3. 生成 Issue body，格式：
   ```markdown
   ## Summary
   <任務描述>

   ## Scope
   - <具體範圍>

   ## Acceptance Criteria
   - [ ] <驗收標準>

   ## Metadata
   - **Spec**: <SPEC_NAME>
   - **Repo**: <從 design.md 或 workflow.yaml 推斷>
   - **Priority**: P2
   - **Release**: false
   ```
4. 創建 Issue：
   ```bash
   gh issue create --title "<任務標題>" --body "<body>" --label "ai-task"
   ```
5. 更新 tasks.md，在該行添加 `<!-- Issue #N -->`

## 輸出

- GitHub Issue 已創建
- tasks.md 已更新
- 回到 main-loop
