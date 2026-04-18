---
name: principal-workflow
description: Run the AWK principal workflow (awkit kickoff, principal loop, dispatch worker, check results, review/merge PR). Triggers: awkit kickoff, start-work, NEXT_ACTION, review pr, dispatch worker, autonomous workflow, generate tasks, create task.
allowed-tools: Read, Grep, Glob, Bash
---

# Principal Workflow

AWK 自動化工作流的主控 Skill。

## 前提

Preflight 已由 `awkit kickoff` 完成，Session 已初始化。

## 啟動

**必須 Read** `phases/main-loop.md` 並進入主循環。

## Phase 參考表

| Phase 檔案 | 說明 |
|------------|------|
| `phases/main-loop.md` | 主循環：決策路由 + loop safety + context management |
| `phases/dispatch.md` | dispatch_worker：命令格式、merge_issue 檢查、行為規範、結果處理 |
| `phases/review.md` | review_pr：Task tool 調用 pr-reviewer subagent |
| `phases/conflict-resolution.md` | needs_conflict_resolution：Task tool 調用 conflict-resolver subagent |

| Task 檔案 | 說明 |
|-----------|------|
| `tasks/generate-tasks.md` | 任務生成（Epic 或 tasks.md 模式） |
| `tasks/create-task.md` | Issue 創建 |
| `tasks/audit-epic-review.md` | Epic 審計結果審查 |

## 自我檢查

每進入一個 Phase 或執行一個 Task，輸出：
```
[PRINCIPAL] <timestamp> | <phase/task> | loaded: <filename>
```
