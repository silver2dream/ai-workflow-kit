# Frequently Asked Questions

常見問題解答。

---

## 安裝與設定

### Q: AWK 支援哪些作業系統？

**A:** 支援 macOS、Linux、Windows (透過 Git Bash 或 WSL)。

---

### Q: 可以不使用 awkit CLI 安裝嗎？

**A:** 可以。手動複製 `.ai/` 目錄到專案，然後執行：
```bash
awkit generate
```

---

### Q: 如何更新 AWK 到最新版本？

**A:**
```bash
# 檢查 CLI 是否有更新
awkit check-update

# 更新 CLI (重新安裝)
# Linux/macOS
curl -fsSL https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.sh | bash

# Windows PowerShell
irm https://github.com/silver2dream/ai-workflow-kit/releases/latest/download/install.ps1 | iex

# 升級專案內的 kit 檔案（保留你的 workflow.yaml）
awkit upgrade

# 重新生成輔助檔案
awkit generate
```

---

### Q: workflow.yaml 可以放在其他位置嗎？

**A:** 目前不支援。必須放在 `.ai/config/workflow.yaml`。

---

## 工作流程

### Q: awkit kickoff 和 awkit generate 有什麼差別？

**A:**
- `awkit generate` - 根據 workflow.yaml 生成設定檔 (CLAUDE.md, AGENTS.md 等)
- `awkit kickoff` - 啟動完整的 AI 工作流程 (審計、建立 Issue、執行任務)
- `kickoff.sh` - legacy bash 版本，建議使用 `awkit kickoff` 取代

---

### Q: 如何暫停工作流程？

**A:** 建立 STOP 檔案：
```bash
touch .ai/state/STOP
```

刪除此檔案後可繼續執行。

---

### Q: --dry-run 模式會做什麼？

**A:** 顯示將執行的操作但不實際執行，用於預覽和測試。

---

### Q: 什麼是 Spec？

**A:** Spec 是任務規格檔，定義要實作的功能。結構如下：
```
.ai/specs/<feature-name>/
├── requirements.md    # 需求文檔 (可選)
├── design.md          # 設計文檔 (可選)
└── tasks.md           # 任務清單 (必要)
```

---

### Q: 如何啟用 Spec？

**A:** 在 workflow.yaml 中加入：
```yaml
specs:
  active:
    - my-feature
```

---

### Q: tasks.md 的格式是什麼？

**A:**
```markdown
# Feature Name

Repo: backend
Coordination: sequential

## Tasks

- [ ] 1. First task
  - [ ] 1.1 Subtask
  - _depends_on: -_

- [ ] 2. Second task
  - _depends_on: 1_
```

---

### Q: 什麼是 Principal 和 Worker？

**A:**
- **Principal (Claude Code)** - 讀取 spec、建立 Issue、審查 PR、決策
- **Worker (Codex)** - 實作程式碼、提交 PR

---

## 整合與擴展

### Q: 可以不使用 GitHub 嗎？

**A:** 目前 AWK 主要設計用於 GitHub。對 GitLab/Bitbucket 的支援需要額外開發。

---

### Q: 如何自訂規則？

**A:**
1. 建立規則檔案：`.ai/rules/my-rule.md`
2. 在 workflow.yaml 中啟用：
```yaml
rules:
  custom:
    - my-rule
```

---

### Q: 可以整合 CI/CD 嗎？

**A:** 可以。AWK 提供 `evaluate.sh` 腳本用於 CI：
```yaml
# .github/workflows/ci.yml
- name: AWK Evaluation
  run: bash .ai/scripts/evaluate.sh --offline --strict
```

---

### Q: 如何加入新的語言支援？

**A:**
1. 在 `.ai/templates/` 建立 CI 模板 (如 `ci-kotlin.yml.j2`)
2. 在 workflow.yaml 中使用新語言
3. (可選) 在 `failure_patterns.json` 加入錯誤模式

---

### Q: 支援哪些 Webhook 通知？

**A:** Slack 和 Discord webhook 通知已規劃但**尚未實作**。配置 schema 中保留了相關欄位，將在未來版本中實作：
```yaml
# notifications: (planned for future release)
# slack_webhook: "${AI_SLACK_WEBHOOK}"
# discord_webhook: "${AI_DISCORD_WEBHOOK}"
```

---

## 錯誤處理

### Q: 如何查看執行日誌？

**A:** 日誌存放在 `.ai/logs/` 目錄，使用 `--log-level debug` 可獲得更詳細的輸出。

---

### Q: 如何查詢失敗的執行記錄？

**A:** 查看日誌目錄 `.ai/exe-logs/` 中的 Worker 日誌：
```bash
# 查看 Principal 日誌
cat .ai/exe-logs/principal.log

# 查看特定 Issue 的 Worker 日誌
cat .ai/exe-logs/issue-<N>.worker.log
```

---

### Q: 連續失敗後工作流程會停止嗎？

**A:** 會。預設連續失敗 3 次後暫停，可在 workflow.yaml 調整：
```yaml
escalation:
  max_consecutive_failures: 5
```

---

### Q: 如何啟用自動重試？

**A:** 在 workflow.yaml 設定：
```yaml
escalation:
  retry_count: 2
  retry_delay_seconds: 5
```

---

## 安全性

### Q: Token 應該怎麼管理？

**A:** 不要在 workflow.yaml 中硬編碼 token。使用環境變數：
```bash
export GH_TOKEN=ghp_xxxx
```

或使用 GitHub CLI 登入：
```bash
gh auth login
```

---

### Q: 敏感操作會被攔截嗎？

**A:** 會。預設攔截包含 `security`、`delete`、`migration` 等關鍵字的操作：
```yaml
escalation:
  triggers:
    - pattern: "security|vulnerability"
      action: "require_human_approval"
```

---

### Q: PR 太大會被拒絕嗎？

**A:** 會觸發人工審查。預設限制：
```yaml
escalation:
  max_single_pr_files: 50
  max_single_pr_lines: 500
```

---

## 其他

### Q: AWK 是開源的嗎？

**A:** 是的。AWK 採用 [Apache License 2.0](../../LICENSE) 開源授權。

---

### Q: 如何回報問題？

**A:** 在 [GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues) 提交，附上：
- 錯誤訊息
- 作業系統
- 重現步驟

---

### Q: 有中文文件嗎？

**A:** 有。README-zh-TW.md 提供繁體中文版本。

---

## 更多資源

- [快速開始](getting-started.md)
- [配置說明](configuration.md)
- [故障排除](troubleshooting.md)
