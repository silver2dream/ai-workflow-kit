#!/usr/bin/env bash
# Test: Skills 結構驗證
# 驗證 Skill 檔案結構和 frontmatter 格式

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Test: Skills Structure ==="
echo ""

SKILL_DIR="$REPO_ROOT/.ai/skills/principal-workflow"

# Test 1: SKILL.md 存在
echo "Test 1: SKILL.md 存在..."

if [[ -f "$SKILL_DIR/SKILL.md" ]]; then
  echo "  ✓ SKILL.md 存在"
else
  echo "  ✗ SKILL.md 不存在"
  exit 1
fi

# Test 2: YAML frontmatter 格式
echo ""
echo "Test 2: YAML frontmatter 格式..."

SKILL_CONTENT=$(cat "$SKILL_DIR/SKILL.md" | tr -d '\r')

# 檢查 frontmatter 開始
FIRST_LINE=$(echo "$SKILL_CONTENT" | sed -n '1p' | tr -d '\r')
if [[ "$FIRST_LINE" == "---" ]]; then
  echo "  ✓ frontmatter 開始標記正確"
else
  echo "  ✗ frontmatter 開始標記錯誤 (got: '$FIRST_LINE')"
  exit 1
fi

# 檢查 name 是單行
if echo "$SKILL_CONTENT" | grep -q "^name: principal-workflow$"; then
  echo "  ✓ name 格式正確"
else
  echo "  ✗ name 格式錯誤"
  exit 1
fi

# 檢查 description 是單行（不是多行）
DESC_LINE=$(echo "$SKILL_CONTENT" | grep "^description:")
if echo "$DESC_LINE" | grep -qv "|"; then
  echo "  ✓ description 是單行"
else
  echo "  ✗ description 不是單行（使用了 | 或 >）"
  exit 1
fi

# 檢查 allowed-tools 是單行逗號分隔
TOOLS_LINE=$(echo "$SKILL_CONTENT" | grep "^allowed-tools:")
if echo "$TOOLS_LINE" | grep -q "Read, Grep, Glob, Bash"; then
  echo "  ✓ allowed-tools 格式正確"
else
  echo "  ✗ allowed-tools 格式錯誤"
  exit 1
fi

# Test 3: 必要檔案存在
echo ""
echo "Test 3: 必要檔案存在..."

REQUIRED_FILES=(
  "phases/main-loop.md"
  "tasks/generate-tasks.md"
  "tasks/create-task.md"
  "tasks/review-pr.md"
  "references/contracts.md"
)

for file in "${REQUIRED_FILES[@]}"; do
  if [[ -f "$SKILL_DIR/$file" ]]; then
    echo "  ✓ $file"
  else
    echo "  ✗ $file 不存在"
    exit 1
  fi
done

# Test 4: main-loop.md 包含必要內容
echo ""
echo "Test 4: main-loop.md 內容..."

MAIN_LOOP="$SKILL_DIR/phases/main-loop.md"

if grep -q "analyze_next.sh" "$MAIN_LOOP"; then
  echo "  ✓ 包含 analyze_next.sh 呼叫"
else
  echo "  ✗ 缺少 analyze_next.sh 呼叫"
  exit 1
fi

if grep -q "NEXT_ACTION" "$MAIN_LOOP"; then
  echo "  ✓ 包含 NEXT_ACTION routing"
else
  echo "  ✗ 缺少 NEXT_ACTION routing"
  exit 1
fi

if grep -q "loop_count" "$MAIN_LOOP"; then
  echo "  ✓ 包含 Loop Safety"
else
  echo "  ✗ 缺少 Loop Safety"
  exit 1
fi

# Test 5: contracts.md 包含變數契約
echo ""
echo "Test 5: contracts.md 內容..."

CONTRACTS="$SKILL_DIR/references/contracts.md"

REQUIRED_ACTIONS=("generate_tasks" "create_task" "dispatch_worker" "check_result" "review_pr" "all_complete" "none")

for action in "${REQUIRED_ACTIONS[@]}"; do
  if grep -q "$action" "$CONTRACTS"; then
    echo "  ✓ 包含 $action"
  else
    echo "  ✗ 缺少 $action"
    exit 1
  fi
done

echo ""
echo "=== All tests passed ==="
