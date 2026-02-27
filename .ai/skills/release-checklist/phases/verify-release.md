# Phase: Verify Release

## Overview

This phase runs all pre-release verification checks and produces a go/no-go decision. Each check is independent and produces a PASS/FAIL/WARN result.

---

## Pre-Flight

Before running checks, confirm the current branch and repository state:

```bash
# Identify current branch
git branch --show-current

# Identify the repo type from config
cat .ai/config/workflow.yaml | grep -A2 'repos:'
```

Verify you are on the **integration branch** (not `main`). If on `main`, warn the user and ask which branch to verify.

---

## Check 1: All Tests Pass

Run the project's test suite and verify zero failures.

### For Go projects:

```bash
go test ./... 2>&1
echo "Exit code: $?"
```

### For Node.js projects:

```bash
npm test 2>&1
echo "Exit code: $?"
```

### For monorepo (multiple repos):

Run tests for each configured repo directory:

```bash
# Example for backend (Go)
cd backend && go test ./... 2>&1; echo "Exit code: $?"

# Example for frontend (if applicable)
cd frontend && npm test 2>&1; echo "Exit code: $?"
```

**PASS criteria**: All test suites exit with code 0
**FAIL criteria**: Any test failure or non-zero exit code

```
[CHECK 1] Tests: <PASS|FAIL>
- Backend: <pass|fail> (<n> tests)
- Frontend: <pass|fail|skip> (<n> tests)
- Failures: <list if any>
```

---

## Check 2: No Uncommitted Changes

Verify the working tree is clean -- no staged, unstaged, or untracked files that should be committed.

```bash
git status --porcelain
```

**PASS criteria**: Output is empty (clean working tree)
**WARN criteria**: Only untracked files in `.ai/` or other non-release directories
**FAIL criteria**: Modified tracked files or staged changes exist

```
[CHECK 2] Working tree: <PASS|WARN|FAIL>
- Modified files: <count>
- Staged files: <count>
- Untracked files: <count>
- Details: <list if any>
```

---

## Check 3: Changelog / Version Verification

Verify that version information and changelog are up to date.

### 3.1 Check for version file or tag

```bash
# Check for version in common locations
cat VERSION 2>/dev/null || cat version.txt 2>/dev/null || echo "No VERSION file"

# Check latest git tag
git tag --sort=-version:refname | head -5

# Check if current commit is tagged
git describe --exact-match HEAD 2>/dev/null || echo "HEAD is not tagged"
```

### 3.2 Check for changelog

```bash
# Look for changelog
ls -la CHANGELOG* HISTORY* RELEASES* 2>/dev/null

# If CHANGELOG.md exists, check last entry date
head -20 CHANGELOG.md 2>/dev/null
```

### 3.3 Check for version in code

```bash
# Go projects: check for version constant
grep -r 'version\s*=' cmd/ internal/ --include="*.go" -l 2>/dev/null | head -5

# Node projects: check package.json
cat package.json 2>/dev/null | grep '"version"'
```

**PASS criteria**: Version is identifiable and changelog has recent entry
**WARN criteria**: No changelog found, or changelog appears outdated
**FAIL criteria**: Version mismatch between files, or no version identifier at all

```
[CHECK 3] Version/Changelog: <PASS|WARN|FAIL>
- Version: <version string or "not found">
- Latest tag: <tag or "none">
- Changelog: <up to date|outdated|missing>
```

---

## Check 4: Dependency Vulnerabilities

Check for known vulnerabilities in project dependencies.

### For Go projects:

```bash
# Check for known vulnerabilities
go list -m all 2>/dev/null | wc -l
govulncheck ./... 2>&1 || echo "govulncheck not available"

# At minimum, verify modules are tidy
go mod tidy -diff 2>&1 || echo "go mod tidy check not supported"
go mod verify 2>&1
```

### For Node.js projects:

```bash
npm audit 2>&1
```

**PASS criteria**: No known vulnerabilities (or only low-severity)
**WARN criteria**: Low-severity vulnerabilities present, or vuln scanner not available
**FAIL criteria**: High or critical severity vulnerabilities found

```
[CHECK 4] Dependencies: <PASS|WARN|FAIL>
- Total dependencies: <count>
- Vulnerabilities: <none|low: n|high: n|critical: n>
- Module verification: <pass|fail>
```

---

## Check 5: CI Status Validation

Verify that CI checks are passing on the current branch.

