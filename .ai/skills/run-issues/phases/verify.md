# Phase: Verify Results

## Overview

This phase verifies all completed work before reporting to the user.

## Verification Checklist

For each issue processed, verify ALL of the following:

### 1. PR Creation Check

```bash
gh pr list --state open --search "issue:<number>" --json number,title,state,headRefName
```

Or check if PR body contains `Closes #<number>`:

```bash
gh pr list --state all --json number,body --jq '.[] | select(.body | contains("Closes #<number>"))'
```

**Pass criteria**: PR exists and references the issue

### 2. Branch Naming Check

Verify branch follows convention:

```
Valid patterns:
- feat/ai-issue-<number>
- fix/ai-issue-<number>
- feat/<topic>
- fix/<topic>
```

```bash
gh pr view <pr_number> --json headRefName --jq '.headRefName'
```

**Pass criteria**: Branch name matches expected pattern

### 3. Commit Format Validation

Check commits on the PR branch:

```bash
gh pr view <pr_number> --json commits --jq '.commits[].messageHeadline'
```

**Required format**: `[type] subject`

Valid examples:
- `[feat] add new endpoint`
- `[fix] resolve race condition`
- `[refactor] simplify auth logic`
- `[docs] update API documentation`
- `[test] add unit tests for handler`
- `[chore] update dependencies`

Invalid examples:
- `feat: add new endpoint` (colon instead of bracket)
- `Add new endpoint` (no type prefix)
- `[FEAT] Add Endpoint` (uppercase)

**Validation regex**:
```
^\[(feat|fix|docs|style|refactor|perf|test|chore)\] .+$
```

**Pass criteria**: ALL commits match the format

### 4. Test Verification

For Go projects, verify tests pass:

```bash
# Navigate to repo and run tests
cd <repo_path> && go test ./...
```

Or check CI status on PR:

```bash
gh pr checks <pr_number> --json name,state --jq '.[] | select(.name | contains("test"))'
```

**Pass criteria**: All tests pass (exit code 0 or CI status "success")

### 5. Build Verification

Verify the code builds:

```bash
cd <repo_path> && go build ./...
```

**Pass criteria**: Build succeeds (exit code 0)

### 6. PR Status Check

Verify PR is in good state:

```bash
gh pr view <pr_number> --json state,mergeable,reviewDecision
```

Check:
- `state`: should be "OPEN"
- `mergeable`: should be "MERGEABLE" (not "CONFLICTING")
- No blocking reviews

**Pass criteria**: PR is mergeable with no conflicts

## Verification Results

For each issue, record:

```json
{
  "issue_number": 123,
  "pr_number": 456,
  "checks": {
    "pr_created": true,
    "branch_naming": true,
    "commit_format": true,
    "tests_pass": true,
    "build_pass": true,
    "pr_mergeable": true
  },
  "status": "PASS",
  "notes": ""
}
```

### Status Determination

| All Checks Pass | Status |
|-----------------|--------|
| Yes | PASS |
| No (tests fail) | FAIL - Tests |
| No (commit format) | FAIL - Commit Format |
| No (build fails) | FAIL - Build |
| No (PR conflict) | FAIL - Conflict |
| No PR created | FAIL - No PR |

## Approval vs Request Changes

### Approve (PASS) when:
- All verification checks pass
- PR is ready for merge

### Request Changes when:
- Commit format is wrong
- Tests are failing
- Build is broken
- PR has conflicts

### Skip verification when:
- Issue was marked as skipped (dependency failure)
- Subagent timed out
- Issue was already completed before run

## Final Report Format

Generate summary table for user:

```markdown
## Issue Processing Report

**Run completed at**: <timestamp>
**Total issues**: <n>
**Successful**: <n>
**Failed**: <n>
**Skipped**: <n>

### Results

| Issue | Title | Priority | Status | PR | Checks | Notes |
|-------|-------|----------|--------|-----|--------|-------|
| #123 | Fix auth bug | P0 | PASS | [#456](url) | 6/6 | Ready to merge |
| #124 | Add feature | P1 | FAIL | [#457](url) | 5/6 | Tests failing |
| #125 | Update docs | P2 | PASS | [#458](url) | 6/6 | Ready to merge |
| #126 | Refactor | P2 | SKIP | - | - | Depends on #124 |

### Failed Issues Detail

#### Issue #124: Add feature
- **PR**: #457
- **Failed check**: Tests
- **Error**: `TestUserHandler` assertion failed
- **Action needed**: Fix test or implementation

### Next Steps
1. Review and merge passing PRs
2. Fix failing issues manually or re-run
3. Skipped issues will be processed in next run after dependencies resolve
```

## Automated Actions

### On PASS
- Add label `verified` to issue
- Add comment: "Verification passed. PR ready for review."

### On FAIL
- Add label `needs-fix` to issue
- Add comment with failure details
- Request changes on PR with specific feedback

```bash
# Example: Add verification comment
gh issue comment <number> --body "Automated verification complete. Status: PASS. PR #<pr> ready for review."

# Example: Request changes
gh pr review <pr_number> --request-changes --body "Commit format violation: expected [type] subject format."
```

## Self-Check

After verification:
```
[RUN-ISSUES] <timestamp> | verify | Verified: <n>, Pass: <n>, Fail: <n>, Skip: <n>
```
