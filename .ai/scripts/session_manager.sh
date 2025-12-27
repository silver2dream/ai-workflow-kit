#!/usr/bin/env bash
# Session Manager for AWK (AI Workflow Kit)
# Manages Principal and Worker session lifecycle
#
# Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 6.3, 6.4

set -euo pipefail

# ============================================================
# Cross-platform jq wrapper (handles Windows .exe)
# ============================================================
_jq() {
  if command -v jq &>/dev/null; then
    jq "$@"
  elif command -v jq.exe &>/dev/null; then
    jq.exe "$@"
  elif [[ -x "/mnt/c/Users/user/bin/jq.exe" ]]; then
    /mnt/c/Users/user/bin/jq.exe "$@"
  else
    echo "[ERROR] jq not found. Please install jq." >&2
    return 1
  fi
}

# ============================================================
# Constants
# ============================================================
readonly SESSION_STATE_DIR=".ai/state/principal"
readonly SESSION_FILE="$SESSION_STATE_DIR/session.json"
readonly SESSIONS_DIR="$SESSION_STATE_DIR/sessions"
readonly RESULTS_DIR=".ai/results"

# ============================================================
# generate_session_id()
# Generate unique session ID in format: <role>-<YYYYMMDD>-<HHMMSS>-<random_hex_4>
# Args: $1 = role (principal or worker)
# Returns: session ID string
# Req: 1.1, 2.1
# ============================================================
generate_session_id() {
  local role="${1:?Usage: generate_session_id <role>}"
  local timestamp
  local random_hex
  
  timestamp=$(date -u +%Y%m%d-%H%M%S)
  random_hex=$(openssl rand -hex 2)
  
  echo "${role}-${timestamp}-${random_hex}"
}

