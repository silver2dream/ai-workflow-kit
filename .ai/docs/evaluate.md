# AI Workflow Kit - 評分標準 v2

## 專案核心目的

> **用 AI (Claude Code + Codex) 自動化「Spec → 實作 → PR → 合併」的開發流程**

**目標用戶**：想用 AI 自動化開發流程的開發者/團隊

---

## 評分規則

### Must-Pass Gate（P0 一票否決）

以下檢查**任一失敗**，整體評分上限為 **4/10 (F)**：

| Gate | 驗證指令 | 通過條件 |
|------|----------|----------|
| G1 | `bash .ai/scripts/kickoff.sh --dry-run` | exit 0 |
| G2 | `bash .ai/scripts/scan_repo.sh && cat .ai/state/repo_scan.json \| python3 -m json.tool` | exit 0 且 JSON 可 parse |
| G3 | `bash .ai/scripts/audit_project.sh && cat .ai/state/audit.json \| python3 -m json.tool` | exit 0 且 JSON 可 parse |
| G4 | `python3 .ai/scripts/validate_config.py` | exit 0 |
| G5 | `file .ai/scripts/*.sh \| grep -v "ASCII text"` | 無輸出（所有 .sh 為 ASCII/UTF-8） |
| G6 | `bash .ai/tests/run_all_tests.sh` | exit 0 |

```bash
# 執行 Must-Pass Gate 檢查
echo "=== Must-Pass Gate ===" && \
bash .ai/scripts/kickoff.sh --dry-run && \
bash .ai/scripts/scan_repo.sh && python3 -m json.tool .ai/state/repo_scan.json > /dev/null && \
bash .ai/scripts/audit_project.sh && python3 -m json.tool .ai/state/audit.json > /dev/null && \
python3 .ai/scripts/validate_config.py && \
bash .ai/tests/run_all_tests.sh && \
echo "=== All Gates Passed ==="
```

---

## 評分面向

| 面向 | 權重 | P0 項數 | P1 項數 | P2 項數 |
|------|------|---------|---------|---------|
| 核心流程 | 30% | 4 | 6 | 4 |
| 可靠性 | 25% | 3 | 5 | 3 |
| 可擴展性 | 20% | 2 | 4 | 3 |
| 易用性 | 15% | 1 | 3 | 3 |
| 安全性 | 10% | 2 | 3 | 2 |

### 分數計算

每個面向的分數：
- P0 未通過：該面向最高 4/10
- P1 未通過：每項扣 1 分
- P2 未通過：每項扣 0.5 分
- 基礎分 10，扣到 0 為止

```
面向分數 = min(10, 10 - (P1未通過數 × 1) - (P2未通過數 × 0.5))
如果有 P0 未通過：面向分數 = min(面向分數, 4)

總分 = Σ(面向分數 × 權重)
```

---

## Checkpoint 清單

### 1. 核心流程 (30%)

#### P0 - 必須通過

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C1.P0.1 | `bash .ai/scripts/kickoff.sh --help 2>&1 \| head -5` | 有輸出且 exit 0 |
| C1.P0.2 | `bash .ai/scripts/run_issue_codex.sh 2>&1 \| grep -i usage` | 顯示 usage |
| C1.P0.3 | `test -f CLAUDE.md && test -f AGENTS.md` | exit 0 |
| C1.P0.4 | `test -f .ai/commands/start-work.md` | exit 0 |

#### P1 - 重要功能

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C1.P1.1 | `python3 .ai/scripts/parse_tasks.py .ai/specs/*/tasks.md --json 2>/dev/null \| python3 -m json.tool` | 有效 JSON 輸出 |
| C1.P1.2 | `grep -l "Phase A\|Phase B\|Phase C\|Phase D" .ai/commands/start-work.md` | 找到檔案 |
| C1.P1.3 | `grep "Ticket Format\|STRICT TEMPLATE" CLAUDE.md` | 有匹配 |
| C1.P1.4 | `bash .ai/scripts/stats.sh --json \| python3 -m json.tool` | 有效 JSON |
| C1.P1.5 | `test -d .ai/results && test -d .ai/runs && test -d .ai/state` | exit 0 |
| C1.P1.6 | `grep "STOP" .ai/scripts/kickoff.sh` | 有匹配（支援停止機制） |

