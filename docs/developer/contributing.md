# 貢獻

怎麼建置、測試、提交與發版。AWK 的內部結構見[架構](architecture.md)。

---

## 環境

- Go 1.25 以上
- `git`、[`gh`](https://cli.github.com/)
- Windows：Git for Windows，而且 `sh`、`bash` 要在 `PATH` 上（從 Git Bash 執行測試最省事）。有些測試會真的執行 shell 指令

```bash
go build ./...
go vet ./...
go test ./...
go build -o awkit ./cmd/awkit   # 本機試用的 binary
```

AWK 沒有 Go 以外的執行期相依。

## 這個 repository 的幾件事

- **它自己就是一個 AWK 專案。** `.claude/` 是 kit 本體，會被嵌進 binary 再裝進使用者的專案（`kit_embed.go`）；改 skill 或 subagent 就是改產品。
- **`CLAUDE.md`、`AGENTS.md`、`.claude/rules/_kit/git-workflow.md` 是產生的。** 要改內容，改 `internal/generate/generator.go`，再執行 `go run ./cmd/awkit generate` 把這幾份更新成新的輸出。
- **設定檔的每個鍵都要有程式讀，也都要有說明。** 加一個鍵：在 `internal/config/config.go` 加欄位與預設值、在 `config_test.go` 的 `full` 範例加上它、在 [`docs/user/configuration.md`](../user/configuration.md) 對應的小節說明它。少了後兩步的任何一步，`internal/config` 的測試都會失敗；文件裡列了一個沒有欄位的鍵也會。
- **文件說的要是程式做的。** 改了行為，同一個 pull request 裡改 README 與 `docs/`。

## 分支與 commit

- pull request 開向 `develop`；`main` 只收發版。
- 分支名稱：`feat/<主題>`、`fix/<主題>`，或其他 commit type 開頭（`refactor/…`、`chore/…`）。
- commit 訊息與 pull request 標題：`[type] subject`，type 是 `feat`、`fix`、`docs`、`style`、`refactor`、`perf`、`test`、`chore` 之一，subject 小寫。不是 Conventional Commits 的 `feat: …`。
- pull request 以 squash 合併。`develop` 要求簽章的 commit，squash 合併時由 GitHub 簽。

```bash
git checkout develop && git pull
git checkout -b feat/your-topic
# …
git commit -m "[feat] add a health endpoint"
git push -u origin feat/your-topic
gh pr create --base develop --title "[feat] add a health endpoint"
```

## 測試

```bash
go test ./...                                   # 全部
go test ./internal/reviewer/ -run TestName -v   # 一個
go test -race ./...                             # 競態檢查（Linux、macOS；Windows 需要 cgo 與 gcc）
go test -coverprofile=cover.out ./... && go tool cover -func=cover.out | tail -1
```

- 測試檔與被測的程式放在同一個套件裡，一個行為一個測試，測試上方的註解寫它保護的是哪一條規則。
- 對外部工具的呼叫（`gh`、`claude`、`git`）用套件層級的函式變數隔開，測試換成假的；不要在測試裡真的打 GitHub。
- **有些測試只在特定環境才會跑。** `TestIntegration_PreflightToLockRelease` 需要已登入的 `gh`、`claude`，而且工作目錄是乾淨的，否則跳過。所以送出之前，在 commit 之後的乾淨工作目錄再跑一次 `go test ./...`。
- 改了規則，問自己：把這條規則改壞，會有測試失敗嗎？沒有就補一個。

## CI

`.github/workflows/ci.yml` 是手寫的（`awkit generate` 不會動它），三個 job：

| Job | 做什麼 |
|---|---|
| `awkit_cli`（ubuntu） | `go vet`、`go test -race` 並要求整體覆蓋率 60% 以上、`awkit evaluate --offline --strict` |
| `awkit_cli_windows`（windows） | 在 Windows 上跑完整的 `go test ./...`，守住路徑、ConPTY 與行程相關的行為 |
| `skeleton`（ubuntu） | 用 `godot-go` preset 建一個專案，以它產生的 CI workflow 的指令建置並測試 |

`awkit evaluate --offline` 是給這個 CI 用的檢查：`.gitignore` 是否忽略了執行期的檔案、專案稽核有沒有 P0 的發現、設定檔是否有效、`internal/buildinfo/version.go` 在不在、文字檔的編碼與行尾是否一致。`--strict` 讓任何一項失敗都以非零結束。

## 發版

版本號只來自 git tag（建置時由 ldflags 寫進 `internal/buildinfo`）；不必手動改版本號，也沒有 CHANGELOG 要維護（release 的說明由 GitHub 從合併的 pull request 產生）。

1. 在 `develop` 上確認可以發版：`release-checklist` skill 會逐項檢查並給出 GO 或 NO-GO。
2. `develop` 若落後 `main`（例如 dependabot 直接合進 `main` 的更新），先把 `main` 合回 `develop`。
3. 開 pull request：`develop` → `main`，標題 `[release] merge develop into main`，以 merge commit 合併（不是 squash）。
4. 在 `main` 的合併 commit 上打 tag 並推上去：

   ```bash
   git fetch origin main
   git tag vX.Y.Z origin/main
   git push origin vX.Y.Z
   ```

   推 tag 會觸發 `.github/workflows/release.yml`：建置五個平台的 binary 並發佈 GitHub release。**推出去就是公開發佈，收不回來。**

## 回報問題

[GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues)。安全性問題照 [SECURITY.md](../../SECURITY.md) 的方式回報，不要開公開的 issue。
