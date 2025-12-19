# AI Workflow Kit - 評分標準 v3.1

## 專案核心目的

> **用 AI (Claude Code + Codex) 自動化「Spec → 實作 → PR → 合併」的開發流程**

**目標用戶**：想用 AI 自動化開發流程的開發者/團隊

---

## 前置條件

### Python 依賴

Offline Gate 需要以下 Python 套件：

```bash
pip3 install pyyaml jsonschema jinja2
```

如果缺少依賴，相關檢查會顯示錯誤訊息並提示安裝指令。

### Online Gate 額外需求

- `gh` CLI 已安裝且已登入 (`gh auth login`)
- 可連線 GitHub API

---

## 評估模式

本標準區分兩種評估模式，避免把「環境問題」當成「Kit 問題」：

| 模式 | 說明 | 前置條件 | 用途 |
|------|------|----------|------|
| **Offline** | 驗證 Kit 本身的品質 | 無（clone 下來就能跑） | CI、自檢、品質評估 |
| **Online** | 驗證完整流程能否運作 | gh auth + 網路 | 部署前驗證 |

**核心原則**：
- **PASS** = 檢查通過
- **FAIL** = Kit 有問題（扣分）
- **SKIP** = 前置條件不滿足，無法判斷（不扣分）

---

## 評分上限

| 條件 | 最高分數 | 等級 |
|------|----------|------|
| Offline Gate 有 FAIL | 4.0 | F |
| Offline Gate 全 PASS，Online Gate 未執行或有 SKIP | 8.5 | B |
| Offline + Online Gate 全 PASS | 10.0 | A |

---

## Offline Gate（P0 一票否決）

**任一 FAIL → 總分上限 4.0 (F)**

這些檢查不需要網路、不需要 gh auth、不需要 claude/codex，只驗證 Kit 本身。

| ID | 驗證指令 | PASS 條件 | 驗證目的 |
|----|----------|-----------|----------|
| O1 | `bash .ai/scripts/scan_repo.sh 2>/dev/null \|\| python3 .ai/scripts/scan_repo.py` | exit 0 | 掃描腳本可執行 |
| O2 | `cat .ai/state/repo_scan.json \| python3 -m json.tool > /dev/null` | exit 0 | 輸出為有效 JSON |
| O3 | `bash .ai/scripts/audit_project.sh 2>/dev/null \|\| python3 .ai/scripts/audit_project.py` | exit 0 | 審計腳本可執行 |
| O4 | `cat .ai/state/audit.json \| python3 -m json.tool > /dev/null` | exit 0 | 輸出為有效 JSON |
| O5 | `python3 .ai/scripts/validate_config.py` | exit 0 | 配置驗證通過 |
| O6 | `! file .ai/scripts/*.sh \| grep -qE 'CRLF\|UTF-16'` | exit 0 | 無 CRLF 或 UTF-16 |
| O7 | `! file README.md CLAUDE.md AGENTS.md \| grep -qE 'UTF-16'` | exit 0 | 主要文件非 UTF-16 |
| O8 | `bash .ai/tests/run_all_tests.sh` | exit 0 | 測試套件通過 |

```bash
# Offline Gate 檢查腳本
echo "=== Offline Gate ==="
OFFLINE_PASS=true

check_offline() {
  local id="$1" cmd="$2" desc="$3"
  if eval "$cmd" > /dev/null 2>&1; then
    echo "✓ $id: $desc"
  else
    echo "✗ $id: $desc"
    OFFLINE_PASS=false
  fi
}

check_offline "O1" "bash .ai/scripts/scan_repo.sh 2>/dev/null || python3 .ai/scripts/scan_repo.py" "scan_repo 可執行"
check_offline "O2" "python3 -m json.tool .ai/state/repo_scan.json" "repo_scan.json 有效"
check_offline "O3" "bash .ai/scripts/audit_project.sh 2>/dev/null || python3 .ai/scripts/audit_project.py" "audit_project 可執行"
check_offline "O4" "python3 -m json.tool .ai/state/audit.json" "audit.json 有效"
check_offline "O5" "python3 .ai/scripts/validate_config.py" "config 驗證通過"
check_offline "O6" "! file .ai/scripts/*.sh | grep -qE 'CRLF|UTF-16'" "腳本無 CRLF/UTF-16"
check_offline "O7" "! file README.md CLAUDE.md AGENTS.md 2>/dev/null | grep -qE 'UTF-16'" "文件非 UTF-16"
check_offline "O8" "bash .ai/tests/run_all_tests.sh" "測試通過"

if [ "$OFFLINE_PASS" = false ]; then
  echo ""; echo "❌ Offline Gate FAILED → 評分上限 4.0 (F)"
  exit 1
fi
echo ""; echo "✅ Offline Gate PASSED"
```