#### P2 - 加分項

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C1.P2.1 | `bash .ai/scripts/stats.sh --html && test -f .ai/state/stats.html` | 產生 HTML |
| C1.P2.2 | `grep "_depends_on" .ai/scripts/parse_tasks.py` | 支援依賴語法 |
| C1.P2.3 | `grep "Coordination\|sequential\|parallel" .ai/commands/start-work.md` | 支援 multi-repo |
| C1.P2.4 | `test -f .ai/templates/design.md.example` | 有範例模板 |

---

### 2. 可靠性 (25%)

#### P0 - 必須通過

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C2.P0.1 | `file .ai/scripts/*.sh \| grep CRLF` | 無輸出（無 CRLF） |
| C2.P0.2 | `python3 -m json.tool .ai/config/failure_patterns.json > /dev/null` | exit 0 |
| C2.P0.3 | `test -f .ai/scripts/attempt_guard.sh && bash .ai/scripts/attempt_guard.sh 2>&1 \| grep -i usage` | 有 usage |

#### P1 - 重要功能

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C2.P1.1 | `echo "cannot find package" \| bash .ai/scripts/analyze_failure.sh - \| grep -i "matched\|type"` | 有匹配結果 |
| C2.P1.2 | `bash .ai/scripts/rollback.sh 2>&1 \| grep -i usage` | 有 usage |
| C2.P1.3 | `bash .ai/scripts/rollback.sh 99999 --dry-run 2>&1` | 有 dry-run 輸出 |
| C2.P1.4 | `bash .ai/scripts/cleanup.sh --dry-run 2>&1` | exit 0 |
| C2.P1.5 | `grep "retryable.*true" .ai/config/failure_patterns.json` | 有可重試模式 |

#### P2 - 加分項

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C2.P2.1 | `grep "failure_history" .ai/scripts/*.sh` | 有歷史記錄機制 |
| C2.P2.2 | `grep "stats_history" .ai/scripts/stats.sh` | 有趨勢追蹤 |
| C2.P2.3 | `bash .ai/scripts/cleanup.sh --days 1 --dry-run 2>&1` | 支援 days 參數 |

---

### 3. 可擴展性 (20%)

#### P0 - 必須通過

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C3.P0.1 | `python3 -m json.tool .ai/config/workflow.schema.json > /dev/null` | exit 0 |
| C3.P0.2 | `test -f .ai/config/workflow.yaml && python3 -c "import yaml; yaml.safe_load(open('.ai/config/workflow.yaml'))"` | exit 0 |

#### P1 - 重要功能

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C3.P1.1 | `test -f .ai/templates/CLAUDE.md.j2 && test -f .ai/templates/AGENTS.md.j2` | exit 0 |
| C3.P1.2 | `ls .ai/templates/ci-*.yml.j2 \| wc -l` | ≥ 5 |
| C3.P1.3 | `bash .ai/scripts/generate.sh --help 2>&1 \| head -3` | 有輸出 |
| C3.P1.4 | `grep -E "submodule\|directory\|root" .ai/config/workflow.schema.json` | 支援三種 repo type |

#### P2 - 加分項

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C3.P2.1 | `test -f .ai/scripts/install.sh` | 有安裝腳本 |
| C3.P2.2 | `test -f .ai/scripts/init.sh` | 有初始化腳本 |
| C3.P2.3 | `ls .ai/templates/ci-*.yml.j2 \| wc -l` | ≥ 8 |

#### 條件化檢查（僅在適用時執行）

| 條件 | 驗證指令 | 通過條件 |
|------|----------|----------|
| 有 submodule | `test -f .gitmodules && test -f .ai/templates/validate-submodules.yml.j2` | exit 0 |
| 有 Go repo | `grep -q "language: go" .ai/config/workflow.yaml && test -f .ai/templates/ci-go.yml.j2` | exit 0 |
| 有 Unity repo | `grep -q "language: unity" .ai/config/workflow.yaml && test -f .ai/templates/ci-unity.yml.j2` | exit 0 |

