# Testing Guide

本文件說明 AI Workflow Kit 的測試架構與執行方式。

> **注意：** AWK 主要測試已遷移至 Go。原有的 Python 測試已棄用。

---

## 測試環境設定

### 安裝依賴

```bash
# Go 1.25+ 已包含測試框架
go version

# (選用) 安裝 gotestsum 以獲得更好的輸出
go install gotest.tools/gotestsum@latest
```

---

## 測試架構

### 目錄結構

```
# Go 測試 (主要)
cmd/awkit/
├── main_test.go              # 主程式測試
├── kickoff_test.go           # kickoff 命令測試
├── session_integration_test.go  # Session 整合測試
└── reviewer_integration_test.go # PR 審查整合測試

internal/
├── errors/errors_test.go     # 錯誤處理測試
├── audit/auditor_test.go     # 專案審計測試
├── evaluate/evaluate_test.go # 評估測試
├── generate/
│   ├── generator_test.go     # 生成器測試
│   └── repo_type_test.go     # Repo 類型測試
├── git/
│   ├── branch_test.go        # 分支操作測試
│   ├── operations_test.go    # Git 操作測試
│   └── worktree_test.go      # Worktree 測試
├── install/install_test.go   # 安裝測試
├── kickoff/
│   ├── config_test.go        # 配置測試
│   ├── fanin_test.go         # Fan-in 測試
│   ├── integration_test.go   # 整合測試
│   └── lock_test.go          # 鎖定測試
└── ...

# 後端測試
backend/health/health_test.go # 後端健康檢查測試
```

> Go 測試的 fixtures 多以 `fstest.MapFS` 或 `t.TempDir()` 內嵌於各測試檔,無獨立的 fixtures 目錄。

---

## 執行測試

### 基本執行

```bash
# 從專案根目錄執行
cd /path/to/ai-workflow-kit

# 執行所有測試
go test ./...

# 執行特定套件測試
go test ./internal/errors/... -v

# 執行特定測試函數
go test ./internal/errors -run TestConfigError -v

# 執行符合模式的測試
go test ./... -run "Test.*Config" -v
```

### 測試輸出選項

```bash
# Verbose 輸出
go test ./... -v

# 使用 gotestsum (更好的輸出格式)
gotestsum ./...

# 短輸出
go test ./... -short
```

### 測試覆蓋率

```bash
# 基本覆蓋率報告
go test ./... -cover

# 詳細覆蓋率 (顯示每個套件)
go test ./... -cover -coverprofile=coverage.out
go tool cover -func=coverage.out

# 產生 HTML 報告
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

---


## CI 整合

### GitHub Actions

實際 CI 定義於 `.github/workflows/ci.yml`,共四個 job:

| Job | Runner | 內容 |
|-----|--------|------|
| `awkit_cli` | ubuntu-latest | `go vet`、`go test -race -coverprofile`（**60% 覆蓋率門檻閘門**）、`awkit evaluate --offline` 與 `--offline --strict`、上傳覆蓋率 artifact |
| `awkit_cli_windows` | **windows-latest** | 完整 `go test ./...` 套件,守護原生 path / ConPTY / process 行為（先關 autocrlf、把 Git Bash 加入 PATH） |
| `backend` | ubuntu-latest | 在 `backend/` 跑 `go test -race ./...` |
| `frontend` | ubuntu-latest | `frontend/Packages/manifest.json` JSON 驗證 + 資料夾檢查 |

> Windows job 是刻意的跨平台守門:kit 出貨 `awkit.exe`、`install.ps1` 與 ConPTY-based 的 Principal runner,只跑 Linux 曾累積過一批 Windows-only 的失敗。race 偵測器跑在 Linux job(`-race` 需要 cgo)。

### 本地 CI 模擬

```bash
go vet ./...
go test -race ./...          # 需 cgo（CGO_ENABLED=1）
awkit evaluate --offline --strict
```

---

## 常見問題

### Q: `-race` 無法執行?

race 偵測器需要 cgo。設定 `CGO_ENABLED=1` 並安裝 C toolchain(Windows 上為 gcc/mingw)。CI 的 race 測試跑在 Linux job。

```bash
CGO_ENABLED=1 go test -race ./...
```

### Q: 部分測試需要 `sh`?

Evidence 驗證與 retry 相關測試會呼叫 `sh`。Windows 上需確保 Git Bash 的 `bin` 在 PATH(CI 的 windows job 會自動加入)。

```bash
go test ./internal/reviewer/ ./internal/ghutil/ -v
```

### Q: 如何跳過特定測試?

```go
func TestSlow(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping in -short mode")
    }
    // ...
}
```

在 `//go:build` 標籤或 `runtime.GOOS` 判斷中排除平台特定測試(如 `process_windows.go` 的 `//go:build windows`)。

---

## 更多資源

- [Go testing 套件](https://pkg.go.dev/testing)
- [貢獻指南](contributing.md) - 測試撰寫規範
- [API 參考](api-reference.md) - 被測試的模組說明