---

## Online Gate（條件式）

**前置條件不滿足 → SKIP（不扣分）**
**前置條件滿足但 FAIL → 評分上限 8.5 (B)**

### 前置條件檢查

| ID | 驗證指令 | 說明 |
|----|----------|------|
| PRE.1 | `command -v gh && gh auth status` | gh CLI 已安裝且已登入 |
| PRE.2 | `curl -s --max-time 5 https://api.github.com > /dev/null` | 可連線 GitHub |

```bash
# 前置條件檢查
check_prereq() {
  if command -v gh > /dev/null 2>&1 && gh auth status > /dev/null 2>&1; then
    if curl -s --max-time 5 https://api.github.com > /dev/null 2>&1; then
      return 0  # 前置條件滿足
    fi
  fi
  return 1  # 前置條件不滿足
}
```

### Online Gate 檢查項目

只有在前置條件滿足時才執行：

| ID | 驗證指令 | PASS 條件 | 驗證目的 |
|----|----------|-----------|----------|
| N1 | `bash .ai/scripts/kickoff.sh --dry-run` | exit 0 | kickoff 流程可啟動 |
| N2 | `bash .ai/scripts/rollback.sh 99999 --dry-run 2>&1 \| grep -i "not found\|usage\|dry"` | 有輸出 | rollback 可執行 |
| N3 | `bash .ai/scripts/stats.sh --json \| python3 -m json.tool` | exit 0 | stats 可查詢 GitHub |

```bash
# Online Gate 檢查腳本
echo "=== Online Gate ==="

if ! check_prereq; then
  echo "○ SKIP: 前置條件不滿足 (gh auth 或網路)"
  echo "  → 評分上限 8.5 (B)"
  ONLINE_STATUS="SKIP"
else
  ONLINE_PASS=true

  check_online() {
    local id="$1" cmd="$2" desc="$3"
    if eval "$cmd" > /dev/null 2>&1; then
      echo "✓ $id: $desc"
    else
      echo "✗ $id: $desc"
      ONLINE_PASS=false
    fi
  }

  check_online "N1" "bash .ai/scripts/kickoff.sh --dry-run" "kickoff 可啟動"
  check_online "N2" "bash .ai/scripts/rollback.sh 99999 --dry-run 2>&1 | grep -qiE 'not found|usage|dry'" "rollback 可執行"
  check_online "N3" "bash .ai/scripts/stats.sh --json | python3 -m json.tool" "stats 可查詢"

  if [ "$ONLINE_PASS" = true ]; then
    echo ""; echo "✅ Online Gate PASSED → 可達 10.0 (A)"
    ONLINE_STATUS="PASS"
  else
    echo ""; echo "⚠️ Online Gate FAILED → 評分上限 8.5 (B)"
    ONLINE_STATUS="FAIL"
  fi
fi
```

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
  - Offline FAIL → 上限 4.0
  - Online SKIP/FAIL → 上限 8.5
  - 全 PASS → 上限 10.0
