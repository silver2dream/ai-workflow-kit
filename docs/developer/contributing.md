# Contributing Guide

本文件說明如何為 AI Workflow Kit 貢獻程式碼。

---

## 開發環境設定

### 必要條件

- Go 1.25+
- Git
- GitHub CLI (`gh`)
- Bash (Windows 使用者需要 Git Bash 或 WSL)

### 安裝開發依賴

AWK 是純 Go 專案,不需要額外的執行期依賴:

```bash
go build ./...
go test ./...
```

### 專案結構

```
.
├── cmd/awkit/        # CLI 進入點
├── internal/         # 核心套件（analyzer, reviewer, worker, lessons, kickoff, ...）
├── .ai/
│   ├── config/       # 配置檔與 Schema
│   ├── templates/    # 範例模板（design.md.example）
│   ├── rules/        # 規則檔案（_kit / _examples）
│   ├── skills/       # Principal/post-mortem/release-checklist 技能
│   └── specs/        # Kiro-style specs
├── .claude/agents/   # 內建 subagents（pr-reviewer, conflict-resolver）
└── docs/             # 文件 (你正在讀的)
```

---

## 開發流程

### 1. Fork 與 Clone

```bash
# Fork 專案後 clone
git clone https://github.com/<your-username>/ai-workflow-kit.git
cd ai-workflow-kit

# 加入 upstream remote
git remote add upstream https://github.com/silver2dream/ai-workflow-kit.git
```

### 2. 建立分支

```bash
# 從 main 建立功能分支
git checkout main
git pull upstream main
git checkout -b feat/your-feature
```

### 分支命名規範

| 類型 | 格式 | 範例 |
|------|------|------|
| 功能 | `feat/description` | `feat/add-gitlab-support` |
| 修復 | `fix/description` | `fix/yaml-parsing-error` |
| 文件 | `docs/description` | `docs/update-api-reference` |
| 重構 | `refactor/description` | `refactor/error-handling` |

### 3. 開發與測試

```bash
# 執行所有 Go 測試
go test ./...

# 執行特定套件測試
go test ./internal/errors/... -v

# 執行測試覆蓋率
go test ./... -cover

# 驗證配置
awkit validate
```

### 4. 提交變更

```bash
# 遵循 commit 格式
git add .
git commit -m "[feat] add GitLab support"
```

### Commit 格式

```
[type] subject

body (optional)
```

**Type 類型：**

| Type | 說明 |
|------|------|
| `feat` | 新功能 |
| `fix` | 修復 bug |
| `docs` | 文件變更 |
| `refactor` | 重構 (不改變功能) |
| `test` | 測試相關 |
| `chore` | 維護性工作 |

**範例：**
```
[feat] add retry mechanism for failed executions

- Add retry_count and retry_delay_seconds to config
- Implement exponential backoff
- Record retry count in the issue result
```

### 5. 推送與建立 PR

```bash
# 推送分支
git push origin feat/your-feature

# 建立 PR
gh pr create --base main --title "[feat] add GitLab support" --body "..."
```

---

## 程式碼規範

AWK 是純 Go 專案,遵循標準 Go 慣例:

