審查指定的 PR，決定是否通過。

用法：`/review-pr <PR_NUMBER>` 或 `/review-pr`（會詢問 PR 編號）

---

## Step 1: 獲取 PR 信息

```bash
gh pr view <PR_NUMBER> --json number,title,body,headRefName,baseRefName,additions,deletions,changedFiles
```

## Step 2: 獲取 PR Diff

```bash
gh pr diff <PR_NUMBER>
```

## Step 3: 確定適用的規則

從 PR body 或 branch name 判斷 Repo 類型，讀取對應規則：

```bash
# 通用規則
cat .claude/rules/git-workflow.md

# Backend (如果 PR 涉及 backend)
cat .claude/rules/backend-nakama-architecture-and-patterns.md

# Frontend (如果 PR 涉及 frontend)
cat .claude/rules/unity-architecture-and-patterns.md
```

## Step 4: 審查清單

檢查以下項目：

### 4.1 Git 規範
- [ ] Commit message 使用 `[type] subject` 格式（小寫）
- [ ] PR base 是 `feat/aether`（除非是 release）
- [ ] PR body 包含 `Closes #N`

### 4.2 範圍限制
- [ ] 變更在 ticket scope 內
- [ ] 沒有不相關的重構
- [ ] 沒有引入新的不必要依賴

### 4.3 架構合規（根據 Repo）

**Backend:**
- [ ] RPC 方法在 `_service.go`，不在 `_module.go`
- [ ] Repository 介面在 `_repository.go`
- [ ] 沒有 service 包 service
- [ ] 沒有 ctx.Value 直接轉型（無 ok-check）

**Frontend:**
- [ ] 沒有硬編碼字串（使用 Localization）
- [ ] UI 事件通過 EventBus 發布
- [ ] 沒有直接場景跳轉（使用 UIFlow）

### 4.4 代碼品質
- [ ] 沒有明顯的邏輯錯誤
- [ ] 沒有安全漏洞（SQL injection, XSS 等）
- [ ] 沒有未處理的錯誤

## Step 5: 做出決定

### 如果通過：

```bash
gh pr review <PR_NUMBER> --approve --body "✅ AI Review 通過

**檢查項目：**
- Git 規範：✓
- 範圍限制：✓
- 架構合規：✓
- 代碼品質：✓
"
```

詢問是否要立即 merge：
```bash
gh pr merge <PR_NUMBER> --squash --delete-branch
```

### 如果不通過：

```bash
gh pr review <PR_NUMBER> --request-changes --body "❌ 需要修正

**問題：**
1. <問題描述>
2. <問題描述>

**建議修正方式：**
- <建議>
"
```

---

## 輸出

報告審查結果：
- 審查的 PR 編號和標題
- 通過/不通過
- 具體原因
- 已執行的操作（approve/request-changes/merge）
