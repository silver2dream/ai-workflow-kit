---
name: post-mortem
description: Structured failure analysis for debugging workflow issues. Collects context, identifies root cause, determines blast radius, and generates remediation and prevention measures. Triggers: /post-mortem, failure analysis, debug workflow, incident review.
allowed-tools: Read, Grep, Glob, Bash
---

# Post-Mortem Skill

Structured failure analysis workflow for diagnosing and learning from workflow failures, build breaks, and operational incidents.

## Overview

This skill provides a systematic approach to failure analysis when something goes wrong in the AWK workflow pipeline. It walks through evidence collection, root cause identification, impact assessment, and actionable remediation -- producing a structured report that can be referenced for future prevention.

## When to Use

Use this skill when:
- A Worker dispatch fails or times out
- A PR is rejected due to unexpected errors
- CI/CD pipeline breaks
- `awkit kickoff` encounters unrecoverable errors
- Any workflow step produces unexpected results
- User invokes `/post-mortem`

## Workflow

### Phase 1: Analyze Failure

**Read** `phases/analyze-failure.md`

Walk through the structured failure analysis:
1. Collect failure context (trace files, logs, error messages)
2. Identify root cause (categorize: config, code, environment, external)
3. Determine blast radius (what was affected)
4. Generate remediation steps
5. Create prevention measures
6. Output structured report

## Critical Rules

1. **DO NOT modify any files during analysis** -- this is a read-only diagnostic skill
2. **Collect evidence first** -- never jump to conclusions without supporting data
3. **Categorize accurately** -- misclassification leads to wrong remediation
4. **Be specific in remediation** -- vague advice is not actionable
5. **Always produce the structured report** -- even if root cause is uncertain

## Integration with AWK Workflow

This skill is used after a failure has occurred. It does not interfere with active workflows.

- Safe to run while `awkit kickoff` is active (read-only)
- Results can inform new issues created via `/create-issues`
- Prevention measures may lead to config or rule updates

## Self-Check

On each phase entry, output:
```
[POST-MORTEM] <timestamp> | <phase> | loaded: <filename>
```

## Quick Reference

| Phase | Action | File |
|-------|--------|------|
| 1. Analyze Failure | Full structured failure analysis | `phases/analyze-failure.md` |
