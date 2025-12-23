#!/usr/bin/env bash
# GitHub Comment Manager for AWK (AI Workflow Kit)
# Provides consistent AWK comment format for Issues and PRs
#
# Requirements: 4.1, 4.2, 4.3, 4.4, 5.2, 5.9

set -euo pipefail

# ============================================================
# Constants
# ============================================================
readonly AWK_COMMENT_PREFIX="<!-- AWK:session:"
readonly AWK_COMMENT_SUFFIX=" -->"

# ============================================================
# add_issue_comment()
# Add AWK tracking comment to Issue
# Args:
#   $1 = issue_id
#   $2 = session_id
#   $3 = comment_type (created | worker_start | worker_complete)
#   $4 = source (optional: tasks.md line number or audit finding ID)
#   $5 = extra_data (optional: additional table rows in markdown format)
# Req: 4.1, 4.2, 4.3, 4.4
# ============================================================
add_issue_comment() {
  local issue_id="${1:?Usage: add_issue_comment <issue_id> <session_id> <comment_type> [source] [extra_data]}"
  local session_id="${2:?}"
  local comment_type="${3:?}"
  local source="${4:-}"
  local extra_data="${5:-}"
  
  local timestamp
  timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  
  # Build comment body
  local comment="${AWK_COMMENT_PREFIX}${session_id}${AWK_COMMENT_SUFFIX}
🤖 **AWK Tracking**

| Field | Value |
|-------|-------|
| Session | \`${session_id}\` |
| Timestamp | ${timestamp} |
| Type | ${comment_type} |"

  # Add source field if provided (Req 4.1)
  if [[ -n "$source" ]]; then
    comment+="
| Source | ${source} |"
  fi
  
  # Add extra data rows if provided (Req 4.3)
  if [[ -n "$extra_data" ]]; then
    comment+="
${extra_data}"
  fi
  
  # Post comment to GitHub
  gh issue comment "$issue_id" --body "$comment"
  
  echo "[COMMENT] Added AWK comment to Issue #${issue_id}: ${comment_type}" >&2
}

# ============================================================
# add_pr_review_comment()
# Add AWK review comment to PR
# Args:
#   $1 = pr_number
#   $2 = session_id
#   $3 = ci_status
#   $4 = diff_hash
#   $5 = code_symbols (new/modified functions, classes)
#   $6 = design_refs (references to design docs)
#   $7 = score (1-10)
#   $8 = score_reason
#   $9 = improvements (optional: suggested improvements)
#   $10 = risks (optional: potential risks)
# Req: 5.2, 5.9
# ============================================================
add_pr_review_comment() {
  local pr_number="${1:?Usage: add_pr_review_comment <pr_number> <session_id> <ci_status> <diff_hash> <code_symbols> <design_refs> <score> <score_reason> [improvements] [risks]}"
  local session_id="${2:?}"
  local ci_status="${3:?}"
  local diff_hash="${4:?}"
  local code_symbols="${5:?}"
  local design_refs="${6:?}"
  local score="${7:?}"
  local score_reason="${8:?}"
  local improvements="${9:-}"
  local risks="${10:-}"
  
  local timestamp
  timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  
  # Build review comment body
  local comment="${AWK_COMMENT_PREFIX}${session_id}${AWK_COMMENT_SUFFIX}
🤖 **AWK Review**

| Field | Value |
|-------|-------|
| Reviewer Session | \`${session_id}\` |
| Review Timestamp | ${timestamp} |
| CI Status | ${ci_status} |
| Diff Hash | \`${diff_hash}\` |
| Score | ${score}/10 |

### Code Symbols (New/Modified)

${code_symbols}

### Design References

${design_refs}

### Score Reason

${score_reason}"

  # Add improvements section if provided
  if [[ -n "$improvements" ]]; then
    comment+="

### Suggested Improvements

${improvements}"
  fi
  
  # Add risks section if provided
  if [[ -n "$risks" ]]; then
    comment+="

### Potential Risks

${risks}"
  fi
  
  # Post comment to GitHub
  gh pr comment "$pr_number" --body "$comment"
  
  echo "[COMMENT] Added AWK review comment to PR #${pr_number}" >&2
}

# ============================================================
# extract_session_from_comment()
# Extract session ID from AWK comment
# Args: $1 = comment_body
# Returns: session ID or empty string
# ============================================================
extract_session_from_comment() {
  local comment_body="${1:?Usage: extract_session_from_comment <comment_body>}"
  
  # Extract session ID from AWK comment marker
  # Format: <!-- AWK:session:<role>-<YYYYMMDD>-<HHMMSS>-<hex4> -->
  # Role must be 'principal' or 'worker'
  # Hex must be lowercase
  if [[ "$comment_body" =~ ${AWK_COMMENT_PREFIX}((principal|worker)-[0-9]{8}-[0-9]{6}-[a-f0-9]{4})${AWK_COMMENT_SUFFIX} ]]; then
    local session_id="${BASH_REMATCH[1]}"
    # Double-check hex is lowercase (some bash versions are case-insensitive)
    local hex_part="${session_id##*-}"
    if [[ "$hex_part" =~ ^[a-f0-9]{4}$ ]] && [[ "$hex_part" == "${hex_part,,}" ]]; then
      echo "$session_id"
    else
      echo ""
    fi
  else
    echo ""
  fi
}

# ============================================================
# format_duration()
# Format duration in seconds to human-readable string
# Args: $1 = duration_seconds
# Returns: formatted string (e.g., "2m 30s" or "1h 5m")
# ============================================================
format_duration() {
  local seconds="${1:?Usage: format_duration <seconds>}"
  
  if [[ $seconds -lt 60 ]]; then
    echo "${seconds}s"
  elif [[ $seconds -lt 3600 ]]; then
    local mins=$((seconds / 60))
    local secs=$((seconds % 60))
    echo "${mins}m ${secs}s"
  else
    local hours=$((seconds / 3600))
    local mins=$(((seconds % 3600) / 60))
    echo "${hours}h ${mins}m"
  fi
}

# ============================================================
# build_worker_complete_extra()
# Build extra data for worker_complete comment
# Args: $1 = pr_url, $2 = duration_seconds
# Returns: formatted extra data rows
# ============================================================
build_worker_complete_extra() {
  local pr_url="${1:-}"
  local duration_seconds="${2:-}"
  
  local extra=""
  
  if [[ -n "$pr_url" ]]; then
    extra+="| PR | ${pr_url} |
"
  fi
  
  if [[ -n "$duration_seconds" ]]; then
    local formatted_duration
    formatted_duration=$(format_duration "$duration_seconds")
    extra+="| Duration | ${formatted_duration} |"
  fi
  
  echo "$extra"
}

# ============================================================
# Main: Allow sourcing or direct execution for testing
# ============================================================
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  case "${1:-}" in
    add_issue_comment)
      shift
      add_issue_comment "$@"
      ;;
    add_pr_review_comment)
      shift
      add_pr_review_comment "$@"
      ;;
    extract_session_from_comment)
      shift
      extract_session_from_comment "$@"
      ;;
    format_duration)
      shift
      format_duration "$@"
      ;;
    build_worker_complete_extra)
      shift
      build_worker_complete_extra "$@"
      ;;
    *)
      echo "Usage: $0 <command> [args...]"
      echo ""
      echo "Commands:"
      echo "  add_issue_comment <issue_id> <session_id> <type> [source] [extra]"
      echo "  add_pr_review_comment <pr> <session> <ci> <hash> <symbols> <refs> <score> <reason> [improvements] [risks]"
      echo "  extract_session_from_comment <comment_body>"
      echo "  format_duration <seconds>"
      echo "  build_worker_complete_extra <pr_url> <duration>"
      exit 1
      ;;
  esac
fi
