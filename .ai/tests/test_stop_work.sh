#!/usr/bin/env bash
# Test: stop_work.sh
# 驗證報告生成和 session 結束

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Test: stop_work.sh ==="
echo ""

# 確保必要目錄存在
mkdir -p "$REPO_ROOT/.ai/state"
mkdir -p "$REPO_ROOT/.ai/exe-logs"

# 初始化狀態
echo "5" > "$REPO_ROOT/.ai/state/loop_count"
echo "2" > "$REPO_ROOT/.ai/state/consecutive_failures"

cd "$REPO_ROOT"

# Test 1: 生成報告
echo "Test 1: 生成報告..."

# 記錄報告數量
BEFORE_COUNT=$(ls -1 "$REPO_ROOT/.ai/state/workflow-report-"*.md 2>/dev/null | wc -l | tr -d ' \r\n' || echo "0")
BEFORE_COUNT=${BEFORE_COUNT:-0}

bash .ai/scripts/stop_work.sh "test_exit" 2>/dev/null || true

AFTER_COUNT=$(ls -1 "$REPO_ROOT/.ai/state/workflow-report-"*.md 2>/dev/null | wc -l | tr -d ' \r\n' || echo "0")
AFTER_COUNT=${AFTER_COUNT:-0}

if [[ "$AFTER_COUNT" -gt "$BEFORE_COUNT" ]]; then
  echo "  ✓ 報告已生成"
else
  echo "  ✗ 報告未生成 (before=$BEFORE_COUNT, after=$AFTER_COUNT)"
  exit 1
fi

# Test 2: 清理 loop_count
echo ""
echo "Test 2: 清理 loop_count..."

if [[ ! -f "$REPO_ROOT/.ai/state/loop_count" ]]; then
  echo "  ✓ loop_count 已清理"
else
  echo "  ✗ loop_count 未清理"
  exit 1
fi

# Test 3: 清理 consecutive_failures
echo ""
echo "Test 3: 清理 consecutive_failures..."

if [[ ! -f "$REPO_ROOT/.ai/state/consecutive_failures" ]]; then
  echo "  ✓ consecutive_failures 已清理"
else
  echo "  ✗ consecutive_failures 未清理"
  exit 1
fi

# Test 4: 報告內容包含 exit reason
echo ""
echo "Test 4: 報告內容..."

LATEST_REPORT=$(ls -1t "$REPO_ROOT/.ai/state/workflow-report-"*.md 2>/dev/null | head -1)

if grep -q "test_exit" "$LATEST_REPORT"; then
  echo "  ✓ 報告包含 exit reason"
else
  echo "  ✗ 報告不包含 exit reason"
  exit 1
fi

echo ""
echo "=== All tests passed ==="
