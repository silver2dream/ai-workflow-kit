# AI Workflow Kit - AWK 評分標準 v6.0

## 專案核心目的

> **用 AI (Claude Code + Codex) 自動化「Spec -> 實作 -> PR -> 合併」的開發流程**

**目標用戶**：想用 AI 自動化開發流程的開發者/團隊

---

## 執行方式

評估透過 Go 測試框架執行（開發者工具）：

```bash
# 完整評估
go test ./internal/evaluate/... -v

# Offline Gate
go test ./internal/evaluate/... -run "Offline" -v

# Online Gate
go test ./internal/evaluate/... -run "Online" -v

# 完整測試套件 (O10)
go test ./...
```

### 版本同步規則

- evaluate.md 必須與 Go 測試保持一致
- 任何變更必須同時更新文檔和測試

---

## 前置條件

### 關於「Offline」的說明

「Offline」意指**評估執行時不需要網路連線**，但**依賴套件需預先安裝**。

**保證**：Offline Gate 不執行任何網路操作（包括 `git fetch`）。
- 有 submodule 的 repo：pinned sha 檢查移到 Online Gate
- 無 submodule 的 repo：完全離線

### Offline Gate 前置條件

| 依賴 | 必要性 | 缺少時行為 | 說明 |
|------|--------|------------|------|
| `go` | 必要 | 測試無法執行 | Go 1.21+ |
| `git` | 必要 | FAIL（O0 無法執行） | git check-ignore 驗證 |

### Online Gate 額外需求

| 依賴 | 必要性 | 缺少時行為 | 說明 |
|------|--------|------------|------|
| `gh` | 必要 | SKIP（Online Gate 跳過） | GitHub CLI |
| 網路連線 | 必要 | SKIP（Online Gate 跳過） | 連線 GitHub API |

---

## SKIP 白名單

SKIP 狀態**僅允許**以下情況，其他情況一律為 FAIL：

| 允許 SKIP 的情況 | 範例 |
|------------------|------|
| 可選依賴缺少 | `file` 指令不存在 -> O8/O9 SKIP |
| 明確不適用 | 無 `.github/workflows` 目錄 -> EXT1 SKIP |
| Online Gate 前置條件不滿足 | `gh` 未安裝或未登入 -> Online Gate SKIP |

**不允許 SKIP 的情況**（必須為 FAIL）：
- 必要配置讀取失敗（如 `integration_branch` 讀不到）
- 測試執行錯誤
- 目錄存在但內容為空（如 `.github/workflows/` 存在但無 workflow 檔案）

---

## 評估模式

| 模式 | 說明 | 前置條件 | 用途 |
|------|------|----------|------|
| **Offline** | 驗證 Kit 本身的品質 | go + git | CI、自檢、品質評估 |
| **Online** | 驗證完整流程能否運作 | Offline + gh + 網路 | 部署前驗證 |

**核心原則**：
- **PASS** = 檢查通過
- **FAIL** = Kit 有問題（扣分）
- **SKIP** = 符合白名單條件（不扣分）

---

## 評分上限與等級

### Score Cap

| 條件 | 最高分數 | 等級上限 |
|------|----------|----------|
| Offline Gate 有 FAIL | 4.0 | F |
| Offline Gate 全 PASS/SKIP，Online Gate 未執行或有 SKIP | 8.5 | B |
| Offline + Online Gate 全 PASS | 10.0 | A |

### 等級門檻

| 分數區間 | 等級 | 說明 |
|----------|------|------|
| 9.0 - 10.0 | A | 生產就緒（需 Online Gate PASS） |
| 8.0 - 8.9 | B | 功能完整 |
| 7.0 - 7.9 | C | 核心可用 |
| 6.0 - 6.9 | D | 有缺失 |
| < 6.0 | F | 不可用 |

### 等級計算

```
final_grade = grade(min(total_score, cap))
```

- `total_score`：依面向評分公式計算的原始總分
- `cap`：由 Gate 結果決定的評分上限
- `final_grade`：取兩者較小值對應的等級

