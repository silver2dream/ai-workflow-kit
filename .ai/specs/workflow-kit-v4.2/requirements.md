# AI Workflow Kit v4.2 - Requirements

## Introduction

修正 evaluate.md/evaluate.sh 的規格嚴謹性問題，確保評估框架邏輯一致、無矛盾。

## Glossary

- **Offline_Gate**: 不需網路的本地驗證，P0 一票否決
- **Online_Gate**: 需要 gh auth + 網路的驗證
- **SKIP**: 檢查項目不適用（不扣分）
- **FAIL**: 檢查項目失敗（扣分）
- **Score_Cap**: 評分上限

## Requirements

### Requirement 1: O6 移出 Offline Gate

**User Story:** As a developer, I want O6 (CI/分支對齊) to be optional, so that repos without CI can still pass Offline Gate.

#### Acceptance Criteria

1. THE Evaluate_Script SHALL move O6 from Offline Gate to Extensibility P1
2. WHEN no `.github/workflows/` directory exists, THE Evaluate_Script SHALL mark O6 as SKIP
3. WHEN `.github/workflows/` exists but is empty, THE Evaluate_Script SHALL mark O6 as FAIL
4. WHEN workflows exist but don't trigger integration_branch, THE Evaluate_Script SHALL mark O6 as FAIL

### Requirement 2: SKIP 白名單

**User Story:** As a reviewer, I want SKIP to have strict rules, so that configuration errors are not mistakenly marked as "not applicable".

#### Acceptance Criteria

1. THE Evaluate_Script SHALL only allow SKIP for: missing optional dependencies (`file`), or explicitly not applicable (no CI)
2. WHEN a required config cannot be read, THE Evaluate_Script SHALL mark as FAIL (not SKIP)
3. WHEN a script execution error occurs, THE Evaluate_Script SHALL mark as FAIL (not SKIP)
4. THE Evaluate_Doc SHALL document the SKIP whitelist explicitly

### Requirement 3: 補齊依賴宣告

**User Story:** As a user, I want all dependencies documented, so that I can prepare my environment correctly.

#### Acceptance Criteria

1. THE Evaluate_Doc SHALL list `curl` in Online Gate prerequisites
2. THE Evaluate_Doc SHALL clarify that "Offline" means no network during evaluation, but dependencies must be pre-installed
3. THE Evaluate_Doc SHALL provide a requirements.txt or explicit pip install command

### Requirement 4: 標註示意程式碼

**User Story:** As a maintainer, I want to avoid doc drift, so that the documentation stays accurate.

#### Acceptance Criteria

1. THE Evaluate_Doc SHALL mark embedded code as "示意" (illustrative) or remove it
2. THE Evaluate_Doc SHALL reference `.ai/scripts/evaluate.sh` as the authoritative source
3. THE Evaluate_Doc SHALL provide only core behavior summary, not full implementation

### Requirement 5: 等級映射明確化

**User Story:** As a reviewer, I want clear grade definitions, so that scores and grades are unambiguous.

#### Acceptance Criteria

1. THE Evaluate_Doc SHALL define grade thresholds: A (9.0-10.0), B (8.0-8.9), C (7.0-7.9), D (6.0-6.9), F (<6.0)
2. THE Evaluate_Doc SHALL clarify that final grade = grade of min(total_score, cap)
3. THE Evaluate_Doc SHALL note that cap directly limits achievable grade

### Requirement 6: audit P0/P1 納入 (--strict 模式)

**User Story:** As a strict reviewer, I want to optionally fail on audit P0/P1, so that I can catch all issues.

#### Acceptance Criteria

1. THE Evaluate_Script SHALL support `--strict` flag
2. WHEN `--strict` is set, THE Evaluate_Script SHALL check audit.json for P0/P1 findings
3. WHEN audit has P0 findings and `--strict` is set, THE Evaluate_Script SHALL mark Offline Gate as FAIL
4. WHEN `--strict` is not set, THE Evaluate_Script SHALL only check audit.json validity (current behavior)
