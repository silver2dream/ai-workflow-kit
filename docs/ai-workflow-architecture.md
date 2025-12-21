# AI Autonomous Workflow Architecture

> Version: 1.2
> Last Updated: 2025-01-04
> Status: Implementation Complete

---

## Documentation Navigation

本文件是 AWK 的高層架構說明。更多詳細文件請參考：

### For Users
- [Getting Started](user/getting-started.md) - 快速開始
- [Configuration](user/configuration.md) - 配置說明
- [Troubleshooting](user/troubleshooting.md) - 故障排除
- [FAQ](user/faq.md) - 常見問題

### For Developers
- [Architecture](developer/architecture.md) - 系統內部架構
- [API Reference](developer/api-reference.md) - 腳本與模組 API
- [Contributing](developer/contributing.md) - 貢獻指南
- [Testing](developer/testing.md) - 測試說明

---

## 1. Executive Summary

### 1.1 Goal

實現「睡前啟動，早上收割」的全自動 AI 開發工作流，形成完整閉環。

### 1.2 Key Principles

| Principle | Description |
|-----------|-------------|
| **零額外 API 成本** | 使用已訂閱的 Claude Code Pro + Codex Business |
| **Sequential Chain** | 同步阻塞調用，不需要輪詢或監聽進程 |
| **GitHub 作為狀態機** | Issues/PRs 可視化追蹤進度 |
| **容錯閉環** | 失敗自動重試，超過閾值停止並通知 |

---

## 2. Architecture Overview

### 2.1 High-Level Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                      │
│    [你]  ──"開始工作"──►  [Claude Code]  ──analyze──►  [GitHub Issue] │
│                              (Principal)                             │
│                                  │                                   │
│                                  │ dispatch                          │
│                                  ▼                                   │
│                              [Codex]  ──────────►  [PR Created]      │
│                              (Worker)                                │
│                                  │                                   │
│                                  │ signal: done                      │
│                                  ▼                                   │
│                          [Claude Code]  ──review──►  [Merge/Reject]  │
│                              (Reviewer)                              │
│                                  │                                   │
│                                  │ if reject                         │
│                                  ▼                                   │
│                          [Create Fix Issue]  ───►  [Loop Back]       │
│                                                                      │
│    [你睡覺中...]                                                      │
│                                                                      │
│    [早上] ──► gh pr list ──► 收割成果                                 │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         LOCAL MACHINE                                │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    Claude Code (Pro Plan)                     │   │
│  │                                                               │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │   │
│  │  │  Analyzer   │  │  Dispatcher │  │  Reviewer   │          │   │
│  │  │             │  │             │  │             │          │   │
│  │  │ - Read specs│  │ - Call Codex│  │ - gh pr diff│          │   │
│  │  │ - Decide    │  │ - Monitor   │  │ - Approve/  │          │   │
│  │  │   next task │  │   result    │  │   Reject    │          │   │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘          │   │
│  │         │                │                │                  │   │
│  │         └────────────────┼────────────────┘                  │   │
│  │                          │                                   │   │
│  │                    Event Router                              │   │
│  │                   (Sequential Chain)                         │   │
│  │                          │                                   │   │
│  └──────────────────────────┼───────────────────────────────────┘   │
│                             │                                        │
│  ┌──────────────────────────┼───────────────────────────────────┐   │
│  │                    Codex (Business Plan)                      │   │
│  │                          │                                    │   │
│  │  ┌───────────────────────▼───────────────────────────────┐   │   │
│  │  │                    Worker                              │   │   │
│  │  │                                                        │   │   │
│  │  │  - codex exec                                          │   │   │
│  │  │  - Implement code                                      │   │   │
│  │  │  - Create PR                                           │   │   │
│  │  │  - Write result.json                                   │   │   │
│  │  └────────────────────────────────────────────────────────┘   │   │
│  │                                                               │   │
│  └───────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐   │
│  │                    .ai/ (State Store)                         │   │
│  │                                                               │   │
│  │  state/            results/           runs/                   │   │
│  │  ├── STOP          ├── issue-1.json   ├── issue-1/           │   │
│  │  ├── audit.json    ├── issue-2.json   │   ├── prompt.txt     │   │
│  │  └── repo_scan.json└── ...            │   └── summary.txt    │   │
│  │                                       └── ...                 │   │
│  └───────────────────────────────────────────────────────────────┘   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ gh CLI
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         GITHUB (State Machine)                        │
│                                                                       │
│  Issues                          PRs                                  │
│  ┌─────────────────────┐        ┌─────────────────────┐              │
│  │ #1 [ai-task]        │───────►│ PR #10              │              │
│  │ #2 [ai-task][fix]   │        │ PR #11              │              │
│  │ #3 [ai-task]        │        │ ...                 │              │
│  └─────────────────────┘        └─────────────────────┘              │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
```

---

## 3. Sequential Chain 執行模式

### 3.1 為什麼選擇 Sequential Chain？

| 方案 | 複雜度 | 優點 | 缺點 |
|------|--------|------|------|
| File Watch | 高 | 可並行 | 需要額外工具 |
| Named Pipe | 中 | 進程間通訊 | 需要管理 FIFO |
| **Sequential Chain** ✅ | **低** | **最簡單，零依賴** | 串行執行 |

我們採用 **Sequential Chain**，因為：
- 不需要額外的監聽進程或工具
- Claude Code 本身就是一個長時間運行的 session
- `run_issue_codex.sh` 是同步阻塞的，完成後直接返回結果
- 對於「睡前啟動，早上收割」的場景，串行執行已經足夠

### 3.2 執行流程

```
┌─────────────────────────────────────────────────────────────┐
│                                                              │
│  Claude Code Session (Long-running)                          │
│                                                              │
│  while has_pending_work():                                   │
│      │                                                       │
│      ├─► [1] Analyze: 決定下一個任務                          │
│      │                                                       │
│      ├─► [2] Create Issue: gh issue create                   │
│      │                                                       │
│      ├─► [3] Dispatch: bash run_issue_codex.sh (blocking)    │
│      │       │                                               │
│      │       └─► Codex 執行...                               │
│      │       └─► 寫入 result.json                            │
│      │       └─► 返回                                        │
│      │                                                       │
│      ├─► [4] Check Result: 讀取 result.json                  │
│      │       │                                               │
│      │       ├─► success: 繼續審查                            │
│      │       └─► failed: 重試或標記失敗                       │
│      │                                                       │
│      ├─► [5] Review: gh pr diff + 判斷                       │
│      │       │                                               │
│      │       ├─► approve: gh pr merge, close issue           │
│      │       └─► reject: request changes + requeue           │
│      │                                                       │
│      └─► [6] Loop: 回到 [1]                                  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### 3.3 關鍵設計決策

