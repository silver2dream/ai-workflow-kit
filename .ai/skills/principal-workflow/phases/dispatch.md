# Dispatch Worker

## CRITICAL: dispatch_worker 命令格式（MANDATORY CHECK）

當 `next_action` 為 `dispatch_worker` 時，**必須**按以下步驟執行：

**Step 1: 檢查 merge_issue 欄位（不可跳過）**

從 JSON 輸出中讀取 `merge_issue` 的值。

**Step 2: 根據 merge_issue 選擇命令格式**

| merge_issue 值 | 命令格式 |
|---------------|----------|
| `conflict` 或 `rebase` | `awkit dispatch-worker --issue <N> --merge-issue <merge_issue> --pr <pr_number>` |
| 空或不存在 | `awkit dispatch-worker --issue <N>` |

**範例**：
```json
{"next_action": "dispatch_worker", "issue_number": 27, "pr_number": 30, "merge_issue": "conflict"}
```
→ 執行：`awkit dispatch-worker --issue 27 --merge-issue conflict --pr 30`

⚠️ **WARNING**: 忽略 merge_issue 會導致 merge conflict/rebase 無法修復，造成無限循環！

## CRITICAL: dispatch_worker 行為規範

執行 `awkit dispatch-worker` 時：
1. **命令是同步的** - 會等待 Worker 完成才返回
2. **不要讀取 log 檔案** - 這會浪費 context
3. **不要監控進度** - 命令會處理一切
4. **不要輸出 Worker 狀態描述** - 等命令完成即可
5. **執行完成後，檢查 WORKER_STATUS 並處理（見下方）**

## dispatch_worker 結果處理

執行 `awkit dispatch-worker` 後，解析輸出的 `WORKER_STATUS`：

| WORKER_STATUS | 動作 |
|---------------|------|
| `success` | 回到 Step 1 |
| `failed` | 回到 Step 1（下輪會重試或標記 worker-failed） |
| `needs_conflict_resolution` | **Read** `phases/conflict-resolution.md` 並執行 |

處理完成後，回到 main-loop Step 1。
