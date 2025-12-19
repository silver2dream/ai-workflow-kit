# AI Workflow Kit - 評分標準 v5.1

## 專案核心目的

> **用 AI (Claude Code + Codex) 自動化「Spec → 實作 → PR → 合併」的開發流程**

**目標用戶**：想用 AI 自動化開發流程的開發者/團隊

---

## 權威來源

> **`.ai/scripts/evaluate.sh` 是唯一權威執行器。**
> **Gate 結果與 score cap 以 evaluate.sh 輸出為準；面向分數/等級需人工依本文檔的細項清單評估。**

### Output Contract

evaluate.sh **承諾輸出**：
- Offline Gate: PASS/FAIL + SKIP count
- Origin Checks: PASS/FAIL/SKIP（僅 `--check-origin` 時輸出）
- Extensibility Checks: PASS/FAIL/SKIP（不影響 Gate）
- Online Gate: PASS/FAIL/SKIP
- Score cap (4.0/8.5/10.0)

evaluate.sh **不承諾輸出**：
- 面向細項分數
- 總分與等級

### 版本同步規則

- evaluate.sh 和 evaluate.md 必須版本一致
- 版本不一致時，Offline Gate 直接 FAIL (O7)
- 任何變更必須同時更新兩個檔案

---

## 前置條件

### 關於「Offline」的說明

「Offline」意指**評估執行時不需要網路連線**，但**依賴套件需預先安裝**。

**保證**：Offline Gate 不執行任何網路操作（包括 `git fetch`）。
- 有 submodule 的 repo：pinned sha 檢查移到 `--check-origin` 選項
- 無 submodule 的 repo：完全離線

評估環境需具備以下依賴，可透過以下指令安裝：

```bash
pip3 install pyyaml jsonschema
```

### Offline Gate 前置條件

| 依賴 | 必要性 | 缺少時行為 | 說明 |
|------|--------|------------|------|
| `bash` | 必要 | 腳本無法執行，exit 127 | Windows 用 Git Bash / WSL |
| `python3` | 必要 | FAIL（無法執行驗證） | 執行驗證腳本 |
| `pyyaml` | 必要 | FAIL（O5 失敗） | 解析 workflow.yaml |
| `jsonschema` | 必要 | FAIL（O5 失敗） | 驗證配置 schema |
| `git` | 必要 | FAIL（O0 無法執行） | git check-ignore 驗證 |
| `file` | 可選 | SKIP（O8/O9 跳過） | 編碼檢查 |

### Online Gate 額外需求

| 依賴 | 必要性 | 缺少時行為 | 說明 |
|------|--------|------------|------|
| `gh` | 必要 | SKIP（Online Gate 跳過） | GitHub CLI |
| `curl` | 必要 | SKIP（Online Gate 跳過） | API 連線測試 |
| 網路連線 | 必要 | SKIP（Online Gate 跳過） | 連線 GitHub API |

---

## SKIP 白名單

SKIP 狀態**僅允許**以下情況，其他情況一律為 FAIL：

| 允許 SKIP 的情況 | 範例 |
|------------------|------|
| 可選依賴缺少 | `file` 指令不存在 → O8/O9 SKIP |
| 明確不適用 | 無 `.github/workflows` 目錄 → EXT1 SKIP |
| Online Gate 前置條件不滿足 | `gh` 未安裝或未登入 → Online Gate SKIP |

**不允許 SKIP 的情況**（必須為 FAIL）：
- 必要配置讀取失敗（如 `integration_branch` 讀不到）
- Python 執行錯誤
- 目錄存在但內容為空（如 `.github/workflows/` 存在但無 workflow 檔案）

---

## 評估模式

| 模式 | 說明 | 前置條件 | 用途 |
|------|------|----------|------|
| **Offline** | 驗證 Kit 本身的品質 | bash + git + Python 3 + pyyaml + jsonschema | CI、自檢、品質評估 |
| **Online** | 驗證完整流程能否運作 | Offline + gh + curl + 網路 | 部署前驗證 |
| **--strict** | 嚴格模式，檢查 audit P0 | 同 Offline + 乾淨工作樹 | 發布前驗證、CI |
| **--check-origin** | 檢查 submodule sha 是否存在於 origin | 同 Offline + 網路 | 有 submodule 時驗證 |

### --check-origin 行為說明

| 情況 | 結果 | 說明 |
|------|------|------|
| 無 submodules | SKIP | 不適用（允許） |
| 網路不可用 | FAIL | 使用者明確要求檢查，無法完成 |
| SHA 不存在於 origin | FAIL | submodule 配置問題 |
| 所有 SHA 都存在 | PASS | 正確配置 |