| 決策 | 說明 |
|------|------|
| **同步阻塞調用** | `run_issue_codex.sh` 完成才返回，不需要輪詢 |
| **文件作為狀態** | `result.json` 記錄執行結果，可靠且可追溯 |
| **Claude Code 作為 Orchestrator** | 不需要獨立的調度進程 |
| **GitHub Labels 作為狀態機** | 可視化追蹤 Issue 生命週期 |

---

## 4. Detailed Component Design

### 4.1 Claude Code Commands

#### `/start-work` - 主入口

```markdown
# .ai/commands/start-work.md

你是 Principal Engineer。執行完整的自動化工作流循環。

## 主循環

while True:
    1. 檢查 pending issues: `gh issue list --label ai-task --state open`
    2. 如果沒有 pending:
       - 若 tasks.md 不存在但 design.md 存在，先生成 tasks.md
       - 分析 <specs.base_path>/<active_spec>/tasks.md
       - 找出未完成任務
       - 創建新 Issue（含完整 ticket 模板）
    3. 選擇優先級最高的 issue
    4. 執行 Worker: `bash .ai/scripts/run_issue_codex.sh <id> <file>`
    5. 讀取結果: `cat .ai/results/issue-<id>.json`
    6. 如果成功且有 PR:
       - 審查: `gh pr diff <pr_number>`
       - 判斷是否符合 .ai/rules/*.md
       - 通過: `gh pr merge --squash --delete-branch --auto`
       - 拒絕: request changes + requeue issue
    7. 繼續下一輪

## 停止條件
- 所有任務完成
- 連續失敗 3 次
- 檢測到 .ai/state/STOP 文件
```

### 4.2 Worker Integration

