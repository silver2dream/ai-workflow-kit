#!/usr/bin/env bash
# Session Manager for AWK (AI Workflow Kit)
# Manages Principal and Worker session lifecycle
#
# Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 6.3, 6.4

set -euo pipefail

# ============================================================
# Cross-platform python wrapper (python3 preferred)
# ============================================================
_py() {
  if command -v python3 >/dev/null 2>&1; then
    python3 "$@"
    return $?
  fi
  if command -v python >/dev/null 2>&1; then
    python "$@"
    return $?
  fi

  echo "[ERROR] python not found. Please install python3." >&2
  return 127
}

# ============================================================
# resolve_state_root()
# Return the main worktree root even when running inside a git worktree.
# - Honors AI_STATE_ROOT if provided.
# - Uses git common dir when available: <common_dir>/.. is the main worktree root.
# ============================================================
resolve_state_root() {
  if [[ -n "${AI_STATE_ROOT:-}" ]]; then
    printf '%s\n' "${AI_STATE_ROOT}"
    return 0
  fi

  if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    local common_dir=""
    common_dir="$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"
    if [[ -z "$common_dir" ]]; then
      common_dir="$(git rev-parse --git-common-dir 2>/dev/null || true)"
      if [[ -n "$common_dir" ]]; then
        common_dir="$(cd "$common_dir" 2>/dev/null && pwd -P || true)"
      fi
    fi

    if [[ -n "$common_dir" ]]; then
      printf '%s\n' "$(dirname "$common_dir")"
      return 0
    fi

    git rev-parse --show-toplevel 2>/dev/null || pwd -P 2>/dev/null || pwd
    return 0
  fi

  pwd -P 2>/dev/null || pwd
}

# ============================================================
# Constants
# ============================================================
readonly STATE_ROOT="$(resolve_state_root)"
readonly SESSION_STATE_DIR="$STATE_ROOT/.ai/state/principal"
readonly SESSION_FILE="$SESSION_STATE_DIR/session.json"
readonly SESSIONS_DIR="$SESSION_STATE_DIR/sessions"
readonly RESULTS_DIR="$STATE_ROOT/.ai/results"

_json_get_or_default() {
  local path="${1:?}"
  local key="${2:?}"
  local default="${3:-}"

  _py - "$path" "$key" "$default" <<'PY'
import json
import sys

path, key, default = sys.argv[1:4]
try:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    value = data.get(key, default)
    if value is None:
        value = ""
    sys.stdout.write(str(value))
except Exception:
    sys.stdout.write(str(default))
PY
}

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
    prev_pid="$(_json_get_or_default "$SESSION_FILE" "pid" "0")"
    prev_start="$(_json_get_or_default "$SESSION_FILE" "pid_start_time" "0")"
    prev_session_id="$(_json_get_or_default "$SESSION_FILE" "session_id" "")"
    
    if [[ "$prev_pid" != "0" ]] && check_principal_running "$prev_pid" "$prev_start"; then
      echo "[ERROR] Another Principal is already running (PID: $prev_pid, Session: $prev_session_id)" >&2
      echo "[ERROR] Please stop the existing Principal before starting a new one." >&2
      exit 1
    fi
    
    # Old Principal is dead, mark as interrupted (only if not already ended)
    if [[ -n "$prev_session_id" ]] && [[ -f "$SESSIONS_DIR/${prev_session_id}.json" ]]; then
      prev_ended_at="$(_json_get_or_default "$SESSIONS_DIR/${prev_session_id}.json" "ended_at" "")"
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
  if [[ ! -f "$SESSION_FILE" ]]; then
    echo ""
    return 1
  fi

  local session_id=""
  if ! session_id="$(_py - "$SESSION_FILE" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)
value = data.get("session_id") or ""
sys.stdout.write(str(value))
PY
)"; then
    echo ""
    return 1
  fi

  if [[ -z "$session_id" ]]; then
    echo ""
    return 1
  fi

  echo "$session_id"
  return 0
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

  # Write action_data to temp file to avoid shell quoting issues.
  local data_tmp="${log_file}.data.tmp"
  printf '%s' "$action_data" > "$data_tmp"

  if _py - "$log_file" "$action_type" "$timestamp" "$data_tmp" <<'PY'; then
import json
import os
import sys

log_file, action_type, timestamp, data_tmp = sys.argv[1:5]

with open(data_tmp, "r", encoding="utf-8") as handle:
    raw = handle.read().rstrip("\n").rstrip("\r")

try:
    with open(log_file, "r", encoding="utf-8") as handle:
        data = json.load(handle)
except Exception:
    data = {}

actions = data.get("actions")
if not isinstance(actions, list):
    actions = []

try:
    parsed = json.loads(raw) if raw else {}
except Exception:
    parsed = {"_raw": raw, "_parse_error": "invalid_json"}

actions.append({"type": action_type, "timestamp": timestamp, "data": parsed})
data["actions"] = actions

tmp_path = log_file + ".tmp"
with open(tmp_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
os.replace(tmp_path, log_file)
PY
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
  
  _py - "$log_file" "$ended_at" "$exit_reason" <<'PY'
import json
import os
import sys

path, ended_at, reason = sys.argv[1:4]

try:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
except Exception:
    data = {}

data["ended_at"] = ended_at
data["exit_reason"] = reason

tmp_path = path + ".tmp"
with open(tmp_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
os.replace(tmp_path, path)
PY
  
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
  
  _py - "$result_file" "$principal_session_id" <<'PY'
import json
import os
import sys

path, psid = sys.argv[1:3]

with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)

session = data.get("session")
if not isinstance(session, dict):
    session = {}
session["principal_session_id"] = psid
data["session"] = session

tmp_path = path + ".tmp"
with open(tmp_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
os.replace(tmp_path, path)
PY
  
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
  
  _py - "$result_file" "$reviewer_session_id" "$review_timestamp" "$decision" "$ci_status" "$timeout_bool" "$merge_timestamp" <<'PY'
import json
import os
import sys

path, rsid, ts, decision, ci_status, timeout_str, merge_ts = sys.argv[1:8]
ci_timeout = (timeout_str == "true")

with open(path, "r", encoding="utf-8") as handle:
    data = json.load(handle)

data["review_audit"] = {
    "reviewer_session_id": rsid,
    "review_timestamp": ts,
    "ci_status": ci_status,
    "ci_timeout": ci_timeout,
    "decision": decision,
    "merge_timestamp": merge_ts,
}

tmp_path = path + ".tmp"
with open(tmp_path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2, ensure_ascii=True)
    handle.write("\n")
os.replace(tmp_path, path)
PY
  
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