**範例**：
- 面向總分 9.5，但 Online Gate SKIP -> cap=8.5 -> 最終等級 B
- 面向總分 7.0，Offline Gate PASS -> cap=8.5 -> 最終等級 C（7.0 < 8.5）

---

## Offline Gate（P0 一票否決）

**任一 FAIL -> 總分上限 4.0 (F)**
**SKIP 不影響評分**

這些檢查不需要網路、不需要 gh auth、不需要 claude/codex，只驗證 Kit 本身。

| 標準 | Go 測試函數 | 說明 |
|------|-------------|------|
| O0 | TestOfflineGate_O0_* | git-ignore 檢查 |
| O1-O2 | TestOfflineGate_ScanRepo_* | Repo 掃描 |
| O3-O4 | TestOfflineGate_AuditProject_* | 專案審計 |
| O5 | TestOfflineGate_ConfigValidation_* | 配置驗證 |
| O7 | TestOfflineGate_VersionSync_* | 版本同步 |
| O8-O9 | TestOfflineGate_FileEncoding_* | 檔案編碼 |
| O10 | TestOfflineGate_TestSuite_* | 測試套件 |

### O0: 無副作用檢查

評估過程不應弄髒工作樹。使用 `git check-ignore` 確保目錄真的會被 Git 忽略。

| ID | 驗證內容 | PASS 條件 |
|----|----------|-----------|
| O0.1 | `.ai/state/` | Git 會忽略 |
| O0.2 | `.ai/results/` | Git 會忽略 |
| O0.3 | `.ai/runs/` | Git 會忽略 |
| O0.4 | `.ai/exe-logs/` | Git 會忽略 |
| O0.5 | `.worktrees/` | Git 會忽略 |

### O1-O4: scan_repo 與 audit_project

| ID | 驗證內容 | PASS 條件 |
|----|----------|-----------|
| O1+O2 | 執行 scan_repo | 輸出有效 JSON |
| O3+O4 | 執行 audit_project | 輸出有效 JSON |
| O4.1 | 檢查 audit P0 findings | 無 P0 findings |

### O5: validate_config

| ID | 驗證內容 | PASS 條件 |
|----|----------|-----------|
| O5 | 驗證 workflow.yaml 配置 | 配置有效 |

### O7: 版本同步（P0 強制）

| ID | 驗證內容 | PASS 條件 |
|----|----------|-----------|
| O7 | 文檔與測試版本一致 | 版本號完全一致 |

### O8/O9: 編碼檢查

| ID | 驗證內容 | PASS 條件 | SKIP 條件 |
|----|----------|-----------|-----------|
| O8 | 腳本檔案編碼 | 無 CRLF/UTF-16 | 無 `file` 指令（可選依賴） |
| O9 | 文檔檔案編碼 | 無 UTF-16 | 無 `file` 指令（可選依賴） |

### O10: 測試套件

| ID | 驗證內容 | PASS 條件 |
|----|----------|-----------|
| O10 | 執行完整測試套件 | `go test ./...` 通過 |

---

## Extensibility Checks（不影響 Gate）

這些檢查為 P1 等級，不影響 Offline Gate 結果，但會影響面向評分。

### EXT1: CI/分支對齊

檢查 CI workflows 是否會被 integration_branch 觸發。

| 情況 | 結果 | 說明 |
|------|------|------|
| 無 `.github/workflows` 目錄 | SKIP | 明確不適用（允許） |
| 目錄存在但無 workflow 檔案 | FAIL | 應有 workflows 或移除目錄 |
| 讀不到 `integration_branch` | FAIL | 配置錯誤（不允許 SKIP） |
| Workflows 不觸發 integration_branch | FAIL | CI 配置問題 |
| Workflows 觸發 integration_branch | PASS | 正確配置 |

### EXT1 與面向評分的映射

| EXT1 結果 | 面向評分影響 |
|-----------|--------------|
| PASS | 無影響 |
| FAIL | 可擴展性面向 P1 未通過（扣 1 分） |
| SKIP | 無影響（不適用不扣分） |

---

## Online Gate（條件式）

**前置條件不滿足 -> SKIP（不扣分）**
**前置條件滿足但 FAIL -> 評分上限 8.5 (B)**