**設計理由**：`--check-origin` 是使用者明確請求的檢查，若因網路問題無法執行，應明確告知（FAIL）而非靜默跳過（SKIP）。

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
- 面向總分 9.5，但 Online Gate SKIP → cap=8.5 → 最終等級 B
- 面向總分 7.0，Offline Gate PASS → cap=8.5 → 最終等級 C（7.0 < 8.5）

---

## Offline Gate（P0 一票否決）

**任一 FAIL → 總分上限 4.0 (F)**
**SKIP 不影響評分**

這些檢查不需要網路、不需要 gh auth、不需要 claude/codex，只驗證 Kit 本身。

### O0: 無副作用檢查

評估過程不應弄髒工作樹。使用 `git check-ignore` 確保目錄真的會被 Git 忽略。

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| O0.1 | `git check-ignore -q .ai/state/` | Git 會忽略 .ai/state/ |
| O0.2 | `git check-ignore -q .ai/results/` | Git 會忽略 .ai/results/ |
| O0.3 | `git check-ignore -q .ai/runs/` | Git 會忽略 .ai/runs/ |
| O0.4 | `git check-ignore -q .ai/exe-logs/` | Git 會忽略 .ai/exe-logs/ |
| O0.5 | `git check-ignore -q .worktrees/` | Git 會忽略 .worktrees/ |

### O1-O4: scan_repo 與 audit_project

| ID | 驗證方式 | PASS 條件 |
|----|----------|-----------|
| O1+O2 | 執行 scan_repo.sh 或 scan_repo.py | 輸出有效 JSON |
| O3+O4 | 執行 audit_project.sh 或 audit_project.py | 輸出有效 JSON |
| O4.1 | `--strict` 模式：檢查 audit.json P0 findings | 無 P0 findings |

**關於 --strict 模式**：

`--strict` 設計用於 **CI 環境或乾淨 checkout**，不建議在本機開發時使用。原因：
- `audit_project` 會檢查是否有 P0 findings（如缺少關鍵檔案）
- 注意：`dirty_worktree` 在 v5.0 已改為 P1，不會觸發 --strict 失敗
- 在 CI 中，乾淨 checkout 後跑 `--strict` 可確保專案配置完整

**建議用法**：
- 本機開發：`bash .ai/scripts/evaluate.sh`（預設模式）
- CI/發布前：`bash .ai/scripts/evaluate.sh --strict`

### O5: validate_config

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| O5 | `python3 .ai/scripts/validate_config.py` | exit 0 |

### O7: 版本同步（P0 強制）

| ID | 驗證邏輯 | PASS 條件 |
|----|----------|-----------|
| O7 | 比對 evaluate.md 與 evaluate.sh 版本號 | 版本號完全一致 |

### O8/O9: 編碼檢查

| ID | 驗證指令 | PASS 條件 | SKIP 條件 |
|----|----------|-----------|-----------|
| O8 | `file .ai/scripts/*.sh` | 無 CRLF/UTF-16 | 無 `file` 指令（可選依賴） |
| O9 | `file README.md CLAUDE.md AGENTS.md` | 無 UTF-16 | 無 `file` 指令（可選依賴） |

### O10: 測試套件

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| O10 | `bash .ai/tests/run_all_tests.sh` | exit 0 |

---

## Extensibility Checks（不影響 Gate）

這些檢查為 P1 等級，不影響 Offline Gate 結果，但會影響面向評分。

### EXT1: CI/分支對齊（原 O6）

檢查 CI workflows 是否會被 integration_branch 觸發。

| 情況 | 結果 | 說明 |
|------|------|------|
| 無 `.github/workflows` 目錄 | SKIP | 明確不適用（允許） |
| 目錄存在但無 workflow 檔案 | FAIL | 應有 workflows 或移除目錄 |
| 讀不到 `integration_branch` | FAIL | 配置錯誤（不允許 SKIP） |
| Python 執行錯誤 | FAIL | 環境問題（不允許 SKIP） |
| Workflows 不觸發 integration_branch | FAIL | CI 配置問題 |
| Workflows 觸發 integration_branch | PASS | 正確配置 |

### EXT1 與面向評分的映射

| EXT1 結果 | 面向評分影響 |
|-----------|--------------|
| PASS | 無影響 |
| FAIL | 可擴展性面向 P1 未通過（扣 1 分） |
| SKIP | 無影響（不適用不扣分） |

**為何移出 Offline Gate**：
- 並非所有專案都使用 GitHub Actions
- 允許 SKIP 與 P0 Gate 的「一票否決」語意矛盾
- 作為 Extensibility P1 更合理

---

## Online Gate（條件式）

**前置條件不滿足 → SKIP（不扣分）**
**前置條件滿足但 FAIL → 評分上限 8.5 (B)**

### 前置條件檢查

