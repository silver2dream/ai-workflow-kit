# AI Workflow Kit v5.0 - Requirements

## Introduction

修正 evaluate 框架的架構一致性問題，確保 Offline Gate 真正離線、audit 產物可追溯。

## Glossary

- **Offline_Gate**: 不需網路的本地驗證
- **audit.json**: 專案審計結果檔案
- **dirty_worktree**: 工作樹有未提交變更

## Requirements

### Requirement 1: Offline Gate 真正離線

**User Story:** As a developer, I want Offline Gate to work without network, so that I can evaluate in air-gapped environments.

#### Acceptance Criteria

1. THE Offline_Gate SHALL NOT perform any network operations (including `git fetch`)
2. WHEN submodules exist, THE audit_project SHALL skip pinned sha fetch verification in Offline mode
3. THE Evaluate_Script SHALL move submodule pinned sha check to Online Gate or a separate `--check-origin` flag
4. THE Evaluate_Doc SHALL accurately reflect that Offline Gate requires zero network access

### Requirement 2: 統一 audit.json schema

**User Story:** As a reviewer, I want consistent audit output, so that I can trace --strict failures.

#### Acceptance Criteria

1. THE audit_project.sh AND audit_project.py SHALL produce identical JSON schema
2. THE schema SHALL include: `severity`, `type`, `path`, `message`, `id` (optional)
3. THE Evaluate_Script SHALL define and validate against a formal audit.json schema
4. WHEN audit.json is overwritten, THE system SHALL preserve the original or use distinct filenames

### Requirement 3: 統一 dirty_worktree severity

**User Story:** As a user, I want consistent severity for dirty worktree, so that --strict behaves predictably.

#### Acceptance Criteria

1. THE audit_project.sh AND audit_project.py SHALL use the same severity for dirty_worktree
2. THE severity SHALL be P1 (not P0), because dirty worktree is expected during local development
3. THE Evaluate_Doc SHALL document that dirty_worktree is P1 and --strict only checks P0

### Requirement 4: 統一 repo_scan.json schema

**User Story:** As a developer, I want scan_repo.sh and scan_repo.py to be interchangeable.

#### Acceptance Criteria

1. THE scan_repo.sh AND scan_repo.py SHALL produce identical JSON schema
2. THE audit_project.sh SHALL work with either scan output format
3. THE schema SHALL be documented in a formal JSON schema file

## Background

這些問題在 v4.3 review 時被指出：

1. `audit_project.sh` 內含 `git fetch origin <sha>` 檢查，違反 Offline 定義
2. `.sh` 和 `.py` 版本的 audit.json schema 不一致，導致覆寫後無法追溯
3. `dirty_worktree` 在 `.sh` 是 P0、在 `.py` 是 P1，造成 --strict 行為不一致
4. `repo_scan.json` 的 `.sh` 和 `.py` 版本 schema 不同，導致 `audit_project.sh` 無法正確讀取 `.py` 產出
