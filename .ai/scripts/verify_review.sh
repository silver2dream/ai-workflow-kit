#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# verify_review.sh - Review 驗證工具
# 驗證 AWK Review Comment 的完整性和正確性
# Requirements: 5.3, 7.4
#
# Exit codes:
#   0 - 驗證通過
#   1 - 驗證失敗（缺少必要欄位或格式錯誤）
#   2 - 需要修正（評分 < 7）
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Cross-platform jq wrapper
_jq() {
  if command -v jq &>/dev/null; then
    jq "$@"
  elif command -v jq.exe &>/dev/null; then
    jq.exe "$@"
  elif [[ -x "/mnt/c/Users/user/bin/jq.exe" ]]; then
    /mnt/c/Users/user/bin/jq.exe "$@"
  else
    echo "[ERROR] jq not found" >&2
    return 1
  fi
}

usage() {
  echo "Usage: $0 <review_comment_file> [session_log_dir]"
  echo ""
  echo "Arguments:"
  echo "  review_comment_file  Path to file containing the review comment"
  echo "  session_log_dir      Optional: Directory containing session logs for verification"
  echo ""
  echo "Exit codes:"
  echo "  0 - Verification passed"
  echo "  1 - Verification failed (missing fields or format error)"
  echo "  2 - Needs revision (score < 7)"
  exit 1
}

REVIEW_FILE="${1:-}"
SESSION_LOG_DIR="${2:-.ai/state/principal/sessions}"

if [[ -z "$REVIEW_FILE" ]] || [[ ! -f "$REVIEW_FILE" ]]; then
  usage
fi

REVIEW_CONTENT=$(cat "$REVIEW_FILE")
ERRORS=()
WARNINGS=()

echo "[verify_review] Verifying review comment..."

# ============================================================
# 1. Check AWK Review Comment marker
# ============================================================
if ! echo "$REVIEW_CONTENT" | grep -q "<!-- AWK Review -->"; then
  ERRORS+=("Missing AWK Review marker")
fi

# ============================================================
# 2. Extract and verify Session ID (Req 7.4 - Anti-Forgery)
# ============================================================
SESSION_ID=$(echo "$REVIEW_CONTENT" | grep -oP '(?<=Session: )[a-z]+-[0-9]{8}-[0-9]{6}-[a-f0-9]{4}' | head -1 || echo "")

if [[ -z "$SESSION_ID" ]]; then
  ERRORS+=("Missing or invalid Session ID")
else
  echo "[verify_review] Session ID: $SESSION_ID"
  
  # Local verification: check if session exists in session logs
  if [[ -d "$SESSION_LOG_DIR" ]]; then
    SESSION_LOG="$SESSION_LOG_DIR/${SESSION_ID}.json"
    if [[ -f "$SESSION_LOG" ]]; then
      echo "[verify_review] ✓ Session ID verified locally"
    else
      WARNINGS+=("Session ID not found in local logs (may be from different machine)")
    fi
  fi
fi

# ============================================================
# 3. Extract and verify Diff Hash
# ============================================================
DIFF_HASH=$(echo "$REVIEW_CONTENT" | grep -oP '(?<=Diff Hash: )[a-f0-9]{8,}' | head -1 || echo "")

if [[ -z "$DIFF_HASH" ]]; then
  ERRORS+=("Missing Diff Hash")
else
  echo "[verify_review] Diff Hash: $DIFF_HASH"
fi

# ============================================================
# 4. Check for code symbols (新增/修改的 func/def/class)
# ============================================================
if echo "$REVIEW_CONTENT" | grep -qE '(程式碼符號|Code Symbols|Symbols):'; then
  echo "[verify_review] ✓ Code symbols section present"
else
  WARNINGS+=("Missing code symbols section")
fi

# ============================================================
# 5. Check for design document references
# ============================================================
if echo "$REVIEW_CONTENT" | grep -qE '(設計引用|Design References|References):'; then
  echo "[verify_review] ✓ Design references section present"
else
  WARNINGS+=("Missing design references section")
fi

# ============================================================
# 6. Extract and verify score (1-10)
# ============================================================
SCORE=$(echo "$REVIEW_CONTENT" | grep -oP '(?<=評分|Score)[:\s]*([0-9]+)' | grep -oP '[0-9]+' | head -1 || echo "")

if [[ -z "$SCORE" ]]; then
  # Try alternative patterns
  SCORE=$(echo "$REVIEW_CONTENT" | grep -oP '[0-9]+/10' | grep -oP '^[0-9]+' | head -1 || echo "")
fi

if [[ -z "$SCORE" ]]; then
  ERRORS+=("Missing score")
elif [[ "$SCORE" -lt 1 ]] || [[ "$SCORE" -gt 10 ]]; then
  ERRORS+=("Invalid score: $SCORE (must be 1-10)")
else
  echo "[verify_review] Score: $SCORE/10"
fi

# ============================================================
# 7. Check for score reasoning
# ============================================================
if echo "$REVIEW_CONTENT" | grep -qE '(評分理由|Reasoning|Rationale):'; then
  echo "[verify_review] ✓ Score reasoning present"
else
  WARNINGS+=("Missing score reasoning")
fi

# ============================================================
# 8. Check for improvement suggestions
# ============================================================
if echo "$REVIEW_CONTENT" | grep -qE '(可改進之處|Improvements|Suggestions):'; then
  echo "[verify_review] ✓ Improvement suggestions present"
else
  WARNINGS+=("Missing improvement suggestions section")
fi

# ============================================================
# 9. Check for potential risks
# ============================================================
if echo "$REVIEW_CONTENT" | grep -qE '(潛在風險|Risks|Concerns):'; then
  echo "[verify_review] ✓ Potential risks section present"
else
  WARNINGS+=("Missing potential risks section")
fi

# ============================================================
# Output results
# ============================================================
echo ""

if [[ ${#WARNINGS[@]} -gt 0 ]]; then
  echo "[verify_review] Warnings:"
  for w in "${WARNINGS[@]}"; do
    echo "  ⚠ $w"
  done
fi

if [[ ${#ERRORS[@]} -gt 0 ]]; then
  echo "[verify_review] Errors:"
  for e in "${ERRORS[@]}"; do
    echo "  ✗ $e"
  done
  echo ""
  echo "[verify_review] FAILED: Review verification failed"
  exit 1
fi

# Check score threshold
if [[ -n "$SCORE" ]] && [[ "$SCORE" -lt 7 ]]; then
  echo ""
  echo "[verify_review] NEEDS REVISION: Score $SCORE < 7"
  exit 2
fi

echo ""
echo "[verify_review] PASSED: Review verification successful"
exit 0
