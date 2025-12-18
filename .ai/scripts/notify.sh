#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# notify.sh - 發送通知
# ============================================================================
# 用法:
#   bash scripts/ai/notify.sh "標題" "內容"
#   bash scripts/ai/notify.sh --summary    # 發送統計摘要
# ============================================================================

MONO_ROOT="$(git rev-parse --show-toplevel)"
cd "$MONO_ROOT"

# ----------------------------------------------------------------------------
# 配置 (可以通過環境變數覆蓋)
# ----------------------------------------------------------------------------
# Slack Webhook URL (可選)
SLACK_WEBHOOK_URL="${AI_SLACK_WEBHOOK:-}"

# Discord Webhook URL (可選)
DISCORD_WEBHOOK_URL="${AI_DISCORD_WEBHOOK:-}"

# 是否使用系統通知
USE_SYSTEM_NOTIFY="${AI_SYSTEM_NOTIFY:-true}"

# ----------------------------------------------------------------------------
# 解析參數
# ----------------------------------------------------------------------------
TITLE=""
MESSAGE=""
SEND_SUMMARY=false

if [[ "${1:-}" == "--summary" ]]; then
  SEND_SUMMARY=true
elif [[ $# -ge 2 ]]; then
  TITLE="$1"
  MESSAGE="$2"
elif [[ $# -eq 1 ]]; then
  TITLE="AI Workflow"
  MESSAGE="$1"
else
  echo "用法: bash scripts/ai/notify.sh \"標題\" \"內容\""
  echo "      bash scripts/ai/notify.sh --summary"
  exit 1
fi

# ----------------------------------------------------------------------------
# 生成摘要
# ----------------------------------------------------------------------------
if [[ "$SEND_SUMMARY" == "true" ]]; then
  # 收集統計數據
  ISSUES_CLOSED=$(gh issue list --label ai-task --state closed --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  ISSUES_OPEN=$(gh issue list --label ai-task --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  ISSUES_FAILED=$(gh issue list --label worker-failed --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  PRS_MERGED=$(gh pr list --state merged --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  PRS_OPEN=$(gh pr list --state open --json number --limit 500 2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  
  TITLE="🤖 AI Workflow 執行報告"
  MESSAGE="✅ 完成: $ISSUES_CLOSED | ⏳ 待處理: $ISSUES_OPEN | ❌ 失敗: $ISSUES_FAILED | 🔀 PR合併: $PRS_MERGED | 📝 PR待審: $PRS_OPEN"
fi

# ----------------------------------------------------------------------------
# 發送系統通知
# ----------------------------------------------------------------------------
send_system_notify() {
  local title="$1"
  local message="$2"
  
  # macOS
  if command -v osascript &>/dev/null; then
    osascript -e "display notification \"$message\" with title \"$title\"" 2>/dev/null || true
    return 0
  fi
  
  # Linux (notify-send)
  if command -v notify-send &>/dev/null; then
    notify-send "$title" "$message" 2>/dev/null || true
    return 0
  fi
  
  # Windows (PowerShell)
  if command -v powershell.exe &>/dev/null; then
    powershell.exe -Command "
      [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
      [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
      \$template = '<toast><visual><binding template=\"ToastText02\"><text id=\"1\">$title</text><text id=\"2\">$message</text></binding></visual></toast>'
      \$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
      \$xml.LoadXml(\$template)
      \$toast = [Windows.UI.Notifications.ToastNotification]::new(\$xml)
      [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('AI Workflow').Show(\$toast)
    " 2>/dev/null || true
    return 0
  fi
  
  # terminal-notifier (macOS 備選)
  if command -v terminal-notifier &>/dev/null; then
    terminal-notifier -title "$title" -message "$message" 2>/dev/null || true
    return 0
  fi
  
  echo "[notify] 無可用的系統通知工具"
  return 1
}

# ----------------------------------------------------------------------------
# 發送 Slack 通知
# ----------------------------------------------------------------------------
send_slack_notify() {
  local title="$1"
  local message="$2"
  
  if [[ -z "$SLACK_WEBHOOK_URL" ]]; then
    return 1
  fi
  
  curl -s -X POST "$SLACK_WEBHOOK_URL" \
    -H 'Content-type: application/json' \
    -d "{
      \"text\": \"*$title*\n$message\"
    }" >/dev/null 2>&1 || true
  
  echo "[notify] Slack 通知已發送"
}

# ----------------------------------------------------------------------------
# 發送 Discord 通知
# ----------------------------------------------------------------------------
send_discord_notify() {
  local title="$1"
  local message="$2"
  
  if [[ -z "$DISCORD_WEBHOOK_URL" ]]; then
    return 1
  fi
  
  curl -s -X POST "$DISCORD_WEBHOOK_URL" \
    -H 'Content-type: application/json' \
    -d "{
      \"content\": \"**$title**\n$message\"
    }" >/dev/null 2>&1 || true
  
  echo "[notify] Discord 通知已發送"
}

# ----------------------------------------------------------------------------
# 發送通知
# ----------------------------------------------------------------------------
SENT=false

# 系統通知
if [[ "$USE_SYSTEM_NOTIFY" == "true" ]]; then
  if send_system_notify "$TITLE" "$MESSAGE"; then
    SENT=true
  fi
fi

# Slack
if [[ -n "$SLACK_WEBHOOK_URL" ]]; then
  send_slack_notify "$TITLE" "$MESSAGE"
  SENT=true
fi

# Discord
if [[ -n "$DISCORD_WEBHOOK_URL" ]]; then
  send_discord_notify "$TITLE" "$MESSAGE"
  SENT=true
fi

# 如果沒有發送任何通知，至少輸出到終端
if [[ "$SENT" == "false" ]]; then
  echo ""
  echo "═══════════════════════════════════════════"
  echo "  $TITLE"
  echo "═══════════════════════════════════════════"
  echo "  $MESSAGE"
  echo "═══════════════════════════════════════════"
  echo ""
fi
