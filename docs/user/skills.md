# Skills 使用指南

本文件說明 AI Workflow Kit 的 Skills 功能。

---

## 什麼是 Skills？

Skills 是 AWK 的技能系統，用於定義 AI Agent 的可執行任務。每個 Skill 是一組結構化的指令，讓 Agent 能夠執行特定的工作流程。

Skills 可以透過 Claude Code 的 Slash Command 或直接對話觸發。

---

## 可用的 Skills

### `/create-issues`

自動建立 GitHub Issues。

**用途：**
- 根據專案分析建立 Issue
- 批量建立任務清單

**使用方式：**
```
/create-issues <描述你想建立什麼類型的 issues>
```

**範例：**
```
/create-issues 更新文檔
/create-issues 重構 API 模組
```

**流程：**
1. **Analyze** - 分析專案結構和現有文件
2. **Breakdown** - 詢問具體需求 (scope, priority)
3. **Propose** - 提出 Issue 清單
4. **Approval** - 等待使用者確認
5. **Create** - 建立 GitHub Issues

---

### `/run-issues`

自動執行多個 GitHub Issues。

**用途：**
- 批量處理已建立的 Issues
- 並行調度 Worker 執行任務

**使用方式：**
```
/run-issues <issue 編號列表>
```

**範例：**
```
/run-issues 98 99 100 101 102
```

**流程：**
1. **Pre-Flight** - 檢查是否有其他工作流程執行中
2. **Fetch** - 取得 Issue 詳情
3. **Analyze** - 分析依賴關係和優先級
4. **Parallelize** - 規劃並行執行批次
5. **Execute** - 調度 Worker 執行任務
6. **Verify** - 驗證執行結果
7. **Report** - 產生執行報告

---

### `/principal-workflow`

Principal Agent 的主工作流程。

**用途：**
- 執行完整的 AWK 自動化工作流程
- 通常由 `awkit kickoff` 自動觸發

**流程：**
1. 讀取 specs/tasks.md
2. 選擇下一個任務
3. 建立 GitHub Issue
4. 調度 Worker 執行
5. 審查 PR
6. 合併或退回
7. 迴圈

---

## Skills 結構

每個 Skill 由以下檔案組成：

```
.ai/skills/<skill-name>/
├── SKILL.md           # 技能入口和總覽
├── phases/            # 流程階段
│   ├── analyze.md     # 分析階段指令
│   ├── execute.md     # 執行階段指令
│   └── verify.md      # 驗證階段指令
├── references/        # 參考文件
└── tasks/             # 任務範本
```

---

## 建立自訂 Skill

### 1. 建立目錄結構

```bash
mkdir -p .ai/skills/my-skill/phases
```

### 2. 建立 SKILL.md

```markdown
# My Custom Skill

描述這個 Skill 的用途。

## When to Use
- 使用情境 1
- 使用情境 2

## Workflow
1. Phase 1
2. Phase 2
3. Phase 3

## Quick Reference
| Phase | Action | File |
|-------|--------|------|
| Analyze | 分析專案 | phases/analyze.md |
```

### 3. 建立階段檔案

在 `phases/` 目錄建立各階段的指令檔案。

---

## 與 awkit CLI 的整合

Skills 可以透過 `awkit kickoff` 自動觸發：

```bash
# 啟動工作流程 (使用 principal-workflow skill)
awkit kickoff

# 乾跑模式
awkit kickoff --dry-run
```

---

## 更多資源

- [快速開始](getting-started.md)
- [配置說明](configuration.md)
- [架構說明](../developer/architecture.md)