# ============================================================
# check_principal_running()
# Check if another Principal is already running (PID + start time verification)
# Args: $1 = pid, $2 = expected_start_time (epoch seconds)
# Returns: 0 if running, 1 if not running or PID reused
# Req: Risk Mitigation (PID Reuse Protection)
# ============================================================
check_principal_running() {
  local pid="${1:?Usage: check_principal_running <pid> <expected_start_time>}"
  local expected_start="${2:?}"
  
  # Check if PID exists
  if ! kill -0 "$pid" 2>/dev/null; then
    return 1  # PID does not exist
  fi
  
  # Get actual process start time
  local actual_start
  if [[ -f "/proc/$pid/stat" ]]; then
    # Linux: get start time from /proc
    actual_start=$(stat -c %Y "/proc/$pid" 2>/dev/null || echo "0")
  elif command -v ps &>/dev/null; then
    # macOS/BSD: use ps to get start time (convert to epoch)
    local lstart
    lstart=$(ps -p "$pid" -o lstart= 2>/dev/null || echo "")
    if [[ -n "$lstart" ]]; then
      actual_start=$(date -j -f "%a %b %d %T %Y" "$lstart" +%s 2>/dev/null || echo "0")
    else
      actual_start="0"
    fi
  else
    actual_start="0"
  fi
  
  # Compare start times (allow 2 second tolerance)
  local diff=$((actual_start - expected_start))
  if [[ ${diff#-} -le 2 ]]; then
    return 0  # Same process, still running
  fi
  
  return 1  # PID was reused by different process
}


# ============================================================
# init_principal_session()
# Initialize a new Principal session
# - Check for existing session (PID reuse protection)
# - Mark old session as interrupted if needed
# - Create new session files
# Returns: session ID string, or exits with error if another Principal is running
# Req: 1.1, 1.2, 1.3
# ============================================================
init_principal_session() {
  mkdir -p "$SESSIONS_DIR"
  
  local current_pid=$$
  local current_start
  current_start=$(date +%s)
  
  # Check if session.json exists (another Principal might be running)
  if [[ -f "$SESSION_FILE" ]]; then
    local prev_pid prev_start prev_session_id
    prev_pid=$(_jq -r '.pid // 0' "$SESSION_FILE" 2>/dev/null || echo "0")
    prev_start=$(_jq -r '.pid_start_time // 0' "$SESSION_FILE" 2>/dev/null || echo "0")
    prev_session_id=$(_jq -r '.session_id // ""' "$SESSION_FILE" 2>/dev/null || echo "")
    
    if [[ "$prev_pid" != "0" ]] && check_principal_running "$prev_pid" "$prev_start"; then
      echo "[ERROR] Another Principal is already running (PID: $prev_pid, Session: $prev_session_id)" >&2
      echo "[ERROR] Please stop the existing Principal before starting a new one." >&2
      exit 1
    fi
    
    # Old Principal is dead, mark as interrupted (only if not already ended)
    if [[ -n "$prev_session_id" ]] && [[ -f "$SESSIONS_DIR/${prev_session_id}.json" ]]; then
      prev_ended_at=$(_jq -r '.ended_at // ""' "$SESSIONS_DIR/${prev_session_id}.json" 2>/dev/null || echo "")
      if [[ -z "$prev_ended_at" || "$prev_ended_at" == "null" ]]; then
        echo "[SESSION] Marking previous session $prev_session_id as interrupted" >&2
        end_principal_session "$prev_session_id" "interrupted"
      fi
    fi
  fi
  
  # Generate new session
  local session_id
  session_id=$(generate_session_id "principal")
  local started_at
  started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  
  # Write current session file
  cat > "$SESSION_FILE" << EOF
{
  "session_id": "$session_id",
  "started_at": "$started_at",
  "pid": $current_pid,
  "pid_start_time": $current_start
}
EOF
  
  # Initialize session log
  cat > "$SESSIONS_DIR/${session_id}.json" << EOF
{
  "session_id": "$session_id",
  "started_at": "$started_at",
  "actions": []
}
EOF
  
  echo "[SESSION] Initialized Principal session: $session_id" >&2
  echo "$session_id"
}

# ============================================================
# get_current_session_id()
# Get the current Principal session ID from session.json
# Returns: session ID string or empty if not found
# ============================================================
get_current_session_id() {
  if [[ -f "$SESSION_FILE" ]]; then
    _jq -r '.session_id // ""' "$SESSION_FILE" 2>/dev/null || echo ""
  else
    echo ""
  fi
}


# ============================================================
# append_session_action()
# Append an action to the Principal session log
# Args: $1 = session_id, $2 = action_type, $3 = action_data (JSON object)
# Req: 1.4
# ============================================================
append_session_action() {
  local session_id="${1:?Usage: append_session_action <session_id> <action_type> <action_data>}"
  local action_type="${2:?}"
  local action_data="${3:-"{}"}"
  local log_file="$SESSIONS_DIR/${session_id}.json"
  
  if [[ ! -f "$log_file" ]]; then
    echo "[WARNING] Session log not found: $log_file" >&2
    return 1
  fi
  
  local timestamp
  timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)

  # Write action_data to temp file to avoid Windows quoting issues.
  # We'll parse it with jq's fromjson, and fall back to recording raw text if invalid.
  local data_tmp="${log_file}.data.tmp"
  printf '%s' "$action_data" > "$data_tmp"

  if _jq --arg type "$action_type" \
        --arg timestamp "$timestamp" \
        --rawfile rawdata "$data_tmp" \
        '
          .actions = (if (.actions | type) == "array" then .actions else [] end) |
          ($rawdata | rtrimstr("\n") | rtrimstr("\r")) as $raw |
          .actions += [{
            "type": $type,
            "timestamp": $timestamp,
            "data": (try ($raw | fromjson) catch {"_raw": $raw, "_parse_error": "invalid_json"})
          }]
        ' \
        "$log_file" > "${log_file}.tmp"; then
    mv "${log_file}.tmp" "$log_file"
    rm -f "$data_tmp"
    echo "[SESSION] Recorded action: $action_type" >&2
    return 0
  fi

  rm -f "${log_file}.tmp" 2>/dev/null || true
  echo "[ERROR] Failed to append action to session log: $log_file" >&2
  echo "[ERROR] Action data saved at: $data_tmp" >&2
  return 1
}

# ============================================================
# update_worker_completion()
# Record worker completion in session log (convenience wrapper)
# Args: $1 = session_id, $2 = issue_id, $3 = worker_session_id, $4 = status, $5 = pr_url
# Req: 1.5
# ============================================================
update_worker_completion() {
  local session_id="${1:?Usage: update_worker_completion <session_id> <issue_id> <worker_session_id> <status> [pr_url]}"
  local issue_id="${2:?}"
  local worker_session_id="${3:?}"
  local status="${4:?}"
  local pr_url="${5:-}"
  
  local action_data
  action_data=$(cat <<EOF
{
  "issue_id": "$issue_id",
  "worker_session_id": "$worker_session_id",
  "status": "$status",
  "pr_url": "$pr_url"
}
EOF
)
  
  append_session_action "$session_id" "worker_completed" "$action_data"
}

# ============================================================
# end_principal_session()
# End a Principal session with timestamp and exit reason
# Args: $1 = session_id, $2 = exit_reason
# Req: 1.6
# ============================================================
end_principal_session() {
  local session_id="${1:?Usage: end_principal_session <session_id> <exit_reason>}"
  local exit_reason="${2:?}"
  local log_file="$SESSIONS_DIR/${session_id}.json"
  
  if [[ ! -f "$log_file" ]]; then
    echo "[WARNING] Session log not found: $log_file" >&2
    return 1
  fi
  
  local ended_at
  ended_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  
  _jq --arg ended "$ended_at" \
     --arg reason "$exit_reason" \
     '. + {"ended_at": $ended, "exit_reason": $reason}' \
     "$log_file" > "${log_file}.tmp" && mv "${log_file}.tmp" "$log_file"
  
  echo "[SESSION] Ended session $session_id with reason: $exit_reason" >&2
}


# ============================================================
# update_result_with_principal_session()
# Update result JSON with Principal session ID
# Args: $1 = issue_id, $2 = principal_session_id
# Req: 6.3
# ============================================================
update_result_with_principal_session() {
  local issue_id="${1:?Usage: update_result_with_principal_session <issue_id> <principal_session_id>}"
  local principal_session_id="${2:?}"
  local result_file="$RESULTS_DIR/issue-${issue_id}.json"
  
  if [[ ! -f "$result_file" ]]; then
    echo "[WARNING] Result file not found: $result_file" >&2
    return 1
  fi
  
  # Update principal_session_id in session section
  _jq --arg psid "$principal_session_id" \
     '.session.principal_session_id = $psid' \
     "$result_file" > "${result_file}.tmp" && mv "${result_file}.tmp" "$result_file"
  
  echo "[SESSION] Updated result.json with principal_session_id: $principal_session_id" >&2
}

# ============================================================
# update_result_with_review_audit()
# Update result JSON with review audit information
# Args: $1 = issue_id, $2 = reviewer_session_id, $3 = decision, 
#       $4 = ci_status, $5 = ci_timeout (true/false), $6 = merge_timestamp
# Req: 6.4
# ============================================================
update_result_with_review_audit() {
  local issue_id="${1:?Usage: update_result_with_review_audit <issue_id> <reviewer_session_id> <decision> <ci_status> <ci_timeout> [merge_timestamp]}"
  local reviewer_session_id="${2:?}"
  local decision="${3:?}"
  local ci_status="${4:?}"
  local ci_timeout="${5:?}"
  local merge_timestamp="${6:-}"
  local result_file="$RESULTS_DIR/issue-${issue_id}.json"
  
  if [[ ! -f "$result_file" ]]; then
    echo "[WARNING] Result file not found: $result_file" >&2
    return 1
  fi
  
  local review_timestamp
  review_timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  
  # Convert ci_timeout to boolean
  local timeout_bool="false"
  if [[ "$ci_timeout" == "true" ]]; then
    timeout_bool="true"
  fi
  
  # Update review_audit section
  _jq --arg rsid "$reviewer_session_id" \
     --arg ts "$review_timestamp" \
     --arg dec "$decision" \
     --arg ci "$ci_status" \
     --argjson timeout "$timeout_bool" \
     --arg merge "$merge_timestamp" \
     '.review_audit = {
       "reviewer_session_id": $rsid,
       "review_timestamp": $ts,
       "ci_status": $ci,
       "ci_timeout": $timeout,
       "decision": $dec,
       "merge_timestamp": $merge
     }' \
     "$result_file" > "${result_file}.tmp" && mv "${result_file}.tmp" "$result_file"
  
  echo "[SESSION] Updated result.json with review_audit" >&2
}

# ============================================================
# Main: Allow sourcing or direct execution for testing
# ============================================================
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  # Direct execution - show usage or run command
  case "${1:-}" in
    generate_session_id)
      shift
      generate_session_id "$@"
      ;;
    init_principal_session)
      init_principal_session
      ;;
    get_current_session_id)
      get_current_session_id
      ;;
    append_session_action)
      shift
      append_session_action "$@"
      ;;
    update_worker_completion)
      shift
      update_worker_completion "$@"
      ;;
    end_principal_session)
      shift
      end_principal_session "$@"
      ;;
    update_result_with_principal_session)
      shift
      update_result_with_principal_session "$@"
      ;;
    update_result_with_review_audit)
      shift
      update_result_with_review_audit "$@"
      ;;
    check_principal_running)
      shift
      check_principal_running "$@"
      ;;
    *)
      echo "Usage: $0 <command> [args...]"
      echo ""
      echo "Commands:"
      echo "  generate_session_id <role>           Generate unique session ID"
      echo "  init_principal_session               Initialize Principal session"
      echo "  get_current_session_id               Get current session ID"
      echo "  append_session_action <sid> <type> <data>  Append action to session log"
      echo "  update_worker_completion <sid> <issue> <worker_sid> <status> [pr_url]"
      echo "  end_principal_session <sid> <reason> End session with reason"
      echo "  update_result_with_principal_session <issue> <psid>"
      echo "  update_result_with_review_audit <issue> <rsid> <decision> <ci> <timeout> [merge_ts]"
      echo "  check_principal_running <pid> <start_time>"
      exit 1
      ;;
  esac
fi
