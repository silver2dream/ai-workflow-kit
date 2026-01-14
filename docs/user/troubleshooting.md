# Troubleshooting Guide

本文件整理 AI Workflow Kit 常見問題的解決方案。

---

## 錯誤類型總覽

| Exit Code | 錯誤類型 | 說明 |
|-----------|----------|------|
| 0 | Success | 執行成功 |
| 1 | Execution Error | 一般執行錯誤 |
| 2 | Config Error | 配置或依賴缺失 |
| 3 | Validation Error | 驗證失敗 |

---

## Config Error (Exit Code 2)

### 找不到配置檔

**症狀：**
```
Config file not found: .ai/config/workflow.yaml
```

**原因：** workflow.yaml 不存在或路徑錯誤

**解決：**
1. 確認 `.ai/config/workflow.yaml` 存在
2. 從正確的專案根目錄執行命令
3. 如果是新專案，使用 `awkit init` 初始化

---

### 缺少 Python 依賴

**症狀：**
```
Please install: pip3 install pyyaml jsonschema jinja2
```

**原因：** 必要的 Python 套件未安裝

**解決：**
```bash
pip3 install pyyaml jsonschema jinja2
```

如果權限不足：
```bash
pip3 install --user pyyaml jsonschema jinja2
```

---

### 找不到 Schema 檔

**症狀：**
```
Schema file not found: .ai/config/workflow.schema.json
```

**原因：** Schema 檔案遺失

**解決：**
```bash
# 重新生成配置
bash .ai/scripts/generate.sh
```

---

## Validation Error (Exit Code 3)

### YAML 格式錯誤

**症狀：**
```
Invalid YAML: expected <block end>, but found '<scalar>'
```

**原因：** YAML 語法錯誤，通常是縮排問題

**解決：**
1. 使用空格縮排，不要使用 Tab
2. 確認冒號後有空格：`key: value`
3. 確認列表項目對齊

**錯誤範例：**
```yaml
repos:
- name: backend    # 錯誤：- 前缺少縮排
  path: backend/
```

**正確範例：**
```yaml
repos:
  - name: backend
    path: backend/
```

---

### 缺少必要欄位

**症狀：**
```
Validation error: 'repos' is a required property
```

**原因：** workflow.yaml 缺少必要欄位

**解決：** 參考 [配置說明](configuration.md) 補齊必要欄位

---

### 無效的 repo type

**症狀：**
```
Invalid repo type: must be one of [root, directory, submodule]
```

**原因：** type 欄位值不正確

**解決：**
```yaml
repos:
  - name: backend
    type: directory    # 只能是 root, directory, 或 submodule
```

---

### Submodule 未初始化

**症狀：**
```
Warning: .gitmodules not found but repo has submodule type
```

**原因：** 配置為 submodule 但專案沒有 .gitmodules

**解決：**
1. 如果不是用 submodule，改用 `type: directory`
2. 如果確實是 submodule：
```bash
git submodule init
git submodule update
```

---

## Execution Error (Exit Code 1)

### Git 操作失敗

**症狀：**
```
error: failed to push some refs
```

**原因：** 遠端有新的變更尚未同步

**解決：**
```bash
git pull --rebase
git push
```

---

### 合併衝突

**症狀：**
```
CONFLICT (content): Merge conflict in <file>
```

**原因：** 無法自動合併變更

**解決：**
1. 手動解決衝突
2. `git add <resolved-files>`
3. `git commit`

---

### GitHub CLI 未授權

**症狀：**
```
gh: authentication required
```

**原因：** GitHub CLI 尚未登入

**解決：**
```bash
gh auth login
```

---

### Claude Code 需要手動批准命令 (Autopilot 卡住)

**症狀：**
```
GitHub CLI operations require approval in the current execution context
```

**原因：** Claude Code 的安全機制需要批准 `gh` 等命令

**解決：**

方法一：重新生成權限設定（推薦）
```bash
bash .ai/scripts/generate.sh
```
這會生成 `.claude/settings.local.json`，自動批准 `gh`、`git`、`codex` 等命令。

方法二：手動編輯 `.claude/settings.local.json`
```json
{
  "permissions": {
    "allow": [
      "Bash(gh:*)",
      "Bash(git:*)",
      "Bash(bash:*)",
      "Bash(codex:*)"
    ]
  }
}
```

方法三：使用 `--dangerously-skip-permissions` flag（不推薦）
```bash
claude --print --dangerously-skip-permissions < .ai/scripts/principal_boot.txt
```

