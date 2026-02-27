# Phase: Analyze Failure

## Overview

This phase walks through a structured failure analysis to identify what went wrong, why it happened, what was affected, and how to prevent recurrence.

---

## Step 1: Collect Failure Context

Gather all available evidence before drawing any conclusions.

### 1.1 Identify the Failure Point

Ask the user (or infer from context):
- **What failed?** (command, phase, step)
- **When did it fail?** (timestamp or relative timing)
- **What was the expected outcome?**
- **What was the actual outcome?**

### 1.2 Collect Trace Files and Logs

```bash
# Check principal log
cat .ai/exe-logs/principal.log | tail -100

# Check worker logs (find the most recent)
ls -lt .ai/exe-logs/issue-*.worker.log 2>/dev/null | head -5

# Check AWK state files
ls -la .ai/state/

# Check recent git activity
git log --oneline -20
git status
```

### 1.3 Collect Environment Context

```bash
# Check workflow config
cat .ai/config/workflow.yaml

# Check for active locks
ls -la .ai/state/*.lock 2>/dev/null

# Check GitHub state (open PRs, recent issues)
gh pr list --state open --limit 10
gh issue list --state open --limit 10
```

### 1.4 Collect Error Messages

Extract exact error messages, stack traces, or exit codes. Record them verbatim -- do not paraphrase.

**Evidence inventory** -- list all collected artifacts:
```
[POST-MORTEM] Evidence collected:
- Principal log: <yes/no> (<line count>)
- Worker logs: <yes/no> (<count>)
- State files: <list>
- Error message: "<exact message>"
- Exit code: <code>
```

---

## Step 2: Identify Root Cause

Categorize the failure into one of these root cause categories:

### Category A: Configuration Error
- Missing or invalid field in `workflow.yaml`
- Wrong branch target, missing repo config
- Schema validation failure
- **Signals**: error mentions config field, YAML parse error, "not found" for config value

### Category B: Code Error
- Bug in `awkit` CLI or internal packages
- Worker produced invalid code (test failure, build break)
- Logic error in generated code
- **Signals**: Go panic/stack trace, test assertion failure, build error

### Category C: Environment Error
- Missing tool (`gh`, `git`, `go` not installed or wrong version)
- Permission denied (file system, GitHub token scope)
- Network failure (GitHub API unreachable)
- Disk space, memory, or process limits
- **Signals**: "command not found", "permission denied", "timeout", "rate limit"

### Category D: External Error
- GitHub API outage or rate limiting
- Upstream dependency failure (broken module, registry down)
- Merge conflict caused by concurrent external changes
- **Signals**: HTTP 5xx from GitHub, "conflict" in merge, dependency resolution failure

### Root Cause Determination

Record the finding:
```
[POST-MORTEM] Root cause:
- Category: <A|B|C|D> (<Configuration|Code|Environment|External>)
- Component: <specific component or file>
- Description: <1-2 sentence explanation>
- Confidence: <high|medium|low>
- Evidence: <reference to specific log line or artifact>
```

If confidence is low, list alternative hypotheses ranked by likelihood.

---

## Step 3: Determine Blast Radius

Assess what was affected by the failure.

### 3.1 Direct Impact
- Which issues/PRs were affected?
- Was any data corrupted (state files, git history)?
- Did any partial changes land (half-committed code)?

### 3.2 Indirect Impact
- Are downstream issues blocked?
- Is the workflow in a broken state requiring manual intervention?
- Are any branches in a dirty state?

### 3.3 Scope Assessment

```
[POST-MORTEM] Blast radius:
- Issues affected: <list of issue numbers>
- PRs affected: <list of PR numbers>
- Branches in bad state: <list>
- State files corrupted: <yes/no> (<list>)
- Workflow blocked: <yes/no>
- Manual intervention required: <yes/no>
```

---

## Step 4: Generate Remediation Steps

Provide specific, actionable steps to recover from the failure. Order by priority.

### 4.1 Immediate Recovery

Steps to get back to a working state:

```markdown
### Immediate Recovery Steps
1. <step 1 with exact command>
2. <step 2 with exact command>
...
```

Common recovery patterns:

```bash
# Reset AWK state (if workflow is stuck)
awkit reset

# Clean up orphaned branches
git branch -d feat/ai-issue-<number>

# Close orphaned PRs
gh pr close <number>

# Re-run failed step
awkit dispatch-worker --issue <number>
```

### 4.2 Verification

Steps to confirm recovery was successful:

```bash
# Verify clean state
awkit doctor
awkit status

# Verify git state
git status
git log --oneline -5
```

---

## Step 5: Create Prevention Measures

Identify changes that would prevent this class of failure from recurring.

### 5.1 Classify Prevention Type

| Type | Example |
|------|---------|
| Config guard | Add validation rule to workflow.yaml schema |
| Code fix | Fix bug in awkit command |
| Preflight check | Add new check to `awkit doctor` |
| Rule update | Update `.ai/rules/` to prevent bad patterns |
| Process change | Update skill or workflow documentation |
| Monitoring | Add alerting or better error messages |

### 5.2 Prevention Recommendations

For each recommendation:
```markdown
### Prevention: <short title>
- **Type**: <config guard|code fix|preflight check|rule update|process change|monitoring>
- **Priority**: <P0|P1|P2>
- **Description**: <what to change>
- **Location**: <file or component to modify>
- **Effort**: <small|medium|large>
```

---

## Step 6: Output Structured Report

Compile all findings into a single structured report.

### Report Template

```markdown
# Post-Mortem Report

**Date**: <timestamp>
**Failure**: <1-line summary>
**Severity**: <P0 critical|P1 high|P2 medium|P3 low>
**Status**: <resolved|mitigated|investigating>

## Timeline
- <time>: <event>
- <time>: <event>
- <time>: Failure detected
- <time>: Analysis started

## Root Cause
- **Category**: <Configuration|Code|Environment|External>
- **Component**: <specific component>
- **Description**: <explanation>
- **Confidence**: <high|medium|low>

## Blast Radius
- **Issues affected**: <list>
- **PRs affected**: <list>
- **Data impact**: <none|partial|significant>
- **Workflow blocked**: <yes|no>

## Remediation
### Immediate
1. <step>

### Verification
1. <step>

## Prevention Measures
| # | Type | Priority | Description | Effort |
|---|------|----------|-------------|--------|
| 1 | <type> | <priority> | <description> | <effort> |

## Lessons Learned
- <insight 1>
- <insight 2>

## Action Items
- [ ] <action 1> (owner: <who>)
- [ ] <action 2> (owner: <who>)
```

### Delivery

1. Output the report to the user
2. If the user requests it, save to `.ai/results/post-mortem-<date>.md`

## Self-Check

After completing analysis:
```
[POST-MORTEM] <timestamp> | analyze-failure | Root cause: <category> | Blast radius: <n> issues | Remediation: <n> steps | Prevention: <n> measures
```
