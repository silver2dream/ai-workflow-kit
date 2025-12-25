#!/usr/bin/env bash
# Test: analyze_next.sh stdout/stderr 分離
# 驗證 stdout 只輸出可 eval 的變數

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Test: analyze_next.sh ==="
echo ""

# 確保必要目錄存在
mkdir -p "$REPO_ROOT/.ai/state"
mkdir -p "$REPO_ROOT/.ai/exe-logs"

# 初始化 loop_count
echo "0" > "$REPO_ROOT/.ai/state/loop_count"
echo "0" > "$REPO_ROOT/.ai/state/consecutive_failures"

cd "$REPO_ROOT"

# Test 1: stdout 只包含變數賦值
echo "Test 1: stdout 只包含變數賦值..."

STDOUT_OUTPUT=$(bash .ai/scripts/analyze_next.sh 2>/dev/null || true)

# 檢查每一行是否都是 KEY=VALUE 格式
INVALID_LINES=0
while IFS= read -r line; do
  if [[ -z "$line" ]]; then
    continue
  fi
  if ! echo "$line" | grep -qE '^[A-Z_]+='; then
    echo "  ✗ 無效的 stdout 行: $line"
    INVALID_LINES=$((INVALID_LINES + 1))
  fi
done <<< "$STDOUT_OUTPUT"

if [[ "$INVALID_LINES" -eq 0 ]]; then
  echo "  ✓ stdout 格式正確"
else
  echo "  ✗ 發現 $INVALID_LINES 行無效輸出"
  exit 1
fi

# Test 2: 可以 eval stdout
echo ""
echo "Test 2: stdout 可以 eval..."

if eval "$STDOUT_OUTPUT" 2>/dev/null; then
  echo "  ✓ eval 成功"
else
  echo "  ✗ eval 失敗"
  exit 1
fi

# Test 3: NEXT_ACTION 有值
echo ""
echo "Test 3: NEXT_ACTION 有值..."

if [[ -n "$NEXT_ACTION" ]]; then
  echo "  ✓ NEXT_ACTION=$NEXT_ACTION"
else
  echo "  ✗ NEXT_ACTION 為空"
  exit 1
fi

# Test 4: stderr 包含 log
echo ""
echo "Test 4: stderr 包含 log..."

STDERR_OUTPUT=$(bash .ai/scripts/analyze_next.sh 2>&1 >/dev/null || true)

if echo "$STDERR_OUTPUT" | grep -q "\[PRINCIPAL\]"; then
  echo "  ✓ stderr 包含 [PRINCIPAL] log"
else
  echo "  ✗ stderr 沒有 [PRINCIPAL] log"
  exit 1
fi

# Test 5: Loop Safety - max_loop
echo ""
echo "Test 5: Loop Safety - max_loop..."

echo "1000" > "$REPO_ROOT/.ai/state/loop_count"

STDOUT_OUTPUT=$(bash .ai/scripts/analyze_next.sh 2>/dev/null || true)
eval "$STDOUT_OUTPUT"

if [[ "$NEXT_ACTION" == "none" ]] && [[ "$EXIT_REASON" == "max_loop_reached" ]]; then
  echo "  ✓ max_loop 觸發正確"
else
  echo "  ✗ max_loop 未正確觸發 (NEXT_ACTION=$NEXT_ACTION, EXIT_REASON=$EXIT_REASON)"
  exit 1
fi

# 清理
echo "0" > "$REPO_ROOT/.ai/state/loop_count"

echo ""
echo "=== All tests passed ==="