---

### 4. 易用性 (15%)

#### P0 - 必須通過

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C4.P0.1 | `file README.md \| grep -E "UTF-8\|ASCII"` | 有匹配 |

#### P1 - 重要功能

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C4.P1.1 | `grep -E "Quick Start\|Getting Started" README.md` | 有快速開始 |
| C4.P1.2 | `grep "kickoff.sh" README.md && grep "stats.sh" README.md` | 有命令說明 |
| C4.P1.3 | `bash .ai/tests/run_all_tests.sh 2>&1 \| grep -E "passed\|failed\|✓\|✗"` | 有清晰輸出 |

#### P2 - 加分項

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C4.P2.1 | `test -f docs/getting-started.md \|\| test -f .ai/docs/getting-started.md` | 有教程 |
| C4.P2.2 | `grep -r "architecture\|diagram" docs/ .ai/docs/ 2>/dev/null` | 有架構文件 |
| C4.P2.3 | `bash .ai/scripts/kickoff.sh --help 2>&1 \| grep -E "dry-run\|background"` | 說明選項 |

---

### 5. 安全性 (10%)

#### P0 - 必須通過

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C5.P0.1 | `grep "escalation" .ai/config/workflow.yaml` | 有 escalation 配置 |
| C5.P0.2 | `grep -E "dry-run\|--dry-run" .ai/scripts/rollback.sh .ai/scripts/cleanup.sh` | 破壞性操作有 dry-run |

#### P1 - 重要功能

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C5.P1.1 | `grep "max_consecutive_failures" .ai/config/workflow.yaml` | 有失敗限制 |
| C5.P1.2 | `grep "max_single_pr_files\|max_single_pr_lines" .ai/config/workflow.yaml` | 有 PR 大小限制 |
| C5.P1.3 | `grep -E "require_human_approval\|pause_and_ask" .ai/config/workflow.yaml` | 有人工審核觸發 |

#### P2 - 加分項

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| C5.P2.1 | `grep -rE "password\|token\|secret" .ai/results/ .ai/state/*.json 2>/dev/null \| grep -v ".schema.json"` | 無敏感資訊落盤 |
| C5.P2.2 | `grep "\-\-auto" .ai/commands/start-work.md` | Merge 使用 --auto |

---

## 配置與現實一致性檢查

這些檢查驗證 `workflow.yaml` 是否與實際 repo 狀態一致。任一失敗扣 1 分（從核心流程）。

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| R1 | `BRANCH=$(python3 -c "import yaml; print(yaml.safe_load(open('.ai/config/workflow.yaml'))['git']['integration_branch'])") && git rev-parse --verify "$BRANCH" 2>/dev/null \|\| git rev-parse --verify "origin/$BRANCH" 2>/dev/null` | 分支存在 |
| R2 | `python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); [exit(1) for r in c['repos'] if r['type']=='submodule' and not __import__('os').path.isfile(r['path'].rstrip('/')+'/.git')]"` | submodule 實際存在 |
| R3 | `python3 -c "import yaml; c=yaml.safe_load(open('.ai/config/workflow.yaml')); [exit(1) for r in c['repos'] if r['type']=='directory' and not __import__('os').path.isdir(r['path'])]"` | directory 實際存在 |

---

## Doc Drift 檢查

檢查文件是否引用過期路徑/分支名。每找到一處扣 0.5 分（從易用性）。

```bash
# 過期路徑檢查（應無輸出）
grep -rn "scripts/ai/" .ai/ CLAUDE.md AGENTS.md README.md 2>/dev/null

# 過期分支檢查（根據專案調整）
# grep -rn "feat/aether\|feat/old-branch" .ai/ CLAUDE.md AGENTS.md 2>/dev/null

# 過期常數檢查
grep -rn "hardcoded-old-value" .ai/ 2>/dev/null
```

---