```bash
# .ai/scripts/run_issue_codex.sh
# 這是現有腳本，已經實現了：
# - 創建 worktree
# - 執行 codex exec
# - 提交代碼
# - 創建 PR
# - 寫入 result.json

# 關鍵：它是同步執行的，返回後 result.json 已就緒
```

### 4.3 Result Schema

```json
// .ai/results/issue-42.json
{
  "issue_id": "42",
  "status": "success",           // success | failed
  "repo": "backend",             // root | backend | frontend
  "branch": "feat/ai-issue-42",
  "base_branch": "<integration_branch>",
  "head_sha": "abc123...",
  "timestamp_utc": "2024-12-18T15:30:00Z",
  "pr_url": "https://github.com/.../pull/123",
  "summary_file": ".ai/runs/issue-42/summary.txt"
}
```

---

## 5. State Management

### 5.1 GitHub Labels as State

```
Issue 生命週期:

  created ──► [ai-task] ──► [in-progress] ──► [pr-ready]
                                │                  │
                                │                  ▼
                                │            [reviewing]
                                │                  │
                                │         ┌───────┴───────┐
                                │         ▼               ▼
                                │   [review-pass]   [review-fail]
                                │         │               │
                                │         ▼               ▼
                                │     [closed]     [requeue]
                                │                        │
                                ▼                        │
                          [worker-failed]                │
                                │                        │
                                └──────── 人工介入 ◄──────┘
```

### 5.2 Local State Files

```
.ai/
├── state/
│   ├── orchestrator_state.json    # 當前執行狀態
│   ├── repo_scan.json             # 最近一次掃描
│   ├── audit.json                 # 最近一次審計
│   └── STOP                       # 存在則停止（touch 創建）
├── results/
│   └── issue-*.json               # 每個 issue 的執行結果
├── runs/
│   └── issue-*/
│       ├── prompt.txt             # 發給 Codex 的 prompt
│       ├── summary.txt            # 執行摘要
│       └── fail_count.txt         # 失敗計數
└── exe-logs/
    └── issue-*.codex.log          # Codex 詳細日誌
```

---

## 6. Error Handling

### 6.1 Retry Strategy

```
失敗處理流程:

Worker 執行
    │
    ▼
檢查 result.json
    │
    ├─► status: success ──► 繼續審查
    │
    └─► status: failed
            │
            ▼
        檢查 fail_count.txt
            │
            ├─► count < 3 ──► 重試
            │
            └─► count >= 3
                    │
                    ▼
              標記 [worker-failed]
              停止此 issue
              人工介入
```

### 6.2 Review Rejection Flow

```
審查拒絕:

gh pr review --request-changes
    │
    ▼
創建 fix issue:
  - Title: [fix] Review feedback for #<original>
  - Body: 包含拒絕原因
  - Labels: ai-task, fix, priority-P1
    │
    ▼
下一輪循環會處理這個 fix issue
```

### 6.3 Graceful Stop

```bash
# 方法 1: 創建停止標記
touch .ai/state/STOP

# 方法 2: 在 Claude Code 中說 "停止工作"

# 方法 3: Ctrl+C（會完成當前步驟後停止）
```

---

## 7. Cost Analysis

### 7.1 Zero Extra API Cost

| Component | What's Used | Cost |
|-----------|-------------|------|
| Claude Code | Pro Plan subscription | $0 extra |
| Codex | Business Plan subscription | $0 extra |
| gh CLI | Free | $0 |
| GitHub | Free tier / existing plan | $0 |

### 7.2 Comparison with API Approach

| Approach | Monthly Cost Estimate |
|----------|----------------------|
| Claude API + OpenAI API | ~$50-200+ |
| Claude Code Pro + Codex Business | **Already subscribed** |

---

## 8. Implementation Checklist