```bash
# Get the latest commit SHA
COMMIT_SHA=$(git rev-parse HEAD)

# Check CI status via GitHub
gh run list --branch $(git branch --show-current) --limit 5 --json status,conclusion,name,headSha

# Or check specific PR CI status
gh pr list --head $(git branch --show-current) --json number,statusCheckRollup --limit 1
```

**PASS criteria**: Most recent CI run on this branch shows "success"
**WARN criteria**: No CI runs found (CI may not be configured)
**FAIL criteria**: Most recent CI run shows "failure" or "cancelled"

```
[CHECK 5] CI Status: <PASS|WARN|FAIL>
- Latest run: <success|failure|pending|none>
- Branch: <branch name>
- Commit: <short SHA>
- Details: <run URL if available>
```

---

## Check 6: Branch State (Up to Date with Main)

Verify the integration branch is up to date with `main` to avoid merge conflicts.

```bash
# Fetch latest from remote
git fetch origin main 2>&1

# Check if integration branch contains all commits from main
git log origin/main..HEAD --oneline | wc -l
git log HEAD..origin/main --oneline | wc -l

# Check for merge conflicts (dry run)
git merge-tree $(git merge-base HEAD origin/main) HEAD origin/main 2>&1 | head -20
```

### Interpret results:

- **Commits ahead of main**: Number of new commits to be released
- **Commits behind main**: Number of commits from main not yet merged into this branch

**PASS criteria**: Branch is 0 commits behind main (fully up to date), with at least 1 commit ahead
**WARN criteria**: Branch is behind main but no conflicts detected
**FAIL criteria**: Branch is behind main with merge conflicts, or branch has 0 commits ahead (nothing to release)

```
[CHECK 6] Branch state: <PASS|WARN|FAIL>
- Current branch: <branch name>
- Commits ahead of main: <count>
- Commits behind main: <count>
- Merge conflicts: <none|detected>
```

---

## Check 7 (Bonus): Build Verification

Verify the project builds successfully.

### For Go projects:

```bash
go build ./... 2>&1
echo "Exit code: $?"
```

### For Node.js projects:

```bash
npm run build 2>&1
echo "Exit code: $?"
```

**PASS criteria**: Build succeeds with exit code 0
**FAIL criteria**: Build fails

```
[CHECK 7] Build: <PASS|FAIL>
- Backend: <pass|fail>
- Frontend: <pass|fail|skip>
```

---

## Go/No-Go Decision

Compile all check results into a final decision.

### Decision Matrix

| Scenario | Decision |
|----------|----------|
| All checks PASS | **GO** |
| All PASS with WARN only | **GO (with notes)** |
| Any check FAIL | **NO-GO** |

### Output Summary

```markdown
# Release Verification Report

**Date**: <timestamp>
**Branch**: <branch name>
**Target**: main
**Commit**: <SHA>

## Results

| # | Check | Status | Details |
|---|-------|--------|---------|
| 1 | Tests | <PASS/FAIL> | <summary> |
| 2 | Working tree | <PASS/WARN/FAIL> | <summary> |
| 3 | Version/Changelog | <PASS/WARN/FAIL> | <summary> |
| 4 | Dependencies | <PASS/WARN/FAIL> | <summary> |
| 5 | CI status | <PASS/WARN/FAIL> | <summary> |
| 6 | Branch state | <PASS/WARN/FAIL> | <summary> |
| 7 | Build | <PASS/FAIL> | <summary> |

## Decision: <GO / NO-GO>

### Blockers (if NO-GO)
1. <blocker description with remediation>

### Warnings (if any)
1. <warning description>

### Next Steps
- <action items>
```

### On GO Decision

Recommend next steps:
```markdown
### Recommended Next Steps
1. Create release PR: `gh pr create --base main --title "[chore] release <version>" --body "Release checklist passed. Merging <branch> into main."`
2. After PR approval, merge to main
3. Tag the release: `git tag v<version>`
4. Push tag: `git push origin v<version>`
```

### On NO-GO Decision

List each blocker with specific remediation:
```markdown
### Blockers to Resolve
1. **<check name>**: <failure description>
   - **Fix**: <specific command or action>
   - **Re-run**: `/release-checklist` after fixing
```

## Self-Check

After verification:
```
[RELEASE-CHECKLIST] <timestamp> | verify-release | Checks: <pass>/<total> | Decision: <GO|NO-GO>
```