## 負向測試（可選加分）

以下測試驗證系統能正確拒絕錯誤輸入。每通過一項加 0.5 分（最多 2 分）。

| ID | 驗證指令 | 通過條件 |
|----|----------|----------|
| N1 | `echo "invalid yaml {{{{" > /tmp/bad.yaml && python3 .ai/scripts/validate_config.py /tmp/bad.yaml 2>&1; echo $?` | exit 非 0 |
| N2 | `bash .ai/scripts/run_issue_codex.sh 2>&1; echo $?` | 缺參數時 exit 非 0 |
| N3 | `echo "[]" > /tmp/empty.json && bash .ai/scripts/analyze_failure.sh /tmp/empty.json 2>&1` | 處理空輸入不崩潰 |
| N4 | `python3 .ai/scripts/parse_tasks.py /nonexistent 2>&1; echo $?` | 檔案不存在時 exit 非 0 |

---

## 評分等級

| 分數 | 等級 | 說明 |
|------|------|------|
| 9.0+ | A | 生產就緒 |
| 8.0-8.9 | B | 功能完整，小改進空間 |
| 7.0-7.9 | C | 核心可用，需補強 |
| 6.0-6.9 | D | 有缺失，需改進 |
| <6.0 / Must-Pass 失敗 | F | 不可用 |

---

## 可選加分項（不影響基礎分）

以下功能為額外加分，不納入基礎評分：

| 項目 | 加分 | 條件 |
|------|------|------|
| 執行時間追蹤 | +0.5 | `grep "duration_seconds\|metrics" .ai/scripts/write_result.sh` |
| 歷史趨勢圖表 | +0.5 | `grep "trends" .ai/scripts/stats.sh` |
| 通知整合 | +0.5 | `test -f .ai/scripts/notify.sh && grep -E "slack\|discord" .ai/scripts/notify.sh` |
| 負向測試全過 | +2.0 | N1-N4 全部通過 |

---

## 完整評估腳本

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "=========================================="
echo "AI Workflow Kit - Evaluation v2"
echo "=========================================="
echo ""

SCORE=0
NOTES=""

# Must-Pass Gate
echo "## Must-Pass Gate (任一失敗 = F)"
GATE_PASSED=true

gate_check() {
  local name="$1"
  local cmd="$2"
  if eval "$cmd" > /dev/null 2>&1; then
    echo "✓ $name"
  else
    echo "✗ $name"
    GATE_PASSED=false
  fi
}

gate_check "G1: kickoff --dry-run" "bash .ai/scripts/kickoff.sh --dry-run"
gate_check "G2: scan_repo.sh" "bash .ai/scripts/scan_repo.sh && python3 -m json.tool .ai/state/repo_scan.json"
gate_check "G3: audit_project.sh" "bash .ai/scripts/audit_project.sh && python3 -m json.tool .ai/state/audit.json"
gate_check "G4: validate_config.py" "python3 .ai/scripts/validate_config.py"
gate_check "G5: No CRLF in .sh" "! file .ai/scripts/*.sh | grep -q CRLF"
gate_check "G6: Tests pass" "bash .ai/tests/run_all_tests.sh"

echo ""
if [ "$GATE_PASSED" = false ]; then
  echo "❌ Must-Pass Gate FAILED - 評分上限: 4/10 (F)"
  echo ""
  echo "請先修復 Gate 檢查後再評估。"
  exit 1
fi
echo "✅ Must-Pass Gate PASSED"
echo ""

# 繼續評分...
echo "## 詳細評分"
echo "(執行各面向 checkpoint...)"
echo ""
echo "評分完成後會顯示："
echo "- 各面向分數"
echo "- 未通過項目清單"
echo "- 總分和等級"
```

---

## 版本紀錄

| 版本 | 日期 | 說明 |
|------|------|------|
| 1.0 | 2025-12-19 | 初始版本 |
| 2.0 | 2025-12-19 | 加入 Must-Pass Gate、P0/P1/P2 分級、可驗證指令、條件化檢查、Doc Drift、負向測試 |
