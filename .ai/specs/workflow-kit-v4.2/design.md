# AI Workflow Kit v4.2 - Design

## Overview

v4.2 修正評估框架的規格嚴謹性問題，確保邏輯一致、無矛盾。

## Architecture

### 變更範圍

1. `.ai/scripts/evaluate.sh` - 主要邏輯修改
2. `.ai/docs/evaluate.md` - 文檔更新

### 不變範圍

- Offline Gate 核心檢查項目 (O0-O5, O7-O10)
- Online Gate 檢查項目 (N1-N3)
- 面向評分公式

---

## Components and Interfaces

### evaluate.sh 變更

#### 1. O6 移出 Offline Gate

```bash
# 舊：O6 在 Offline Gate 內
# 新：O6 移到獨立的 Extensibility Check 區塊

# === Extensibility Checks (不影響 Offline Gate) ===
echo "## Extensibility Checks"

# O6: CI/分支對齊 (P1, 不影響 Gate)
if [ ! -d .github/workflows ]; then
  echo "[SKIP] EXT1: CI/branch alignment (no .github/workflows)"
elif [ "$(find .github/workflows -name '*.yml' -o -name '*.yaml' 2>/dev/null | wc -l)" -eq 0 ]; then
  echo "[FAIL] EXT1: .github/workflows exists but empty"
else
  # 執行 Python 檢查...
fi
```

#### 2. SKIP 白名單

```bash
# SKIP 只允許以下情況：
# 1. 可選依賴缺少 (file 指令)
# 2. 明確不適用 (無 CI workflows)

# 不允許 SKIP 的情況改為 FAIL：
# - 讀不到 integration_branch → FAIL (配置錯誤)
# - Python 執行錯誤 → FAIL (環境問題)
```

#### 3. --strict 模式

```bash
#!/usr/bin/env bash
# ...
STRICT=false
while [[ $# -gt 0 ]]; do
  case $1 in
    --strict) STRICT=true; shift ;;
    --online) MODE="--online"; shift ;;
    --offline) MODE="--offline"; shift ;;
    *) shift ;;
  esac
done

# 在 O3/O4 後加入：
if [ "$STRICT" = true ]; then
  # 檢查 audit.json 是否有 P0/P1
  P0_COUNT=$(python3 -c "
import json
with open('.ai/state/audit.json') as f:
    audit = json.load(f)
p0 = [f for f in audit.get('findings', []) if f.get('severity') == 'P0']
print(len(p0))
" 2>/dev/null || echo "0")
  
  if [ "$P0_COUNT" -gt 0 ]; then
    check_fail "O4.1" "audit has $P0_COUNT P0 findings (--strict)"
  fi
fi
```

---

## Data Models

無新增資料模型。

---

## Correctness Properties

### Property 1: SKIP 白名單一致性

*For any* check that returns SKIP, the reason must be in the allowed whitelist (optional dependency missing OR explicitly not applicable).

**Validates: Requirements 2.1, 2.4**

### Property 2: O6 不影響 Offline Gate

*For any* repo without CI workflows, Offline Gate result should not be affected by O6.

**Validates: Requirements 1.1, 1.2**

### Property 3: --strict 模式正確性

*For any* audit.json with P0 findings, running with --strict should FAIL.

**Validates: Requirements 6.2, 6.3**

---

## Error Handling

| 錯誤情況 | 處理方式 |
|----------|----------|
| 讀不到 integration_branch | FAIL (不是 SKIP) |
| Python 執行錯誤 | FAIL (不是 SKIP) |
| audit.json 無效 | FAIL |
| file 指令不存在 | SKIP (可選依賴) |

---

## Testing Strategy

### Unit Tests

1. 測試 O6 移出後 Offline Gate 不受影響
2. 測試 SKIP 白名單邏輯
3. 測試 --strict 模式

### Property Tests

1. SKIP 原因必須在白名單內
2. --strict 模式正確檢查 audit P0
