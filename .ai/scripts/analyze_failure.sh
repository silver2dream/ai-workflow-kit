#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# analyze_failure.sh - 分析失敗日誌並識別錯誤類型
# ============================================================================
# 用法:
#   bash .ai/scripts/analyze_failure.sh <log_file>
#   echo "error log" | bash .ai/scripts/analyze_failure.sh -
#
# 輸出 (JSON):
#   {
#     "matched": true,
#     "pattern_id": "go_compile_error",
#     "type": "compile_error",
#     "retryable": false,
#     "suggestion": "Fix the compilation error",
#     "matched_text": "undefined: foo"
#   }
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_ROOT="$(dirname "$SCRIPT_DIR")"
PATTERNS_FILE="$AI_ROOT/config/failure_patterns.json"

# 讀取日誌
if [[ "${1:-}" == "-" ]]; then
  LOG_CONTENT=$(cat)
elif [[ -n "${1:-}" ]] && [[ -f "$1" ]]; then
  LOG_CONTENT=$(cat "$1")
else
  echo '{"matched":false,"type":"unknown","retryable":false,"suggestion":"No log provided"}' 
  exit 0
fi

# 檢查 patterns 文件
if [[ ! -f "$PATTERNS_FILE" ]]; then
  echo '{"matched":false,"type":"unknown","retryable":false,"suggestion":"Patterns file not found"}'
  exit 0
fi

# 使用 Python 進行模式匹配
python3 - "$PATTERNS_FILE" "$LOG_CONTENT" <<'PYTHON'
import sys
import json
import re

patterns_file = sys.argv[1]
log_content = sys.argv[2] if len(sys.argv) > 2 else ""

# 載入模式
try:
    with open(patterns_file, 'r', encoding='utf-8') as f:
        data = json.load(f)
        patterns = data.get('patterns', [])
except Exception as e:
    print(json.dumps({
        "matched": False,
        "type": "unknown",
        "retryable": False,
        "suggestion": f"Failed to load patterns: {e}"
    }))
    sys.exit(0)

# 匹配模式
result = {
    "matched": False,
    "type": "unknown",
    "retryable": False,
    "suggestion": "Unknown error"
}

for pattern in patterns:
    try:
        regex = pattern.get('regex', '')
        if regex and re.search(regex, log_content, re.IGNORECASE | re.MULTILINE):
            # 找到匹配的文字
            match = re.search(regex, log_content, re.IGNORECASE | re.MULTILINE)
            matched_text = match.group(0) if match else ""
            
            result = {
                "matched": True,
                "pattern_id": pattern.get('id', 'unknown'),
                "type": pattern.get('type', 'unknown'),
                "language": pattern.get('language', '*'),
                "retryable": pattern.get('retryable', False),
                "max_retries": pattern.get('max_retries', 0),
                "retry_delay_seconds": pattern.get('retry_delay_seconds', 0),
                "suggestion": pattern.get('suggestion', ''),
                "matched_text": matched_text[:200]  # 限制長度
            }
            break
    except re.error:
        continue

print(json.dumps(result, ensure_ascii=False))
PYTHON
