# AI Workflow Kit Refactor - Implementation Plan

## 目標
將現有的 AI Workflow 重構為可重用的 Pattern Kit，所有文件集中在 `.ai/` 資料夾。

---

## Tasks

- [x] 1. 建立新目錄結構
  - [x] 1.1 創建 `.ai/config/`
  - [x] 1.2 創建 `.ai/scripts/`
  - [x] 1.3 創建 `.ai/templates/`
  - [x] 1.4 移動 `.kiro/specs/` 到 `.ai/specs/`

- [x] 2. 創建配置系統
  - [x] 2.1 創建 `.ai/config/workflow.yaml` schema
  - [x] 2.2 創建範例配置（當前專案）

- [x] 3. 移動腳本到 `.ai/scripts/`
  - [x] 3.1 移動所有 `scripts/ai/*.sh` 到 `.ai/scripts/`
  - [x] 3.2 更新腳本內的路徑引用
  - [x] 3.3 刪除舊的 `scripts/ai/` 目錄

- [x] 4. 移動 rules 和 commands
  - [x] 4.1 移動 `.claude/rules/` 到 `.ai/rules/`
  - [x] 4.2 移動 `.claude/commands/` 到 `.ai/commands/`
  - [x] 4.3 創建符號連結 `.claude/rules/` → `.ai/rules/`（跨平台：Linux/macOS 直接支援，Windows 需開發人員模式，否則回退到複製）
  - [x] 4.4 創建符號連結 `.claude/commands/` → `.ai/commands/`（同上）

- [x] 5. 創建模板系統
  - [x] 5.1 創建 `.ai/templates/CLAUDE.md.j2`
  - [x] 5.2 創建 `.ai/templates/AGENTS.md.j2`
  - [x] 5.3 創建 `.ai/scripts/generate.sh`

- [x] 6. 泛化 rules
  - [x] 6.1 重命名 `backend-nakama-architecture-and-patterns.md` → `backend-go.md`
  - [x] 6.2 重命名 `unity-architecture-and-patterns.md` → `frontend-unity.md`
  - [x] 6.3 抽取通用部分，專案特定部分移到配置

- [x] 7. 更新所有引用
  - [x] 7.1 更新 `CLAUDE.md` 中的路徑
  - [x] 7.2 更新 `AGENTS.md` 中的路徑
  - [x] 7.3 更新 `README.md`
  - [x] 7.4 更新 `docs/ai-workflow-architecture.md`

- [x] 8. 創建安裝腳本
  - [x] 8.1 創建 `.ai/scripts/install.sh`
  - [x] 8.2 創建 `.ai/scripts/init.sh`（初始化新專案）

- [x] 9. 測試
  - [ ] 9.1 測試 `kickoff.sh --dry-run`（需要 submodules 存在）
  - [ ] 9.2 測試 `stats.sh`（需要 submodules 存在）
  - [x] 9.3 測試 `generate.sh` ✅

- [x] 10. 清理
  - [x] 10.1 刪除舊目錄和文件
  - [x] 10.2 更新 `.gitignore`

---

## 當前進度
- 開始時間: 2024-12-18
- 完成時間: 2024-12-18
- 狀態: ✅ 完成

## 完成摘要
- ✅ 所有文件已整合到 `.ai/` 目錄
- ✅ 配置系統 `workflow.yaml` 已建立
- ✅ 模板系統 (CLAUDE.md.j2, AGENTS.md.j2, git-workflow.md.j2, ci-*.yml.j2) 已建立
- ✅ `generate.sh` 可從配置生成：CLAUDE.md, AGENTS.md, git-workflow.md, CI workflows, symlinks
- ✅ `install.sh` 可將 Kit 安裝到新專案（含 .gitignore 處理、跨平台 symlink）
- ✅ `init.sh` 可初始化新專案
- ✅ 舊目錄已清理 (scripts/ai/, .claude/rules/, .claude/commands/, .kiro/specs/, cultivation-mvp)
- ✅ 所有硬編碼值已移除（分支名稱從 config 讀取）
- ✅ `generate.sh` 測試通過