| 標準 | Go 測試函數 | 前置條件 |
|------|-------------|----------|
| N1 | TestOnlineGate_N1_* | gh CLI |
| N2 | TestOnlineGate_N2_* | gh CLI |
| N3 | TestOnlineGate_N3_* | gh CLI |

### 前置條件檢查

| ID | 驗證內容 | 說明 |
|----|----------|------|
| PRE.1 | gh CLI 已安裝且已登入 | `gh auth status` |
| PRE.2 | 可連線 GitHub | GitHub API 可達 |

### Online Gate 檢查項目

| ID | 驗證內容 | PASS 條件 |
|----|----------|-----------|
| N1 | kickoff dry-run | 成功執行 |
| N2 | rollback dry-run | 輸出包含預期訊息 |
| N3 | stats 輸出 | 輸出有效 JSON |

---

## 面向評分

### 權重與分級

| 面向 | 權重 | P0 項數 | P1 項數 | P2 項數 |
|------|------|---------|---------|---------|
| 核心流程 | 30% | 3 | 5 | 4 |
| 可靠性 | 25% | 2 | 7 | 3 |
| 可擴展性 | 20% | 2 | 4 | 3 |
| 易用性 | 15% | 1 | 6 | 9 |
| 安全性 | 10% | 2 | 3 | 2 |

### 分數計算

```
面向分數 = 10 - (P1未通過數 x 1) - (P2未通過數 x 0.5)
如果有 P0 未通過：面向分數 = min(面向分數, 4)
面向分數最低為 0

原始總分 = Sum(面向分數 x 權重)
最終總分 = min(原始總分, 評分上限)
最終等級 = grade(最終總分)
```

---

## 如何使用

```bash
# Offline 模式（完全離線）
go test ./internal/evaluate/... -run "Offline" -v

# Online 模式
go test ./internal/evaluate/... -run "Online" -v

# 完整評估
go test ./internal/evaluate/... -v

# 完整測試套件
go test ./...
```

---

## 版本紀錄

| 版本 | 日期 | 說明 |
|------|------|------|
| 1.0 | 2025-12-19 | 初始版本 |
| 2.0 | 2025-12-19 | 加入 Must-Pass Gate、P0/P1/P2 分級 |
| 3.0 | 2025-12-19 | 拆分 Offline/Online Gate、加入 SKIP 狀態、明確前置條件 |
| 3.1 | 2025-12-19 | 統一 Online Gate、type-specific 驗證 |
| 3.2 | 2025-12-19 | 修正前置條件矛盾、O1-O4 合併邏輯、O6/O7 SKIP 規則、N2 pipefail 問題 |
| 3.3 | 2025-12-19 | bash 改為必要、新增 O0 gitignore 檢查、O6 CI/分支對齊、擴大敏感資訊掃描 |
| 4.0 | 2025-12-19 | evaluate.sh 成為唯一權威、版本強制一致(O7)、O0 用 git check-ignore、O6 用 Python 解析 YAML |
| 4.1 | 2025-12-19 | 明確 Output Contract、統一前置條件表格、修正 O6 PyYAML on: 布林值問題 |
| 4.2 | 2025-12-19 | O6 移出 Offline Gate 改為 EXT1、SKIP 白名單、--strict 模式、補齊 curl 依賴、等級映射明確化、標註示意程式碼 |
| 4.3 | 2025-12-19 | EXT1 與面向評分映射、--strict 使用前提說明 |
| 5.0 | 2025-12-19 | Offline Gate 真正離線（移除 git fetch）、統一 audit/scan schema、dirty_worktree 改為 P1、新增 --check-origin 選項 |
| 5.1 | 2025-12-19 | 修正 --strict 文檔（dirty_worktree 是 P1）、--check-origin 網路錯誤改為 FAIL（非 SKIP） |
| 5.2 | 2025-12-21 | 新增文件完整性檢查：使用者文件(P1)、開發者文件(P2)、lib 模組(P1)、README 文件索引(P2) |
| 6.0 | 2026-01-14 | 遷移至 Go 測試框架：移除 Python/Shell 腳本、評估透過 go test 執行、標準映射至 Go 測試函數 |
