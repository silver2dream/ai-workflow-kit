# Review PR

審查 PR 並決定 approve 或 request-changes。

## 輸入

- `PR_NUMBER`: PR 編號（必填）
- `ISSUE_NUMBER`: 關聯 Issue（可選）

## 步驟

### 1. 獲取 PR 信息

```bash
gh pr view "$PR_NUMBER" --json title,body,additions,deletions,files,baseRefName
gh pr diff "$PR_NUMBER"
```

### 2. 執行 5 項審查標準

1. **Commit 格式**：符合 `[type] subject`
2. **範圍限制**：變更在 ticket scope 內
3. **架構合規**：符合 `.ai/rules/_kit/git-workflow.md`
4. **代碼質量**：無調試代碼、無明顯 bug
5. **安全檢查**：無敏感資訊洩露

### 3. 生成 AWK Review Comment

```markdown
<!-- AWK Review -->

## Review Summary

**Diff Hash**: <sha256 前 16 字元>
**Review Cycle**: <N>

### 評分: <N>/10

### 評分理由:
<詳細說明>

### 可改進之處:
<建議>

### 潛在風險:
<風險>
```

### 4. 驗證 Review

```bash
bash .ai/scripts/verify_review.sh ".ai/temp/review-$PR_NUMBER.md"
```

### 5. 發布審查

- 分數 >= 7：`gh pr review "$PR_NUMBER" --approve`
- 分數 < 7：`gh pr review "$PR_NUMBER" --request-changes`

### 6. 自動合併（如果 approve）

```bash
gh pr merge "$PR_NUMBER" --squash --delete-branch --auto
```

## 輸出

- PR 已審查
- 如果 approve 且 CI 通過，PR 已合併
- 回到 main-loop
