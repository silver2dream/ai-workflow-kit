審查指定的 PR，決定是否通過。

用法：`/review-pr <PR_NUMBER>` 或 `/review-pr`（會詢問 PR 編號）

---

## Step 1: 獲取 PR 信息

```bash
gh pr view <PR_NUMBER> --json number,title,body,headRefName,baseRefName,additions,deletions,changedFiles,statusCheckRollup
```

## Step 2: 檢查 CI 狀態（必須）

```bash
gh pr checks <PR_NUMBER>
```

**CI 狀態判斷：**
- ✅ 所有 checks 通過 → 繼續審查
- ⏳ checks 仍在執行 → 等待完成後再審查
- ❌ 任何 check 失敗 → 直接 reject，創建 fix issue

如果 CI 失敗，不需要審查代碼，直接：
```bash
gh pr review <PR_NUMBER> --request-changes --body "❌ CI 失敗

**失敗的 checks：**
$(gh pr checks <PR_NUMBER> --json name,state --jq '.[] | select(.state != \"SUCCESS\") | \"- \" + .name + \": \" + .state')

請修復 CI 錯誤後重新提交。
"
```

## Step 3: 獲取 PR Diff

```bash
gh pr diff <PR_NUMBER>
```

## Step 3.1: 檢查 Submodule 變更 (Req 21.1-21.4)

檢查 PR 是否包含 submodule 變更：

```bash
# 檢查是否有 submodule 變更
SUBMODULE_CHANGES=$(gh pr diff <PR_NUMBER> | grep -E "^diff --git.*Subproject commit" || true)

if [[ -n "$SUBMODULE_CHANGES" ]]; then
  echo "⚠️ PR 包含 submodule 變更"
  
  # 獲取變更的 submodule 路徑
  CHANGED_SUBMODULES=$(gh pr diff <PR_NUMBER> | grep -B1 "Subproject commit" | grep "^diff --git" | sed 's/.*a\///' | sed 's/ b\/.*//' | sort -u)
  
  for submodule in $CHANGED_SUBMODULES; do
    echo "  - Submodule: $submodule"
    
    # 獲取 submodule commit 變更
    OLD_SHA=$(gh pr diff <PR_NUMBER> | grep -A1 "diff --git a/$submodule" | grep "^-Subproject commit" | sed 's/-Subproject commit //' || echo "")
    NEW_SHA=$(gh pr diff <PR_NUMBER> | grep -A1 "diff --git a/$submodule" | grep "^+Subproject commit" | sed 's/+Subproject commit //' || echo "")
    
    if [[ -n "$OLD_SHA" && -n "$NEW_SHA" ]]; then
      echo "    Old SHA: $OLD_SHA"
      echo "    New SHA: $NEW_SHA"
      
      # 檢查 submodule commit 是否已 push 到 origin (Req 21.3)
      if ! git -C "$submodule" fetch origin "$NEW_SHA" --depth=1 2>/dev/null; then
        echo "    ⚠️ WARNING: Submodule commit $NEW_SHA not found on origin!"
        echo "    This may cause issues when others clone the repo."
      fi
      
      # 顯示 submodule 的 commit diff (Req 21.2)
      echo "    Submodule commits:"
      git -C "$submodule" log --oneline "$OLD_SHA..$NEW_SHA" 2>/dev/null || echo "    (unable to show commits)"
    fi
  done
fi
```

## Step 4: 確定適用的規則

從 PR body 或 branch name 判斷 Repo 類型，讀取對應規則：

```bash
# 通用規則
cat .ai/rules/_kit/git-workflow.md

# Backend (如果 PR 涉及 backend)
cat .ai/rules/backend-go.md

# Frontend (如果 PR 涉及 frontend)
cat .ai/rules/frontend-unity.md
```

## Step 5: 審查清單

檢查以下項目：

### 4.1 Git 規範
- [ ] Commit message 使用 `[type] subject` 格式（小寫）
- [ ] PR base 是配置的 integration branch（見 workflow.yaml）
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

## Step 6: 做出決定

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