---

### kickoff.sh 長時間無輸出

**症狀：** 執行 `kickoff.sh` 後很久沒有任何訊息

**原因：** Principal 在執行前置檢查和分析，但沒有輸出進度

**解決：**
1. 使用 `awkit kickoff` 取代 `kickoff.sh`（提供 PTY 即時輸出和 Spinner 動畫）
2. 或升級到最新版本：`awkit upgrade && bash .ai/scripts/generate.sh`

---

## 語言特定錯誤

### Go

#### 編譯錯誤

**症狀：**
```
cannot find package "xxx"
```

**解決：**
```bash
go mod tidy
```

#### 測試失敗

**症狀：**
```
--- FAIL: TestXxx
```

**解決：** 檢查測試程式碼，修正失敗的測試案例

---

### Node.js

#### Module Not Found

**症狀：**
```
Cannot find module 'xxx'
```

**解決：**
```bash
npm install
# 或
pnpm install
```

#### npm ERR!

**症狀：**
```
npm ERR! code ERESOLVE
```

**解決：**
```bash
rm -rf node_modules package-lock.json
npm install
```

---

### Python

#### Import Error

**症狀：**
```
ModuleNotFoundError: No module named 'xxx'
```

**解決：**
```bash
pip3 install xxx
```

#### pytest 失敗

**症狀：**
```
FAILED tests/test_xxx.py::test_function
```

**解決：** 檢查測試程式碼和實作

---

## 平台特定問題

### Windows

#### 終端機顯示中文亂碼

**症狀：** 執行 `awkit init` 或查看 workflow.yaml 時，中文註解顯示為亂碼

**原因：** Windows PowerShell 預設使用系統編碼（Big5/GBK），而非 UTF-8

**解決：**

方法一：設定 PowerShell 使用 UTF-8
```powershell
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
chcp 65001
```

方法二：使用 Windows Terminal（推薦）
- Windows Terminal 預設支援 UTF-8
- 從 Microsoft Store 安裝

方法三：使用 Git Bash
- Git Bash 預設支援 UTF-8

**注意：** 這只是顯示問題，實際檔案內容是正確的 UTF-8 編碼。

---

#### 路徑過長

**症狀：**
```
fatal: cannot create directory at 'xxx': Filename too long
```

**解決：**
```bash
git config --system core.longpaths true
```

#### 行尾符號問題

**症狀：** Shell 腳本無法執行

**解決：**
```bash
git config --global core.autocrlf input
```

#### Bash 不可用

**症狀：**
```
'bash' is not recognized as an internal or external command
```

**解決：**
1. 安裝 Git for Windows (包含 Git Bash)
2. 或使用 WSL

---

### macOS

#### Bash 版本過舊

**症狀：** 腳本功能異常

**解決：**
```bash
brew install bash
```

#### 權限問題

**症狀：**
```
Permission denied
```

**解決：**
```bash
chmod +x .ai/scripts/*.sh
```

---

### Linux

#### 缺少 Python

**症狀：**
```
python3: command not found
```

**解決：**
```bash
# Ubuntu/Debian
sudo apt install python3 python3-pip

# CentOS/RHEL
sudo yum install python3 python3-pip
```

---

## 網路相關錯誤

### 連線逾時

**症狀：**
```
ETIMEDOUT
```

**解決：**
1. 檢查網路連線
2. 如果使用 Proxy，設定環境變數：
```bash
export HTTP_PROXY=http://proxy:port
export HTTPS_PROXY=http://proxy:port
```

---

### API 限流

**症狀：**
```
API rate limit exceeded
```

**解決：**
1. 等待限流解除
2. 使用 Personal Access Token 提高限額

---

## 診斷工具

### 驗證配置

```bash
python3 .ai/scripts/validate_config.py
```

### 掃描專案狀態

```bash
python3 .ai/scripts/scan_repo.py --json
```

### 執行審計

```bash
python3 .ai/scripts/audit_project.py --json
```

### 查看執行追蹤

```bash
python3 .ai/scripts/query_traces.py --status failed
```

---

## 取得幫助

如果上述方案無法解決問題：

1. 查看 [FAQ](faq.md)
2. 檢查 [GitHub Issues](https://github.com/silver2dream/ai-workflow-kit/issues)
3. 提交新的 Issue，附上：
   - 錯誤訊息完整內容
   - 作業系統與版本
   - Python 版本 (`python3 --version`)
   - 重現步驟