```

---

## Checkpoint 清單

### 1. 核心流程 (30%)

#### P0 - 必須通過

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C1.P0.1 | `test -f CLAUDE.md && test -f AGENTS.md` | exit 0 |
| C1.P0.2 | `test -f .ai/commands/start-work.md` | exit 0 |
| C1.P0.3 | `bash .ai/scripts/run_issue_codex.sh 2>&1 \| grep -qi usage` | 有 usage |

#### P1 - 重要功能

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C1.P1.1 | `python3 .ai/scripts/parse_tasks.py .ai/specs/*/tasks.md --json 2>/dev/null \| python3 -m json.tool` | 有效 JSON |
| C1.P1.2 | `grep -qE "Phase A\|Phase B\|Phase C\|Phase D" .ai/commands/start-work.md` | 有匹配 |
| C1.P1.3 | `grep -q "Ticket Format" CLAUDE.md` | 有匹配 |
| C1.P1.4 | `test -d .ai/results && test -d .ai/runs && test -d .ai/state` | exit 0 |
| C1.P1.5 | `grep -q "STOP" .ai/scripts/kickoff.sh` | 有匹配 |

#### P2 - 加分項

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C1.P2.1 | `grep -q "_depends_on" .ai/scripts/parse_tasks.py` | 支援依賴 |
| C1.P2.2 | `grep -qE "Coordination\|sequential\|parallel" .ai/commands/start-work.md` | 支援 multi-repo |
| C1.P2.3 | `test -f .ai/templates/design.md.example` | 有範例 |
| C1.P2.4 | `bash .ai/scripts/stats.sh --help 2>&1 \| grep -qE "html\|json"` | 多輸出格式 |

---

### 2. 可靠性 (25%)

#### P0 - 必須通過

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C2.P0.1 | `python3 -m json.tool .ai/config/failure_patterns.json > /dev/null` | 有效 JSON |
| C2.P0.2 | `test -f .ai/scripts/attempt_guard.sh` | 存在 |

#### P1 - 重要功能

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C2.P1.1 | `echo "cannot find package" \| bash .ai/scripts/analyze_failure.sh - 2>/dev/null \| grep -qi "matched\|type"` | 有分析結果 |
| C2.P1.2 | `test -f .ai/scripts/rollback.sh` | 存在 |
| C2.P1.3 | `test -f .ai/scripts/cleanup.sh` | 存在 |
| C2.P1.4 | `grep -q "retryable" .ai/config/failure_patterns.json` | 有重試機制 |

#### P2 - 加分項

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C2.P2.1 | `grep -q "failure_history" .ai/scripts/*.sh 2>/dev/null` | 有歷史記錄 |
| C2.P2.2 | `grep -q "stats_history" .ai/scripts/stats.sh` | 有趨勢追蹤 |
| C2.P2.3 | `grep -qE "\-\-days" .ai/scripts/cleanup.sh` | 支援 days 參數 |

---

### 3. 可擴展性 (20%)

#### P0 - 必須通過

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C3.P0.1 | `python3 -m json.tool .ai/config/workflow.schema.json > /dev/null` | 有效 JSON |
| C3.P0.2 | `python3 -c "import yaml; yaml.safe_load(open('.ai/config/workflow.yaml'))"` | 有效 YAML |

#### P1 - 重要功能

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C3.P1.1 | `test -f .ai/templates/CLAUDE.md.j2 && test -f .ai/templates/AGENTS.md.j2` | 存在 |
| C3.P1.2 | `ls .ai/templates/ci-*.yml.j2 2>/dev/null \| wc -l \| xargs test 5 -le` | ≥ 5 個 |
| C3.P1.3 | `test -f .ai/scripts/generate.sh` | 存在 |
| C3.P1.4 | `grep -qE "submodule\|directory\|root" .ai/config/workflow.schema.json` | 支援三種類型 |

#### P2 - 加分項

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C3.P2.1 | `test -f .ai/scripts/install.sh` | 有安裝腳本 |
| C3.P2.2 | `test -f .ai/scripts/init.sh` | 有初始化腳本 |
| C3.P2.3 | `ls .ai/templates/ci-*.yml.j2 2>/dev/null \| wc -l \| xargs test 8 -le` | ≥ 8 個 |

#### 條件化檢查

| 條件 | 驗證指令 | PASS 條件 |
|------|----------|-----------|
| 有 .gitmodules | `! test -f .gitmodules \|\| test -f .ai/templates/validate-submodules.yml.j2` | 存在或不適用 |
| config 有 go | `! grep -q "language: go" .ai/config/workflow.yaml \|\| test -f .ai/templates/ci-go.yml.j2` | 存在或不適用 |
| config 有 unity | `! grep -q "language: unity" .ai/config/workflow.yaml \|\| test -f .ai/templates/ci-unity.yml.j2` | 存在或不適用 |

---

### 4. 易用性 (15%)

#### P0 - 必須通過

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C4.P0.1 | `! file README.md \| grep -qE 'UTF-16'` | 非 UTF-16 |

#### P1 - 重要功能

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C4.P1.1 | `grep -qiE "quick.?start\|getting.?started" README.md` | 有快速開始 |
| C4.P1.2 | `grep -q "kickoff" README.md && grep -q "stats" README.md` | 有命令說明 |
| C4.P1.3 | `bash .ai/tests/run_all_tests.sh 2>&1 \| grep -qE "passed\|✓"` | 輸出清晰 |

#### P2 - 加分項

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C4.P2.1 | `test -f docs/getting-started.md -o -f .ai/docs/getting-started.md` | 有教程 |
| C4.P2.2 | `find docs .ai/docs -name "*.md" -exec grep -l "architecture" {} \; 2>/dev/null \| head -1` | 有架構文件 |
| C4.P2.3 | `grep -qE "\-\-dry-run" .ai/scripts/kickoff.sh` | 有 dry-run |

---

### 5. 安全性 (10%)

#### P0 - 必須通過

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C5.P0.1 | `grep -q "escalation" .ai/config/workflow.yaml` | 有 escalation |
| C5.P0.2 | `grep -qE "\-\-dry-run" .ai/scripts/rollback.sh .ai/scripts/cleanup.sh` | 破壞性操作有 dry-run |

#### P1 - 重要功能

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C5.P1.1 | `grep -q "max_consecutive_failures" .ai/config/workflow.yaml` | 有失敗限制 |
| C5.P1.2 | `grep -qE "max_single_pr_files\|max_single_pr_lines" .ai/config/workflow.yaml` | 有 PR 限制 |
| C5.P1.3 | `grep -qE "require_human_approval\|pause_and_ask" .ai/config/workflow.yaml` | 有人工審核 |

#### P2 - 加分項

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| C5.P2.1 | `! grep -rE "ghp_\|token.*=" .ai/results/ .ai/state/ 2>/dev/null \| grep -v schema` | 無敏感資訊 |
| C5.P2.2 | `grep -q "\-\-auto" .ai/commands/start-work.md` | merge 用 --auto |

---

## 配置一致性檢查

驗證 `workflow.yaml` 與實際 repo 狀態一致。每項 FAIL 從核心流程扣 0.5 分。

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| R1 | `BRANCH=$(python3 -c "import yaml; print(yaml.safe_load(open('.ai/config/workflow.yaml'))['git']['integration_branch'])") && git rev-parse --verify "$BRANCH" 2>/dev/null \|\| git rev-parse --verify "origin/$BRANCH" 2>/dev/null` | 分支存在 |
| R2 | `python3 -c "import yaml,os; c=yaml.safe_load(open('.ai/config/workflow.yaml')); exit(0 if all(os.path.exists(r['path']) for r in c['repos']) else 1)"` | 路徑存在 |
| R3 | `python3 .ai/scripts/validate_config.py` | type-specific 驗證通過 |

### R3 Type-Specific 驗證規則 (v3.1 新增)

`validate_config.py` 會根據 `repos[].type` 進行額外驗證：

| type | 驗證規則 |
|------|----------|
| `submodule` | `.gitmodules` 必須存在且包含該 path，且該 path 下應是 git repo |
| `directory` | path 必須是目錄（警告：如有 `.git` 建議改用 submodule） |
| `root` | path 必須是 `./` 或空 |

---

## Doc Drift 檢查

檢查是否引用過期路徑。每處 FAIL 從易用性扣 0.5 分。

```bash
# 不應有輸出
grep -rn "scripts/ai/" .ai/ CLAUDE.md AGENTS.md README.md 2>/dev/null
```

---

## 負向測試（可選加分）

驗證系統能正確拒絕錯誤輸入。每項 +0.25 分（最多 +1.0）。

| ID | 驗證指令 | PASS 條件 |
|----|----------|-----------|
| NEG.1 | `! python3 .ai/scripts/validate_config.py /nonexistent 2>/dev/null` | 錯誤路徑 exit 非 0 |
| NEG.2 | `! bash .ai/scripts/run_issue_codex.sh 2>/dev/null` | 缺參數 exit 非 0 |
| NEG.3 | `! python3 .ai/scripts/parse_tasks.py /nonexistent 2>/dev/null` | 錯誤路徑 exit 非 0 |
| NEG.4 | `echo "" \| bash .ai/scripts/analyze_failure.sh - 2>/dev/null; test $? -eq 0` | 空輸入不崩潰 |

---

## 評分等級

| 分數 | 等級 | 說明 |
|------|------|------|
| 9.0 - 10.0 | A | 生產就緒（需 Online Gate PASS） |
| 8.0 - 8.9 | B | 功能完整 |
| 7.0 - 7.9 | C | 核心可用 |
| 6.0 - 6.9 | D | 有缺失 |
| < 6.0 | F | 不可用 |

---

## 如何使用

### 預設模式（Offline）

任何人 clone 下來就能評估 Kit 品質：

```bash
bash .ai/scripts/evaluate.sh
# 或
bash .ai/scripts/evaluate.sh --offline
```

### 完整模式（Online）

需要 gh auth + 網路，驗證完整流程：

```bash
bash .ai/scripts/evaluate.sh --online
```

### CI 整合

```yaml
# .github/workflows/evaluate.yml
- name: Evaluate Kit
  run: bash .ai/scripts/evaluate.sh --offline
```

---

## 完整評估腳本

實際腳本位於 `.ai/scripts/evaluate.sh`，以下為簡化版本：

```bash
#!/usr/bin/env bash
set -euo pipefail

MODE="${1:---offline}"
echo "=========================================="
echo "AI Workflow Kit - Evaluation v3.1"
echo "Mode: $MODE"
echo "=========================================="
echo ""

# === Offline Gate ===
echo "## Offline Gate"
OFFLINE_PASS=true

check_offline() {
  local id="$1" cmd="$2" desc="$3"
  if eval "$cmd" > /dev/null 2>&1; then
    echo "[PASS] $id: $desc"
  else
    echo "[FAIL] $id: $desc"
    OFFLINE_PASS=false
  fi
}

check_offline "O1" "bash .ai/scripts/scan_repo.sh 2>/dev/null || python3 .ai/scripts/scan_repo.py" "scan_repo"
check_offline "O2" "python3 -m json.tool .ai/state/repo_scan.json" "repo_scan.json"
check_offline "O3" "bash .ai/scripts/audit_project.sh 2>/dev/null || python3 .ai/scripts/audit_project.py" "audit_project"
check_offline "O4" "python3 -m json.tool .ai/state/audit.json" "audit.json"
check_offline "O5" "python3 .ai/scripts/validate_config.py" "validate_config (含 type-specific)"
check_offline "O6" "! file .ai/scripts/*.sh | grep -qE 'CRLF|UTF-16'" "無 CRLF/UTF-16"
check_offline "O7" "! file README.md CLAUDE.md AGENTS.md 2>/dev/null | grep -qE 'UTF-16'" "文件編碼"
check_offline "O8" "bash .ai/tests/run_all_tests.sh" "測試套件"

echo ""
if [ "$OFFLINE_PASS" = false ]; then
  echo "❌ Offline Gate FAILED"
  echo "評分上限: 4.0 (F)"
  exit 1
fi
echo "✅ Offline Gate PASSED"

# === Online Gate (如果請求) ===
SCORE_CAP=8.5
if [ "$MODE" = "--online" ]; then
  echo ""
  echo "## Online Gate"

  # 前置條件
  if ! command -v gh > /dev/null 2>&1; then
    echo "[SKIP] gh CLI 未安裝"
  elif ! gh auth status > /dev/null 2>&1; then
    echo "[SKIP] gh 未登入"
  elif ! curl -s --max-time 5 https://api.github.com > /dev/null 2>&1; then
    echo "[SKIP] 無法連線 GitHub"
  else
    ONLINE_PASS=true

    check_online() {
      local id="$1" cmd="$2" desc="$3"
      if eval "$cmd" > /dev/null 2>&1; then
        echo "[PASS] $id: $desc"
      else
        echo "[FAIL] $id: $desc"
        ONLINE_PASS=false
      fi
    }

    check_online "N1" "bash .ai/scripts/kickoff.sh --dry-run" "kickoff"
    check_online "N2" "bash .ai/scripts/rollback.sh 99999 --dry-run 2>&1 | grep -qiE 'not found|usage|dry'" "rollback"
    check_online "N3" "bash .ai/scripts/stats.sh --json | python3 -m json.tool" "stats"

    echo ""
    if [ "$ONLINE_PASS" = true ]; then
      echo "✅ Online Gate PASSED"
      SCORE_CAP=10.0
    else
      echo "⚠️ Online Gate FAILED"
    fi
  fi
fi

echo ""
echo "=========================================="
echo "評分上限: $SCORE_CAP"
echo "=========================================="
```

---

## 版本紀錄

| 版本 | 日期 | 說明 |
|------|------|------|
| 1.0 | 2025-12-19 | 初始版本 |
| 2.0 | 2025-12-19 | 加入 Must-Pass Gate、P0/P1/P2 分級 |
| 3.0 | 2025-12-19 | 拆分 Offline/Online Gate、加入 SKIP 狀態、明確前置條件 |
| 3.1 | 2025-12-19 | 加入前置條件章節、統一 Online Gate 檢查項目（含 rollback）、type-specific 配置驗證 |
