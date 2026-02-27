---
name: release-checklist
description: Pre-release verification to ensure quality before merging to main. Checks tests, uncommitted changes, changelog, dependencies, CI status, and branch state. Triggers: /release-checklist, pre-release check, release verification, go/no-go.
allowed-tools: Read, Grep, Glob, Bash
---

# Release Checklist Skill

Pre-release verification workflow that systematically validates all quality gates before a release merge to main.

## Overview

This skill runs a comprehensive checklist of verifications to determine whether the current state of the codebase is ready for release. It produces a clear go/no-go decision with supporting evidence for each check.

## When to Use

Use this skill when:
- Preparing to merge an integration branch into `main`
- Before creating a release tag
- User invokes `/release-checklist`
- User asks "are we ready to release?"
- Before creating a release PR (`feat/<branch>` -> `main`)

## Workflow

### Phase 1: Verify Release

**Read** `phases/verify-release.md`

Run the full pre-release verification:
1. Verify all tests pass
2. Check for uncommitted changes
3. Verify changelog/version
4. Check dependency vulnerabilities
5. Validate CI status
6. Verify branch state (up to date with main)
7. Output go/no-go decision

## Critical Rules

1. **ALL checks must pass for a GO decision** -- no exceptions
2. **Do not auto-fix issues** -- report them and let the user decide
3. **Be explicit about what failed** -- include exact error output
4. **Check the correct branch** -- verify you are on the integration branch, not main
5. **Never push or merge** -- this skill is verification only

## Integration with AWK Workflow

This skill is typically the last step before a release PR:

```
awkit kickoff → issues processed → PRs merged to integration branch
        ↓
/release-checklist → GO decision
        ↓
Create PR: integration branch → main (Release: true)
```

Per `.ai/rules/_kit/git-workflow.md`:
- Release PRs target `main`
- Only create release PRs when the ticket says `Release: true`

## Self-Check

On each phase entry, output:
```
[RELEASE-CHECKLIST] <timestamp> | <phase> | loaded: <filename>
```

## Quick Reference

| Phase | Action | File |
|-------|--------|------|
| 1. Verify Release | Full pre-release verification | `phases/verify-release.md` |