### Phase 1: Foundation ✅
- [x] scan_repo.sh
- [x] audit_project.sh
- [x] audit_to_tickets.sh (manual utility, not in main flow)
- [x] run_issue_codex.sh
- [x] write_result.sh
- [x] CLAUDE.md, AGENTS.md
- [x] .claude/rules/*.md

### Phase 2: Commands ✅
- [x] .claude/commands/start-work.md
- [x] .claude/commands/analyze-next.md
- [x] .claude/commands/dispatch-worker.md
- [x] .claude/commands/review-pr.md
- [x] .claude/commands/stop-work.md

### Phase 3: Integration ✅
- [x] kickoff.sh - 一鍵啟動腳本
- [x] stats.sh - 統計報告
- [x] notify.sh - 通知機制
- [ ] 端到端測試（手動驗證）

### Manual Utilities (Optional)

These scripts are not part of the automatic workflow loop, but can be used manually:
- `audit_to_tickets.sh` - convert audit findings into issue tickets
- `cleanup.sh` - remove old worktrees/branches/results
- `stats.sh` - generate workflow statistics (text/json/html)
- `notify.sh` - send system/Slack/Discord notifications

### Phase 4: Polish ✅
- [x] 通知機制（系統通知 + Slack/Discord 可選）
- [x] 統計儀表板（text/json/html 格式）
- [x] 文檔完善

---

## 9. Quick Start

```bash
# 1. 確認環境（kickoff.sh 會自動檢查）
claude --version    # Claude Code CLI
codex --version     # Codex CLI
gh auth status      # GitHub 認證

# 2. 啟動工作流
# 方式 A: 一鍵啟動（推薦）
bash .ai/scripts/kickoff.sh

# 方式 B: 背景執行（睡前啟動）
bash .ai/scripts/kickoff.sh --background

# 方式 C: 只做前置檢查
bash .ai/scripts/kickoff.sh --dry-run

# 方式 D: 手動進入 Claude Code
claude
> /start-work

# 3. 查看進度
bash .ai/scripts/stats.sh              # 統計報告
bash .ai/scripts/stats.sh --json       # JSON 格式
bash .ai/scripts/stats.sh --html       # 生成 HTML 報告
gh issue list --label ai-task         # GitHub Issues
gh pr list                            # GitHub PRs

# 4. 停止（如果需要）
touch .ai/state/STOP
# 或在 Claude Code 中說 "停止工作流"
# 或執行 /stop-work

# 5. 手動發送通知
bash .ai/scripts/notify.sh "標題" "內容"
bash .ai/scripts/notify.sh --summary   # 發送統計摘要
```

### 通知配置（可選）

```bash
# Slack 通知
export AI_SLACK_WEBHOOK="https://hooks.slack.com/services/xxx/yyy/zzz"

# Discord 通知
export AI_DISCORD_WEBHOOK="https://discord.com/api/webhooks/xxx/yyy"

# 禁用系統通知
export AI_SYSTEM_NOTIFY=false
```

---

## 10. FAQ

### Q: 為什麼不用 GitHub Actions 執行 Claude/Codex?
A: GitHub Actions 執行 Claude/Codex 需要 API，會產生額外成本。我們使用本地 CLI（已訂閱）來避免這個成本。

### Q: 那 frontend/backend 的 CI workflow 是做什麼的?
A: CI workflow（`backend-ci.yml`、`frontend-ci.yml`）是**獨立於 AI workflow 的安全網**：
- Codex 本地執行時已跑過驗證命令，Sequential Chain 不等待 CI
- CI 作為 branch protection 的 required check，`gh pr merge` 會被擋住如果 CI 沒過
- 這是最後一道防線，防止本地驗證漏掉的問題

### Q: 如何保證不會無限循環?
A:
1. fail_count 限制每個 issue 最多重試 3 次
2. STOP 文件可以隨時停止
3. 連續失敗檢測會自動暫停

### Q: 可以並行執行嗎?
A: 當前設計是串行的（一次一個 issue）。並行需要更複雜的狀態管理，可以作為未來優化。

### Q: 早上醒來怎麼看結果?
A:
```bash
gh issue list --label ai-task     # 查看任務狀態
gh pr list                         # 查看 PR
cat .ai/results/*.json            # 查看執行結果
```

---

## Appendix A: File Templates

### A.1 Issue Template

```markdown
# [type] Task Title

- Repo: backend | frontend | root
- Severity: P0 | P1 | P2
- Source: tasks.md #N | audit:F001

## Objective
What to achieve.

## Scope
What to change.

## Non-goals
What NOT to change.

## Constraints
- obey AGENTS.md
- obey .claude/rules/*.md

## Verification
- Commands to run

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2
```

### A.2 Stop File

```bash
# 創建停止標記
echo "Stopped by user at $(date)" > .ai/state/STOP
```

---

*Document Version: 1.1 | Last Updated: 2024-12-18*