- 提交前跑 `gofmt`(或 `goimports`)與 `go vet ./...`
- 命名依 [Effective Go](https://go.dev/doc/effective_go):exported 用 `CamelCase`、unexported 用 `camelCase`,縮寫維持大小寫一致(`URL`、`ID`、`PR`)
- 錯誤處理:回傳 `error` 而非 panic;用 `fmt.Errorf("...: %w", err)` 包裝以保留 error chain
- 每個 exported 型別/函式都要有以名稱開頭的 doc comment
- 平台特定程式碼用 `//go:build` 標籤分檔(如 `*_windows.go` / `*_unix.go`)
- 對 agent/LLM 輸出的解析一律 **fail-closed**:格式不符即明確錯誤,不要用寬鬆預設值掩蓋(參見 `internal/reviewer` 的結構化 review 契約)

---

## 測試規範

### 測試檔案結構

```
# Go 測試 (主要測試架構)
cmd/awkit/
├── main_test.go
├── kickoff_test.go
└── *_test.go

internal/
├── errors/errors_test.go
├── audit/auditor_test.go
├── evaluate/evaluate_test.go
├── generate/generator_test.go
├── git/operations_test.go
├── lessons/lessons_test.go
├── reviewer/structured_test.go
└── ...
```

### 撰寫測試 (Go)

```go
package errors

import (
    "testing"
)

func TestAWKError(t *testing.T) {
    t.Run("creates error with message", func(t *testing.T) {
        err := NewError("test message")
        if err.Error() != "test message" {
            t.Errorf("expected 'test message', got %s", err.Error())
        }
    })

    t.Run("returns correct exit code", func(t *testing.T) {
        err := NewConfigError("config error")
        if err.ExitCode() != 2 {
            t.Errorf("expected exit code 2, got %d", err.ExitCode())
        }
    })
}
```

### 測試命令

```bash
# 執行所有測試
go test ./...

# 帶覆蓋率執行
go test ./... -cover

# 帶 verbose 輸出
go test ./... -v

# 執行特定測試函數
go test ./internal/errors -run TestAWKError -v
```

### 測試覆蓋率要求

- 新增的程式碼應有對應的測試
- 核心套件 (`internal/errors`, `internal/analyzer`, `internal/reviewer`, `internal/worker`) 覆蓋率應 > 70%
- PR 不應降低整體覆蓋率

---

## 文件規範

### 文件類型

| 類型 | 位置 | 對象 |
|------|------|------|
| 使用者文件 | `docs/user/` | Kit 使用者 |
| 開發者文件 | `docs/developer/` | Kit 貢獻者 |
| API 文件 | 程式碼內 docstring | 開發者 |

### Markdown 風格

- 使用 ATX 風格標題 (`#`)
- 程式碼區塊標註語言
- 使用表格呈現結構化資訊
- 中英文之間加空格

### API 文件格式

Go doc comment 以符號名稱開頭,說明用途、參數語義與回傳/錯誤條件:

```go
// SubmitReview submits a PR review and handles the outcome (merge,
// changes_requested, review_blocked, ...). A structured submission
// (opts.Structured) bypasses markdown parsing; ctx bounds all GitHub calls.
// Returns the verdict, or an error on an operational failure.
func SubmitReview(ctx context.Context, opts SubmitReviewOptions) (*SubmitReviewResult, error) {
    // ...
}
```

---

## Pull Request 流程

### PR Checklist

提交 PR 前請確認：

- [ ] 程式碼符合風格規範
- [ ] 所有測試通過 (`go test ./...`)
- [ ] 新功能有對應測試
- [ ] 文件已更新 (如適用)
- [ ] Commit 格式正確

### PR 描述模板

```markdown
## Summary

簡述變更內容。

## Changes

- 變更項目 1
- 變更項目 2

## Testing

說明如何測試這些變更。

## Related Issues

Closes #123
```

### Review 流程

1. **自動檢查** - CI 執行測試
2. **Code Review** - 維護者審查
3. **修改** - 根據回饋修改
4. **合併** - 審查通過後 squash merge

---

## 發布流程

### 版本號規則

遵循 Semantic Versioning：

- **MAJOR** - 不相容的 API 變更
- **MINOR** - 向後相容的新功能
- **PATCH** - 向後相容的 bug 修復

### 發布 Checklist

1. 更新 CHANGELOG.md
2. 更新版本號
3. 建立 Release PR
4. 合併後建立 Git tag
5. 發布 GitHub Release

---

## 常見問題

### Q: 測試失敗怎麼辦？

```bash
# 查看詳細輸出 (Go)
go test ./... -v

# 執行特定套件測試
go test ./internal/errors/... -v

# 跑單一測試
go test ./internal/reviewer/ -run TestParseStructuredReview -v
```

### Q: 如何在本地端到端試跑工作流?

```bash
# 使用 awkit CLI
awkit kickoff --dry-run

# 啟用 debug 輸出
awkit generate
```

### Q: Windows 上腳本執行失敗？

確保使用 Git Bash 或 WSL，並檢查行尾符號：

```bash
git config --global core.autocrlf input
```

---

## 聯繫方式

- **Issues**: [GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues)
- **Discussions**: [GitHub Discussions](https://github.com/silver2dream/ai-workflow-kit/discussions)

---

## 更多資源

- [架構說明](architecture.md) - 系統內部架構
- [API 參考](api-reference.md) - 函數與模組說明
- [測試說明](testing.md) - 測試架構與執行