| ID | 驗證指令 | 說明 |
|----|----------|------|
| PRE.1 | `command -v gh && gh auth status` | gh CLI 已安裝且已登入 |
| PRE.2 | `command -v curl` | curl 已安裝 |
| PRE.3 | `curl -s --max-time 5 https://api.github.com` | 可連線 GitHub |

### Online Gate 檢查項目

| ID | 驗證方式 | PASS 條件 |
|----|----------|-----------|
| N1 | `bash .ai/scripts/kickoff.sh --dry-run` | exit 0 |
| N2 | rollback dry-run 輸出包含預期訊息 | 見下方說明 |
| N3 | `bash .ai/scripts/stats.sh --json` | 輸出有效 JSON |

**N2 特殊處理**：捕獲輸出檢查，不依賴 exit code，避免 pipefail 誤判。

---

## 面向評分

### 權重與分級

| 面向 | 權重 | P0 項數 | P1 項數 | P2 項數 |
|------|------|---------|---------|---------|
| 核心流程 | 30% | 3 | 5 | 4 |
| 可靠性 | 25% | 2 | 4 | 3 |
| 可擴展性 | 20% | 2 | 4 | 3 |
| 易用性 | 15% | 1 | 3 | 3 |
| 安全性 | 10% | 2 | 3 | 2 |

### 分數計算

```
面向分數 = 10 - (P1未通過數 × 1) - (P2未通過數 × 0.5)
如果有 P0 未通過：面向分數 = min(面向分數, 4)
面向分數最低為 0

原始總分 = Σ(面向分數 × 權重)
最終總分 = min(原始總分, 評分上限)
最終等級 = grade(最終總分)
```

---

## Checkpoint 清單

> **注意**：以下驗證指令為**示意**，實際執行請使用 `.ai/scripts/evaluate.sh`。

### 1. 核心流程 (30%)

#### P0 - 必須通過

| ID | 驗證指令（示意） | PASS 條件 |
|----|------------------|-----------|
| C1.P0.1 | `test -f CLAUDE.md && test -f AGENTS.md` | 存在 |
| C1.P0.2 | `test -f .ai/commands/start-work.md` | 存在 |
| C1.P0.3 | `bash .ai/scripts/run_issue_codex.sh 2>&1 \| grep -qi usage` | 有 usage |

#### P1 - 重要功能

| ID | 驗證指令（示意） | PASS 條件 |
|----|------------------|-----------|
| C1.P1.1 | `python3 .ai/scripts/parse_tasks.py ... --json` | 有效 JSON |
| C1.P1.2 | `grep -qE "Phase A\|Phase B..." .ai/commands/start-work.md` | 有匹配 |
| C1.P1.3 | `grep -q "Ticket Format" CLAUDE.md` | 有匹配 |
| C1.P1.4 | `test -d .ai/results && test -d .ai/runs && test -d .ai/state` | 存在 |
| C1.P1.5 | `grep -q "STOP" .ai/scripts/kickoff.sh` | 有匹配 |

### 2. 可靠性 (25%)

#### P0 - 必須通過

| ID | 驗證指令（示意） | PASS 條件 |
|----|------------------|-----------|
| C2.P0.1 | `python3 -m json.tool .ai/config/failure_patterns.json` | 有效 JSON |
| C2.P0.2 | `test -f .ai/scripts/attempt_guard.sh` | 存在 |

### 3. 可擴展性 (20%)

#### P0 - 必須通過

| ID | 驗證指令（示意） | PASS 條件 |
|----|------------------|-----------|
| C3.P0.1 | `python3 -m json.tool .ai/config/workflow.schema.json` | 有效 JSON |
| C3.P0.2 | `python3 -c "import yaml; yaml.safe_load(...)"` | 有效 YAML |

### 4. 易用性 (15%)

#### P0 - 必須通過

| ID | 驗證指令（示意） | PASS 條件 |
|----|------------------|-----------|
| C4.P0.1 | `file README.md \| grep -qE 'UTF-16'` | 無匹配 |

### 5. 安全性 (10%)

#### P0 - 必須通過

| ID | 驗證指令（示意） | PASS 條件 |
|----|------------------|-----------|
| C5.P0.1 | `grep -q "escalation" .ai/config/workflow.yaml` | 有匹配 |
| C5.P0.2 | `grep -qE "\-\-dry-run" .ai/scripts/rollback.sh ...` | 有匹配 |

---

## 如何使用

```bash
# Offline 模式（預設，完全離線）
bash .ai/scripts/evaluate.sh

# Online 模式
bash .ai/scripts/evaluate.sh --online

# 嚴格模式（檢查 audit P0，建議在 CI 使用）
bash .ai/scripts/evaluate.sh --strict

# 檢查 submodule origin（需要網路）
bash .ai/scripts/evaluate.sh --check-origin

# 組合使用
bash .ai/scripts/evaluate.sh --online --strict --check-origin
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
